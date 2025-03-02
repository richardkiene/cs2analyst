package visibility

import (
	"log"
	"log/slog"
	"math"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// IsShooterPointingAtTarget checks if the shooter is pointing at the target
// while considering obstacles in the map model.
func IsShooterPointingAtTarget(shooter, target types.PlayerTickData, shooterModel, targetModel Model, mapModel MapModel) bool {
	// Step 1: Compute the eye position of the shooter
	shooterEyeLevel := shooter.Position
	shooterEyeLevel.Z += 55 // Approximate eye level in CS2

	// Step 2: Compute the expected yaw and pitch
	dirVector := r3.Vector{
		X: target.Position.X - shooterEyeLevel.X,
		Y: target.Position.Y - shooterEyeLevel.Y,
		Z: target.Position.Z + 55 - shooterEyeLevel.Z, // Aim at chest level
	}
	targetDistance := dirVector.Norm()
	expectedYaw := NormalizeAngle(math.Atan2(dirVector.Y, dirVector.X) * (180.0 / math.Pi))
	horizontalDistance := math.Sqrt(dirVector.X*dirVector.X + dirVector.Y*dirVector.Y)
	expectedPitch := math.Atan2(dirVector.Z, horizontalDistance) * (180.0 / math.Pi)

	shooterYaw := NormalizeAngle(float64(shooter.ViewAngleX))
	shooterPitch := float64(shooter.ViewAngleY)
	yawDifference := math.Abs(expectedYaw - shooterYaw)
	if yawDifference > 180 {
		yawDifference = 360 - yawDifference
	}
	pitchDifference := math.Abs(expectedPitch - shooterPitch)

	if yawDifference > 50.0 || pitchDifference > 50.0 {
		return false
	}

	// Step 3: Define multiple target points (head, shoulders, chest, pelvis, legs) for improved accuracy
	targetPoints := []r3.Vector{
		target.Position.Add(r3.Vector{X: 0, Y: 0, Z: 72}), // Head
		target.Position.Add(r3.Vector{X: 0, Y: 0, Z: 64}), // Shoulders
		target.Position.Add(r3.Vector{X: 0, Y: 0, Z: 55}), // Chest
		target.Position.Add(r3.Vector{X: 0, Y: 0, Z: 40}), // Pelvis
		target.Position.Add(r3.Vector{X: 0, Y: 0, Z: 30}), // Legs
	}

	// Step 4: Check if any target points are visible using multiple rays
	visibleCount := 0
	for _, targetPoint := range targetPoints {
		if checkVisibilityWithOffsets(shooterEyeLevel, targetPoint, mapModel, targetDistance) {
			visibleCount++
		}
	}

	// If at least one point is visible, return true
	if visibleCount > 0 {
		return true
	}

	log.Printf("All target points blocked, no visibility to target.")
	return false
}

// checkVisibilityWithOffsets tests visibility by slightly adjusting the ray
func checkVisibilityWithOffsets(shooterPos, targetPos r3.Vector, mapModel MapModel, maxDist float64) bool {
	offsets := []r3.Vector{
		{X: 0, Y: 0, Z: 0},  // Center
		{X: 1, Y: 0, Z: 0},  // Right
		{X: -1, Y: 0, Z: 0}, // Left
		{X: 0, Y: 1, Z: 0},  // Forward
		{X: 0, Y: -1, Z: 0}, // Backward
		{X: 0, Y: 0, Z: 1},  // Up
		{X: 0, Y: 0, Z: -1}, // Down
		// Diagonal offsets for improved accuracy
		{X: 1, Y: 1, Z: 0},
		{X: -1, Y: -1, Z: 0},
		{X: 1, Y: -1, Z: 0},
		{X: -1, Y: 1, Z: 0},
	}

	for _, offset := range offsets {
		adjustedTarget := targetPos.Add(offset)
		rayDirection := adjustedTarget.Sub(shooterPos).Normalize()

		// Check for intersections along the ray, handling transparent materials
		if !checkRayWithTransparency(shooterPos, rayDirection, mapModel.BaseModel.bvh, maxDist) {
			return true // No blocking obstacles found
		}
	}
	return false
}

// checkRayWithTransparency checks if a ray is blocked by non-transparent objects
func checkRayWithTransparency(origin, direction r3.Vector, node *BVHNode, maxDistance float64) bool {
	if node == nil {
		return false // No node, no blocking
	}

	// Initialize variables for tracking hit information
	currentOrigin := origin
	remainingDistance := maxDistance

	// TODO: debugging code, remove it
	hitCount := 0
	transparentHitCount := 0

	// Continue tracing the ray through transparent objects
	for {
		hitPos := r3.Vector{}
		blocked, material := rayIntersectsBVHClosestHit(currentOrigin, direction, node, &hitPos, remainingDistance)

		// TODO: Debugging code, remove it
		hitCount++

		// If no hit or hit is beyond our range, we're done
		if !blocked {
			// TODO: debugging code, remove it
			slog.Info("Ray passed through scene without hitting anything",
				"hitCount", hitCount,
				"transparentHits", transparentHitCount)
			return false // No blocking
		}

		// If the material is transparent, continue from just beyond the hit point
		if material.IsTransparent {
			// TODO: debugging code, remove it
			transparentHitCount++
			slog.Info("Hit transparent material",
				"materialName", material.Name,
				"opacity", material.Opacity,
				"hitPos", hitPos)

			// Calculate new origin slightly beyond the hit point
			hitDistance := currentOrigin.Sub(hitPos).Norm()

			// Adjust the remaining distance and move the origin forward
			remainingDistance -= hitDistance
			if remainingDistance <= 0 {
				// TODO: debugging code, remove it
				slog.Info("Reached maximum distance after transparent hits",
					"transparentHits", transparentHitCount)
				return false // Reached maximum distance
			}

			// Move slightly beyond the hit point to avoid self-intersection
			const epsilon = 0.01
			currentOrigin = hitPos.Add(direction.Mul(epsilon))

			// Continue the loop to check for the next hit
			continue
		}

		// TODO: debugging code, remove it
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
func rayIntersectsBVHClosestHit(rayOrigin, rayDir r3.Vector, node *BVHNode, hitPosition *r3.Vector, maxDistance float64) (bool, MaterialProperties) {
	if node == nil {
		return false, MaterialProperties{}
	}

	// Check if ray intersects the bounding box of this node
	if !rayIntersectsAABB(rayOrigin, rayDir, node.bbox.Min, node.bbox.Max) {
		return false, MaterialProperties{}
	}

	closestHit := r3.Vector{}
	minDistance := maxDistance
	var closestMaterial MaterialProperties

	// If this is a leaf node, check each triangle
	if len(node.triangles) > 0 {
		for i, tri := range node.triangles {
			if rayIntersectsTriangleWithHit(rayOrigin, rayDir, tri, hitPosition) {
				dist := rayOrigin.Sub(*hitPosition).Norm()
				if dist < minDistance {
					minDistance = dist
					closestHit = *hitPosition
					// Get the material properties for this triangle
					if i < len(node.materials) {
						closestMaterial = node.materials[i]
					}
				}
			}
		}
		*hitPosition = closestHit
		return minDistance < maxDistance, closestMaterial
	}

	// Recursively check child nodes for closer intersection
	leftHit, leftMat := rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.left, hitPosition, minDistance)
	leftDist := math.MaxFloat64
	if leftHit {
		leftDist = rayOrigin.Sub(*hitPosition).Norm()
	}

	rightHit, rightMat := rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.right, hitPosition, minDistance)
	rightDist := math.MaxFloat64
	if rightHit {
		rightDist = rayOrigin.Sub(*hitPosition).Norm()
	}

	if leftHit && (!rightHit || leftDist < rightDist) {
		return true, leftMat
	}
	if rightHit {
		return true, rightMat
	}
	return false, MaterialProperties{}
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
