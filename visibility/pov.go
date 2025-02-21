package visibility

import (
	"log"
	"math"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// IsShooterPointingAtTarget checks if the shooter is pointing at the target
// while considering obstacles in the map model.
func IsShooterPointingAtTarget(shooter, target types.PlayerTickData, shooterModel, targetModel, mapModel Model) bool {
	// Step 1: Compute the eye position of the shooter and target
	shooterEyeLevel := shooter.Position
	shooterEyeLevel.Z += 55 // Adjusted approximate eye level in CS2
	targetEyeLevel := target.Position
	targetEyeLevel.Z += 55 // Adjust target's position to their eye level

	// Step 2: Compute direction vector from shooter's eye to target's eye
	dirVector := r3.Vector{
		X: targetEyeLevel.X - shooterEyeLevel.X,
		Y: targetEyeLevel.Y - shooterEyeLevel.Y,
		Z: targetEyeLevel.Z - shooterEyeLevel.Z,
	}
	targetDistance := dirVector.Norm() // Distance to target

	// Step 3: Compute expected yaw (horizontal angle)
	expectedYaw := math.Atan2(dirVector.Y, dirVector.X) * (180.0 / math.Pi)
	if expectedYaw < 0 {
		expectedYaw += 360 // Normalize to [0, 360]
	}

	// Step 4: Compute expected pitch (vertical angle)
	horizontalDistance := math.Sqrt(dirVector.X*dirVector.X + dirVector.Y*dirVector.Y)
	expectedPitch := math.Atan2(dirVector.Z, horizontalDistance) * (180.0 / math.Pi)

	// Step 5: Normalize shooter's angles
	shooterYaw := NormalizeAngle(float64(shooter.ViewAngleX))
	shooterPitch := float64(shooter.ViewAngleY)

	// Step 6: Compute yaw and pitch difference
	yawDifference := math.Abs(expectedYaw - shooterYaw)
	if yawDifference > 180 {
		yawDifference = 360 - yawDifference // Ensure shortest distance
	}

	pitchDifference := math.Abs(expectedPitch - shooterPitch)

	// Step 7: Check if shooter is looking at target within FOV
	fovThreshold := 50.0 // 100-degree FOV total
	if yawDifference > fovThreshold || pitchDifference > fovThreshold {
		return false
	}

	// Step 8: Generate a ray from the shooter's eye position towards the target's eye level
	rayDirection := computeAimDirection(shooter.ViewAngleX, shooter.ViewAngleY)

	// Debug: Log Ray Path Details
	//log.Printf("Ray Start: %+v, Direction: %+v, Target: %+v, Max Distance: %.2f", shooterEyeLevel, rayDirection, targetEyeLevel, targetDistance)

	// Step 9: Perform line-of-sight check with map geometry using BVH intersection
	hitPosition := r3.Vector{}
	blocked, closestHit := rayIntersectsBVHClosestHit(shooterEyeLevel, rayDirection, mapModel.bvh, &hitPosition, targetDistance)
	//blocked, _ := rayIntersectsBVHClosestHit(shooterEyeLevel, rayDirection, mapModel.bvh, &hitPosition, targetDistance)
	if blocked {
		log.Printf("Ray blocked at %+v (Closest hit), Expected Target at %+v", closestHit, targetEyeLevel)
		return false // Something is blocking the view
	}

	// Step 10: Adjust target hitbox to world coordinates
	targetWorldMin := targetModel.min.Add(target.Position)
	targetWorldMax := targetModel.max.Add(target.Position)

	// Debug: Log final world-space hitbox
	//log.Printf("Target Hitbox Bounds (Relative): Min: %+v, Max: %+v", targetModel.min, targetModel.max)
	//log.Printf("Target Hitbox Bounds (Adjusted World): Min: %+v, Max: %+v", targetWorldMin, targetWorldMax)

	// Step 11: Check if crosshair is on the target
	if rayIntersectsAABB(shooterEyeLevel, rayDirection, targetWorldMin, targetWorldMax) {
		//log.Printf("Ray intersects hitbox AABB! Checking detailed intersection...")
		hitTarget := isRayHittingTargetWithDebug(shooterEyeLevel, rayDirection, targetModel, target.Position)
		if hitTarget {
			//log.Printf("Ray successfully hit the target!")
			return true
		} else {
			//log.Printf("Ray passed through AABB but did NOT hit target's hitbox.")
			return false
		}
	} else {
		//log.Printf("Ray did NOT intersect target's AABB. No possible hit.")
		return false
	}
}

// isRayHittingTargetWithDebug adds debugging to check why the ray is not hitting the model
func isRayHittingTargetWithDebug(rayOrigin, rayDir r3.Vector, targetModel Model, targetPos r3.Vector) bool {
	if targetModel.triangles == nil || len(targetModel.triangles) == 0 {
		//log.Printf("Error: Target model has no triangles! Possible missing or uninitialized model data.")
		return false
	}

	for _, tri := range targetModel.triangles {
		// Transform triangle to world space
		worldV1 := tri.V1.Add(targetPos)
		worldV2 := tri.V2.Add(targetPos)
		worldV3 := tri.V3.Add(targetPos)

		//log.Printf("Checking intersection with WORLD triangle: V1=%+v, V2=%+v, V3=%+v", worldV1, worldV2, worldV3)
		if worldV1 == (r3.Vector{}) || worldV2 == (r3.Vector{}) || worldV3 == (r3.Vector{}) {
			//log.Printf("Error: Triangle contains nil or zeroed vector! Skipping invalid triangle.")
			continue
		}

		hit := rayIntersectsTriangleWithHitDebug(rayOrigin, rayDir, types.Triangle{V1: worldV1, V2: worldV2, V3: worldV3})
		if hit {
			//log.Printf("Ray hit a WORLD triangle in the model!")
			return true
		}
	}
	//log.Printf("Ray did NOT hit any triangles in the model.")
	return false
}

// rayIntersectsTriangleWithHitDebug adds debugging to triangle intersection checks
func rayIntersectsTriangleWithHitDebug(rayOrigin, rayDir r3.Vector, tri types.Triangle) bool {
	hitPos := r3.Vector{}
	hit := rayIntersectsTriangleWithHit(rayOrigin, rayDir, tri, &hitPos)
	if hit {
		//log.Printf("Triangle hit detected: V1=%+v, V2=%+v, V3=%+v", tri.V1, tri.V2, tri.V3)
	} else {
		//log.Printf("Triangle missed: V1=%+v, V2=%+v, V3=%+v", tri.V1, tri.V2, tri.V3)
	}
	return hit
}

// rayIntersectsBVHClosestHit ensures the closest intersection is returned, avoiding false positives from distant objects
func rayIntersectsBVHClosestHit(rayOrigin, rayDir r3.Vector, node *BVHNode, hitPosition *r3.Vector, maxDistance float64) (bool, r3.Vector) {
	if node == nil {
		return false, r3.Vector{}
	}

	// Check if ray intersects the bounding box of this node
	if !rayIntersectsAABB(rayOrigin, rayDir, node.bbox.Min, node.bbox.Max) {
		return false, r3.Vector{}
	}

	closestHit := r3.Vector{}
	minDistance := maxDistance

	// If this is a leaf node, check each triangle
	if len(node.triangles) > 0 {
		for _, tri := range node.triangles {
			if rayIntersectsTriangleWithHit(rayOrigin, rayDir, tri, hitPosition) {
				dist := rayOrigin.Sub(*hitPosition).Norm()
				if dist < minDistance {
					minDistance = dist
					closestHit = *hitPosition
				}
			}
		}
		return minDistance < maxDistance, closestHit
	}

	// Recursively check child nodes for closer intersection
	leftHit, leftPos := rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.left, hitPosition, minDistance)
	rightHit, rightPos := rayIntersectsBVHClosestHit(rayOrigin, rayDir, node.right, hitPosition, minDistance)

	if leftHit && (!rightHit || rayOrigin.Sub(leftPos).Norm() < rayOrigin.Sub(rightPos).Norm()) {
		return true, leftPos
	}
	if rightHit {
		return true, rightPos
	}
	return false, closestHit
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

// computeAimDirection generates a normalized direction vector from view angles
func computeAimDirection(yaw, pitch float32) r3.Vector {
	yawRad := float64(yaw) * (math.Pi / 180.0)
	pitchRad := float64(pitch) * (math.Pi / 180.0)

	x := math.Cos(pitchRad) * math.Cos(yawRad)
	y := math.Cos(pitchRad) * math.Sin(yawRad)
	z := math.Sin(pitchRad)

	return r3.Vector{X: x, Y: y, Z: z}
}

// isRayHittingTarget checks if the shooter's aim ray intersects with the target's model
func isRayHittingTarget(rayOrigin, rayDirection r3.Vector, target Model) bool {
	// First, check if the ray intersects with the bounding box of the model
	if !rayIntersectsAABB(rayOrigin, rayDirection, target.min, target.max) {
		return false // If it doesn't hit the bounding box, it won't hit the model
	}

	// Then, check against individual hitboxes
	for _, hitbox := range target.hitboxes {
		if rayIntersectsAABB(rayOrigin, rayDirection, hitbox.MinBounds, hitbox.MaxBounds) {
			return true // Ray hits a hitbox
		}
	}

	// Lastly, check against the triangles in the BVH for precise hit detection
	return rayIntersectsBVH(rayOrigin, rayDirection, target.bvh)
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

// rayIntersectsBVH performs a ray-triangle intersection check using a BVH
func rayIntersectsBVH(rayOrigin, rayDir r3.Vector, node *BVHNode) bool {
	if node == nil {
		return false
	}

	// Check if ray intersects the bounding box of this node
	if !rayIntersectsAABB(rayOrigin, rayDir, node.bbox.Min, node.bbox.Max) {
		return false
	}

	// If this is a leaf node, check each triangle
	if len(node.triangles) > 0 {
		for _, tri := range node.triangles {
			if rayIntersectsTriangle2(rayOrigin, rayDir, tri) {
				return true
			}
		}
		return false
	}

	// Recursively check child nodes
	return rayIntersectsBVH(rayOrigin, rayDir, node.left) || rayIntersectsBVH(rayOrigin, rayDir, node.right)
}

// rayIntersectsTriangle checks if a ray intersects a triangle
func rayIntersectsTriangle2(rayOrigin, rayDir r3.Vector, tri types.Triangle) bool {
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
	return t > epsilon
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
