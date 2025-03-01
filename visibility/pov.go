package visibility

import (
	"log"
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
		log.Printf("Shooter has visibility to target (visible points: %d)", visibleCount)
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
		hitPosition := r3.Vector{}
		//blocked, hitObject := rayIntersectsBVHClosestHit(shooterPos, rayDirection, mapModel.bvh, &hitPosition, maxDist)
		blocked, _ := rayIntersectsBVHClosestHit(shooterPos, rayDirection, mapModel.BaseModel.bvh, &hitPosition, maxDist)

		// Handle transparent objects like grates/windows
		//if blocked && hitObject.IsTransparent {
		//	continue // Ignore transparent objects and keep checking
		//}

		if !blocked {
			return true
		}
	}
	return false
}

// isRayHittingTargetWithDebug adds debugging to check why the ray is not hitting the model
func isRayHittingTargetWithDebug(rayOrigin, rayDir r3.Vector, targetModel Model, targetPos r3.Vector) bool {
	if len(targetModel.triangles) == 0 {
		log.Printf("Error: Target model has no triangles! Possible missing or uninitialized model data.")
		return false
	}

	for _, tri := range targetModel.triangles {
		worldV1 := tri.V1.Add(targetPos)
		worldV2 := tri.V2.Add(targetPos)
		worldV3 := tri.V3.Add(targetPos)

		log.Printf("Checking intersection with WORLD triangle: V1=%+v, V2=%+v, V3=%+v", worldV1, worldV2, worldV3)
		if hit := rayIntersectsTriangleWithHitDebug(rayOrigin, rayDir, types.Triangle{V1: worldV1, V2: worldV2, V3: worldV3}); hit {
			log.Printf("Ray hit a WORLD triangle in the model!")
			return true
		}
	}

	log.Printf("Ray did NOT hit any triangles in the model.")
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
