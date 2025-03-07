package visibility

import (
	"log/slog"
	"math"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// IsShooterPointingAtTarget checks if the shooter is pointing at the target
// by doing a direct line-of-sight check from shooter eye to each target point.
// Updated to use coordinate transformation for proper alignment with map geometry.
func IsShooterPointingAtTarget(shooter, target types.PlayerTickData, shooterModel, targetModel Model, mapModel MapModel) bool {
	coords := NewDefaultSource2Coordinates()

	// 1) Transform both player's positions to model space
	transformedShooter := transformPlayerTickToModelSpace(shooter, &mapModel)
	transformedTarget := transformPlayerTickToModelSpace(target, &mapModel)

	// 2) Compute shooter's eye position using the transformed position
	shooterEye := GetAdjustedEyePosition(transformedShooter, &shooterModel, &mapModel)

	slog.Info("Shooter position data",
		"raw_replay_pos", shooter.Position,
		"transformed_pos", transformedShooter.Position,
		"eye_pos", shooterEye,
		"viewAngleX", shooter.ViewAngleX,
		"viewAngleY", shooter.ViewAngleY,
	)

	// 3) Use the shooter's view angles to compute their forward vector
	shooterYaw := float64(shooter.ViewAngleX)   // X in degrees
	shooterPitch := float64(shooter.ViewAngleY) // Y in degrees

	yawRad := shooterYaw * (math.Pi / 180.0)
	pitchRad := shooterPitch * (math.Pi / 180.0)

	slog.Info("Computed shooter angles",
		"yawRad", yawRad,
		"pitchRad", pitchRad)

	// Forward vector in Source2 coordinates
	forwardVector := coords.ApplyCS2Rotation(shooter.ViewAngleX, shooter.ViewAngleY, r3.Vector{X: 1, Y: 0, Z: 0})

	// 4) Define the target points (hitbox approximation) using transformed coordinates
	// The height values are in Source2 units, so scale them to model space
	scaleHeight := func(height float64) float64 {
		return height * coords.UnitScale
	}

	targetPoints := []r3.Vector{
		transformedTarget.Position.Add(r3.Vector{X: 0, Y: 0, Z: scaleHeight(72)}), // head
		transformedTarget.Position.Add(r3.Vector{X: 0, Y: 0, Z: scaleHeight(64)}), // shoulders
		transformedTarget.Position.Add(r3.Vector{X: 0, Y: 0, Z: scaleHeight(55)}), // chest
		transformedTarget.Position.Add(r3.Vector{X: 0, Y: 0, Z: scaleHeight(40)}), // pelvis
		transformedTarget.Position.Add(r3.Vector{X: 0, Y: 0, Z: scaleHeight(30)}), // legs
	}

	// 5) Decide on a max angle difference (shooter FOV half-angle)
	// If you want ~100° total, set 50° as half-angle
	maxAngleDifference := 50.0

	// 6) For each target point, do a direct line-of-sight check
	for _, tp := range targetPoints {
		// A) Compute direction from shooter eye to this target point
		dir := tp.Sub(shooterEye)
		dist := dir.Norm()
		if dist < 1e-3 {
			// Degenerate case: very close or identical positions
			continue
		}
		dirNorm := dir.Normalize()

		// B) Check angle difference
		// Dot product to find angle between shooter's forward vector and dirNorm
		dot := forwardVector.Dot(dirNorm)
		// Clamp to [-1, 1] to avoid floating-point issues
		dot = math.Max(-1.0, math.Min(1.0, dot))
		angleDiff := math.Acos(dot) * (180.0 / math.Pi)

		if angleDiff <= maxAngleDifference {
			// Within FOV, now check if geometry is blocking
			slog.Info("Target point is within FOV, checking line-of-sight",
				"angleDiff", angleDiff,
				"shooterYaw", shooterYaw,
				"shooterPitch", shooterPitch,
				"targetPos", tp,
			)

			// Call your transparency-aware ray check
			blocked := checkRayWithTransparency(
				shooterEye,
				dirNorm,
				mapModel.BaseModel.bvh,
				dist*1.1, // maxDistance slightly bigger than distance to target
			)

			if !blocked {
				// Not blocked => we have line-of-sight to this target point
				return true
			}
		} else {
			slog.Debug("Skipping target point outside FOV",
				"angleDiff", angleDiff,
				"maxAngleDifference", maxAngleDifference,
				"targetPos", tp,
			)
		}
	}

	// If no target point was found visible, return false
	return false
}

// checkRayWithTransparency checks if a ray is blocked by non-transparent objects
func checkRayWithTransparency(origin, direction r3.Vector, node *BVHNode, maxDistance float64) bool {
	if node == nil {
		return false // No node, no blocking
	}

	slog.Info("Checking ray with transparency",
		"origin", origin,
		"direction", direction,
		"maxDistance", maxDistance,
		"node.bbox.Min", node.bbox.Min,
		"node.bbox.Max", node.bbox.Max,
		"triangles", len(node.triangles),
		"materials", len(node.materials),
	)

	// Initialize variables for tracking hit information
	currentOrigin := origin
	remainingDistance := maxDistance

	// Debug counters
	hitCount := 0
	transparentHitCount := 0

	// Continue tracing the ray through transparent objects
	for {
		hitPos := r3.Vector{}
		blocked, material, _ := rayIntersectsBVHClosestHit(currentOrigin, direction, node, &hitPos, remainingDistance)

		hitCount++

		// If no hit or hit is beyond our range, we're done
		if !blocked {
			slog.Info("Ray passed through scene without hitting anything",
				"hitCount", hitCount,
				"transparentHits", transparentHitCount)
			return false // No blocking
		}

		// Check if the material should be treated as transparent:
		// 1. First check the IsTransparent flag (set during GLTF import)
		// 2. If that's false, fallback to the runtime material name check
		materialIsTransparent := material.IsTransparent

		// If not marked as transparent in the GLTF, check the runtime material detection
		if !materialIsTransparent && isTransparentMaterial(material.Name) {
			materialIsTransparent = true
			slog.Debug("Material detected as transparent by name pattern",
				"materialName", material.Name)
		}

		if materialIsTransparent {
			transparentHitCount++
			slog.Info("Hit transparent material",
				"materialName", material.Name,
				"opacity", material.Opacity,
				"hitPos", hitPos,
				"markedTransparentInGLTF", material.IsTransparent,
				"detectedByNamePattern", isTransparentMaterial(material.Name))

			// Calculate new origin slightly beyond the hit point
			hitDistance := currentOrigin.Sub(hitPos).Norm()

			// Adjust the remaining distance and move the origin forward
			remainingDistance -= hitDistance
			if remainingDistance <= 0 {
				slog.Info("Reached maximum distance after transparent hits",
					"transparentHits", transparentHitCount)
				return false // Reached maximum distance
			}

			// Move slightly beyond the hit point to avoid self-intersection
			const epsilon = 0.001
			currentOrigin = hitPos.Add(direction.Mul(epsilon))

			// Continue the loop to check for the next hit
			continue
		}

		slog.Info("Hit opaque material, ray blocked",
			"materialName", material.Name,
			"opacity", material.Opacity,
			"hitPos", hitPos,
			"totalHits", hitCount,
			"transparentHits", transparentHitCount)

		// Found a non-transparent blocking object
		return true
	}
}

// rayIntersectsBVHClosestHit ensures the closest intersection is returned, avoiding false positives from distant objects
// Return: (didWeHit, whichMaterial, whichTriangleIndex)
func rayIntersectsBVHClosestHit(
	rayOrigin, rayDir r3.Vector,
	node *BVHNode,
	hitPosition *r3.Vector,
	maxDistance float64,
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
				if dist < minDistance {
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
		rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.left, &leftPos, maxDistance)
	leftDist := math.MaxFloat64
	if leftHit {
		leftDist = rayOrigin.Sub(leftPos).Norm()
	}

	rightPos := r3.Vector{}
	rightHit, rightMat, rightIdx :=
		rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.right, &rightPos, maxDistance)
	rightDist := math.MaxFloat64
	if rightHit {
		rightDist = rayOrigin.Sub(rightPos).Norm()
	}

	// Now decide which side is closer, if either
	if leftHit && (!rightHit || leftDist < rightDist) {
		// The left child gave us the closest intersection
		*hitPosition = leftPos

		// The final intersection is from the left side
		slog.Info("Collision found",
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
		slog.Info("Collision found",
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
