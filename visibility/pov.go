package visibility

import (
	"math"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// IsShooterPointingAtTarget determines if the shooter is aiming at the target within a 100-degree FOV,
// and also if their crosshairs are on any part of the target's model.
func IsShooterPointingAtTarget(shooter, target types.PlayerTickData, shooterModel, targetModel Model) bool {
	// Step 1: Compute direction vector from shooter to target
	dirVector := r3.Vector{
		X: target.Position.X - shooter.Position.X,
		Y: target.Position.Y - shooter.Position.Y,
		Z: target.Position.Z - shooter.Position.Z,
	}

	// Step 2: Compute expected yaw (horizontal angle)
	expectedYaw := math.Atan2(dirVector.Y, dirVector.X) * (180.0 / math.Pi)
	if expectedYaw < 0 {
		expectedYaw += 360 // Normalize to [0, 360]
	}

	// Step 3: Compute expected pitch (vertical angle)
	horizontalDistance := math.Sqrt(dirVector.X*dirVector.X + dirVector.Y*dirVector.Y)
	expectedPitch := math.Atan2(dirVector.Z, horizontalDistance) * (180.0 / math.Pi)

	// Convert shooter’s pitch to match expected pitch format
	shooterPitch := float64(shooter.ViewAngleY)
	if shooterPitch > 90 {
		shooterPitch -= 360 // Convert to range [-90, 90]
	}

	// Define the allowed field of view (100-degree total, meaning ±50 degrees from center)
	fovThreshold := 50.0

	// Step 4: Check if the shooter is looking in the general direction of the target
	yawDifference := math.Abs(expectedYaw - float64(shooter.ViewAngleX))
	if yawDifference > 180 {
		yawDifference = 360 - yawDifference // Normalize to shortest angular difference
	}

	pitchDifference := math.Abs(expectedPitch - shooterPitch)

	if yawDifference > fovThreshold || pitchDifference > fovThreshold {
		return false // Shooter is not even facing the target within the FOV threshold
	}

	// Step 5: Check if the crosshairs intersect with the target model
	rayDirection := computeAimDirection(shooter.ViewAngleX, shooter.ViewAngleY)

	return isRayHittingTarget(shooter.Position, rayDirection, targetModel)
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
