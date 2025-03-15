package visibility

import (
	"log/slog"
	"math"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

func IsShooterPointingAtTarget(shooter, target types.PlayerTickData, shooterModel, targetModel Model, mapModel MapModel, activeSmokes []types.ActiveSmoke, currentTick int) bool {
	// TODO: This should really be a config or env value that is passed at run time
	debugEnabled := false //shooter.SteamID == 76561199002420143 && target.SteamID == 76561198237889474

	// Use the shared eye position function
	eyePos := GetEyePosition(shooter)

	// Transform to model space for ray casting
	coords := NewDefaultSource2Coordinates()
	transformedEyePos := coords.CS2ToModelSpace(eyePos)
	transformedTargetPos := coords.CS2ToModelSpace(target.Position)

	if debugEnabled {
		slog.Info("Checking visibility from shooter to target",
			"shooter_position", shooter.Position,
			"target_position", target.Position,
			"transformed_eye", transformedEyePos,
			"transformed_target", transformedTargetPos)
	}

	// Check if any active smoke obstructs the line of sight
	for _, smoke := range activeSmokes {
		if smoke.IsActiveAt(currentTick) && isSmokeBlocking(transformedEyePos, transformedTargetPos, smoke) {
			slog.Info("LOS Blocked by Smoke",
				"currentTick", currentTick,
				"smoke", smoke,
				"shooter.SteamID", shooter.SteamID,
				"shooter.PlayerName", shooter.PlayerName,
				"shooter_position", shooter.Position,
				"target.SteamID", target.SteamID,
				"target.PlayerName", target.PlayerName,
				"target_position", target.Position,
				"transformed_eye", transformedEyePos,
				"transformed_target", transformedTargetPos)
			return false
		}
	}

	// Get the shooter's forward vector in model space
	shooterForward := coords.ApplyCS2Rotation(shooter.ViewAngleX, shooter.ViewAngleY, r3.Vector{X: 1, Y: 0, Z: 0})

	// Use the shared sample point generator
	targetSamplePoints := GenerateTargetSamplePoints(transformedTargetPos, shooterForward)

	// Try each sample point until we find one that's visible
	for i, samplePoint := range targetSamplePoints {
		// Direction and distance to this sample point
		dirToSample := samplePoint.Sub(transformedEyePos).Normalize()
		distance := transformedEyePos.Sub(samplePoint).Norm()

		if debugEnabled {
			slog.Info("Checking sample point",
				"index", i,
				"position", samplePoint,
				"direction", dirToSample,
				"distance", distance)
		}

		// Use the shared ray casting function
		blocked, hitPos, material := CastRayToTarget(transformedEyePos, dirToSample, mapModel.BaseModel.bvh, distance*1.1, debugEnabled)

		if blocked {
			hitDistance := transformedEyePos.Sub(hitPos).Norm()

			// Use slightly more precise equality check for the distance comparison
			const distanceEpsilon = 0.1
			if math.Abs(hitDistance-distance) <= distanceEpsilon {
				// Hit is exactly at target distance (within epsilon)
				if debugEnabled {
					slog.Info("Ray hit exactly at target distance, considering visible",
						"hitDistance", hitDistance,
						"targetDistance", distance,
						"difference", math.Abs(hitDistance-distance))
				}
				return true
			}

			// Check if hit is beyond target (this shouldn't happen with precise ray casting)
			if hitDistance > distance+distanceEpsilon {
				if debugEnabled {
					slog.Info("Hit is beyond target, target is visible",
						"hitDistance", hitDistance,
						"targetDistance", distance,
						"difference", hitDistance-distance)
				}
				return true
			}

			if debugEnabled {
				slog.Info("Ray blocked by material",
					"material", material.Name,
					"hitDistance", hitDistance,
					"targetDistance", distance,
					"hitPos", hitPos)
			}
		} else {
			if debugEnabled {
				// No blocking!
				slog.Info("Target is visible via sample point", "index", i, "position", samplePoint)
			}
			return true
		}
	}

	// None of the sample points were visible
	if debugEnabled {
		slog.Info("Target is not visible - all sample points blocked")
	}

	return false
}

func isSmokeBlocking(rayOrigin, rayEnd r3.Vector, smoke types.ActiveSmoke) bool {
	// Simple check to see if the line segment from rayOrigin to rayEnd intersects with the smoke area.
	// This is a basic implementation and might need more sophisticated collision detection in practice.

	// Check if rayOrigin or rayEnd is inside the smoke
	if pointInCircle(rayOrigin, smoke.Position, smoke.OuterRadius) && !pointInCircle(rayOrigin, smoke.Position, smoke.InnerRadius) {
		return true
	}
	if pointInCircle(rayEnd, smoke.Position, smoke.OuterRadius) && !pointInCircle(rayEnd, smoke.Position, smoke.InnerRadius) {
		return true
	}

	// Check if the line segment intersects with the outer boundary of the smoke
	// This is a simple approximation and may not cover all cases.
	if segmentIntersectsCircle(rayOrigin, rayEnd, smoke.Position, smoke.OuterRadius) {
		return true
	}

	return false
}

func pointInCircle(point, center r3.Vector, radius float64) bool {
	distance := math.Sqrt(math.Pow(point.X-center.X, 2) + math.Pow(point.Y-center.Y, 2))
	return distance <= radius
}

func segmentIntersectsCircle(p1, p2, center r3.Vector, radius float64) bool {
	// Line equation: (y - y1) = m(x - x1)
	// Circle equation: (x - h)^2 + (y - k)^2 = r^2
	// Substitute line equation into circle equation and solve for x or y

	// This is a simplified version and might not handle all edge cases.
	// Consider using a library for more robust geometry calculations.

	// Calculate the direction vector of the segment
	dx := p2.X - p1.X
	dy := p2.Y - p1.Y

	// Calculate the vector from the circle's center to the start of the line segment
	fx := p1.X - center.X
	fy := p1.Y - center.Y

	// Calculate the coefficients for the quadratic equation
	a := dx*dx + dy*dy
	b := 2 * (fx*dx + fy*dy)
	c := fx*fx + fy*fy - radius*radius

	// Solve the quadratic equation using the quadratic formula
	discriminant := b*b - 4*a*c

	if discriminant < 0 {
		return false // No intersection
	}

	t1 := (-b - math.Sqrt(discriminant)) / (2 * a)
	t2 := (-b + math.Sqrt(discriminant)) / (2 * a)

	// Check if the intersection points are within the line segment
	if t1 >= 0 && t1 <= 1 {
		return true
	}
	if t2 >= 0 && t2 <= 1 {
		return true
	}

	return false
}

// rayIntersectsBVHClosestHit ensures the closest intersection is returned, avoiding false positives from distant objects
// Return: (didWeHit, whichMaterial, whichTriangleIndex)
func rayIntersectsBVHClosestHit(
	rayOrigin, rayDir r3.Vector,
	node *BVHNode,
	hitPosition *r3.Vector,
	maxDistance float64,
	debugEnabled bool,
) (bool, MaterialProperties, int) {
	if node == nil {
		return false, MaterialProperties{}, -1
	}

	// Check bounding box
	if !rayIntersectsAABB(rayOrigin, rayDir, node.bbox.Min, node.bbox.Max) {
		return false, MaterialProperties{}, -1
	}

	closestHit := r3.Vector{}
	minDistance := maxDistance
	var closestMaterial MaterialProperties
	closestIndex := -1

	// If leaf node, check each triangle
	if len(node.triangles) > 0 {
		for i, tri := range node.triangles {
			if rayIntersectsTriangleWithHit(rayOrigin, rayDir, tri, hitPosition) {
				dist := rayOrigin.Sub(*hitPosition).Norm()
				// When comparing distances, use an epsilon for floating point equality
				const epsilon = 1e-6
				if dist < minDistance-epsilon {
					minDistance = dist
					closestHit = *hitPosition
					// Store the material & the index for that triangle
					if i < len(node.materials) {
						closestMaterial = node.materials[i]
					} else {
						// In case you have more triangles than materials
						closestMaterial = MaterialProperties{}
					}
					closestIndex = i
				}
			}
		}

		*hitPosition = closestHit
		// If minDistance < maxDistance, we have a valid hit
		hitFound := (minDistance < maxDistance)
		return hitFound, closestMaterial, closestIndex
	}

	// Otherwise, recurse into child nodes
	// We need to track which side gave the closer intersection
	leftPos := r3.Vector{}
	leftHit, leftMat, leftIdx :=
		rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.left, &leftPos, maxDistance, debugEnabled)
	leftDist := math.MaxFloat64
	if leftHit {
		leftDist = rayOrigin.Sub(leftPos).Norm()
	}

	rightPos := r3.Vector{}
	rightHit, rightMat, rightIdx :=
		rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.right, &rightPos, maxDistance, debugEnabled)
	rightDist := math.MaxFloat64
	if rightHit {
		rightDist = rayOrigin.Sub(rightPos).Norm()
	}

	// Now decide which side is closer, if either
	if leftHit && (!rightHit || leftDist < rightDist) {
		// The left child gave us the closest intersection
		*hitPosition = leftPos

		// The final intersection is from the left side
		slog.Debug("Collision found",
			"triangleIndex", leftIdx,
			"materialName", leftMat.Name,
			"materialOpacity", leftMat.Opacity,
			"materialRefractionIndex", leftMat.RefractionIndex,
			"materialIsTransparent", leftMat.IsTransparent,
			"hitPosition", *hitPosition,
		)
		return true, leftMat, leftIdx
	}
	if rightHit {
		// The right child was closer
		*hitPosition = rightPos

		// The final intersection is from the right side
		slog.Debug("Collision found",
			"triangleIndex", rightIdx,
			"materialName", rightMat.Name,
			"materialOpacity", rightMat.Opacity,
			"materialRefractionIndex", rightMat.RefractionIndex,
			"materialIsTransparent", rightMat.IsTransparent,
			"hitPosition", *hitPosition,
		)
		return true, rightMat, rightIdx
	}

	// If neither child was hit, return no collision
	return false, MaterialProperties{}, -1
}

// rayIntersectsTriangleWithHit checks if a ray intersects a triangle and stores the hit position.
func rayIntersectsTriangleWithHit(rayOrigin, rayDir r3.Vector, tri types.Triangle, hitPosition *r3.Vector) bool {
	if hitPosition == nil {
		//	log.Printf("Error: hitPosition is nil, cannot assign!")
		return false
	}

	// Ensure triangle vertices are valid
	if tri.V1 == (r3.Vector{}) || tri.V2 == (r3.Vector{}) || tri.V3 == (r3.Vector{}) {
		//log.Printf("Error: Triangle contains invalid vertices! Skipping intersection check.")
		return false
	}

	const epsilon = 1e-6
	edge1 := tri.V2.Sub(tri.V1)
	edge2 := tri.V3.Sub(tri.V1)

	h := rayDir.Cross(edge2)
	a := edge1.Dot(h)

	if math.Abs(a) < epsilon {
		return false // Ray is parallel to triangle
	}

	f := 1.0 / a
	s := rayOrigin.Sub(tri.V1)
	u := f * s.Dot(h)

	if u < 0.0 || u > 1.0 {
		return false
	}

	q := s.Cross(edge1)
	v := f * rayDir.Dot(q)

	if v < 0.0 || u+v > 1.0 {
		return false
	}

	t := f * edge2.Dot(q)
	if t > epsilon {
		*hitPosition = rayOrigin.Add(rayDir.Mul(t))
		return true
	}

	return false
}

// rayIntersectsAABB checks if a ray intersects an axis-aligned bounding box (AABB)
func rayIntersectsAABB(rayOrigin, rayDir, minBounds, maxBounds r3.Vector) bool {
	tMin := (minBounds.X - rayOrigin.X) / rayDir.X
	tMax := (maxBounds.X - rayOrigin.X) / rayDir.X
	if tMin > tMax {
		tMin, tMax = tMax, tMin
	}

	tYMin := (minBounds.Y - rayOrigin.Y) / rayDir.Y
	tYMax := (maxBounds.Y - rayOrigin.Y) / rayDir.Y
	if tYMin > tYMax {
		tYMin, tYMax = tYMax, tYMin
	}

	if (tMin > tYMax) || (tYMin > tMax) {
		return false
	}

	if tYMin > tMin {
		tMin = tYMin
	}
	if tYMax < tMax {
		tMax = tYMax
	}

	tZMin := (minBounds.Z - rayOrigin.Z) / rayDir.Z
	tZMax := (maxBounds.Z - rayOrigin.Z) / rayDir.Z
	if tZMin > tZMax {
		tZMin, tZMax = tZMax, tZMin
	}

	if (tMin > tZMax) || (tZMin > tMax) {
		return false
	}

	return true
}

func NormalizeAngle(angle float64) float64 {
	if angle < 0 {
		angle += 360
	}
	if angle >= 360 {
		angle -= 360
	}
	return angle
}
