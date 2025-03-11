package visibility

import (
	"log/slog"
	"math"
	"strings"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// IsShooterPointingAtTarget checks if the shooter is pointing at the target
// by doing a direct line-of-sight check from shooter eye to each target point.
// Updated to use coordinate transformation for proper alignment with map geometry.
// IsShooterPointingAtTarget checks if the shooter is pointing at the target
// by doing a direct line-of-sight check from shooter eye to multiple target points.
// It sends multiple rays to account for partial visibility.
func IsShooterPointingAtTarget(shooter, target types.PlayerTickData, shooterModel, targetModel Model, mapModel MapModel) bool {
	// Calculate eye position in CS2 coordinates
	eyeHeight := 64.0
	if shooter.IsCrouched {
		eyeHeight = 46.0
	}

	eyePos := r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y,
		Z: shooter.Position.Z + eyeHeight,
	}

	// Transform to model space for ray casting
	coords := NewDefaultSource2Coordinates()
	transformedEyePos := coords.CS2ToModelSpace(eyePos)
	transformedTargetPos := coords.CS2ToModelSpace(target.Position)

	slog.Info("Checking visibility from shooter to target",
		"shooter_position", shooter.Position,
		"target_position", target.Position,
		"transformed_eye", transformedEyePos,
		"transformed_target", transformedTargetPos)

	// Get the shooter's forward vector in model space
	shooterForward := coords.ApplyCS2Rotation(shooter.ViewAngleX, shooter.ViewAngleY, r3.Vector{X: 1, Y: 0, Z: 0})

	// Generate properly aligned sample points on the target player's body
	targetSamplePoints := generateTargetSamplePointsWithRotation(transformedTargetPos, shooterForward)

	// Generate multiple sample points on the target player's body to check visibility
	// This accounts for partial visibility cases where only part of the player is visible
	//targetSamplePoints := generateTargetSamplePoints(transformedTargetPos)
	//targetSamplePoints := generateTargetSamplePointsInModelSpace(transformedTargetPos)

	// Try each sample point until we find one that's visible
	for i, samplePoint := range targetSamplePoints {
		// Direction and distance to this sample point
		dirToSample := samplePoint.Sub(transformedEyePos).Normalize()
		distance := transformedEyePos.Sub(samplePoint).Norm()

		slog.Info("Checking sample point",
			"index", i,
			"position", samplePoint,
			"direction", dirToSample,
			"distance", distance)

		// Check for obstructions using ray casting
		blocked := checkRayWithTransparency(
			transformedEyePos,
			dirToSample,
			mapModel.BaseModel.bvh,
			distance*1.1, // Add 10% for safety
		)

		if !blocked {
			slog.Info("Target is visible via sample point", "index", i, "position", samplePoint)
			return true // Found a visible sample point
		}
	}

	// None of the sample points were visible
	slog.Info("Target is not visible - all sample points blocked")
	return false
}

func generateTargetSamplePointsWithRotation(targetPos r3.Vector, forward r3.Vector) []r3.Vector {
	// Standard player dimensions in model space
	const (
		playerHeight = 72.0
		playerWidth  = 32.0
	)

	// Calculate up and right vectors based on forward vector
	// If X is up in model space, we need to adjust the up vector
	up := r3.Vector{X: 1, Y: 0, Z: 0} // Assuming X is up in model space
	right := forward.Cross(up).Normalize()
	if right.Norm() < 0.01 {
		// If forward is parallel to up, use a different axis
		right = r3.Vector{X: 0, Y: 0, Z: 1}
	}
	// Recalculate up to ensure orthogonality
	up = right.Cross(forward).Normalize()

	// Generate points at different heights and positions
	points := []r3.Vector{
		// Center position (default)
		targetPos,

		// Head level (top)
		targetPos.Add(up.Mul(playerHeight * 0.85)),

		// Chest level (upper body)
		targetPos.Add(up.Mul(playerHeight * 0.65)),

		// Waist level (mid body)
		targetPos.Add(up.Mul(playerHeight * 0.45)),

		// Legs (lower body)
		targetPos.Add(up.Mul(playerHeight * 0.25)),

		// Add points with horizontal offsets using right vector
		// Right side at chest height
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(right.Mul(playerWidth * 0.35)),

		// Left side at chest height
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(right.Mul(playerWidth * 0.35)),

		// Front at chest height (using forward)
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(forward.Mul(playerWidth * 0.35)),

		// Back at chest height (using forward)
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(forward.Mul(playerWidth * 0.35)),

		// Add diagonal points
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(right.Mul(playerWidth * 0.3)).Add(forward.Mul(playerWidth * 0.3)),
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(right.Mul(playerWidth * 0.3)).Sub(forward.Mul(playerWidth * 0.3)),
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(right.Mul(playerWidth * 0.3)).Add(forward.Mul(playerWidth * 0.3)),
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(right.Mul(playerWidth * 0.3)).Sub(forward.Mul(playerWidth * 0.3)),
	}

	return points
}

func generateTargetSamplePointsInModelSpace(targetModelPos r3.Vector) []r3.Vector {
	// Standard player dimensions in CS2 units
	const (
		playerHeight = 72.0
		playerWidth  = 32.0
	)

	// Generate sample points with correct axis orientations
	// In model space: X is vertical, Y and Z are horizontal
	samplePoints := []r3.Vector{
		// Center position (base of model)
		targetModelPos,

		// Head level (top)
		{X: targetModelPos.X + playerHeight*0.85, Y: targetModelPos.Y, Z: targetModelPos.Z},

		// Chest level (upper body)
		{X: targetModelPos.X + playerHeight*0.65, Y: targetModelPos.Y, Z: targetModelPos.Z},

		// Waist level (mid body)
		{X: targetModelPos.X + playerHeight*0.45, Y: targetModelPos.Y, Z: targetModelPos.Z},

		// Legs (lower body)
		{X: targetModelPos.X + playerHeight*0.25, Y: targetModelPos.Y, Z: targetModelPos.Z},

		// Right side (at chest height)
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y, Z: targetModelPos.Z + playerWidth*0.35},

		// Left side (at chest height)
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y, Z: targetModelPos.Z - playerWidth*0.35},

		// Front (at chest height)
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y + playerWidth*0.35, Z: targetModelPos.Z},

		// Back (at chest height)
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y - playerWidth*0.35, Z: targetModelPos.Z},

		// Corners (diagonal offsets) at chest height
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y + playerWidth*0.3, Z: targetModelPos.Z + playerWidth*0.3},
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y - playerWidth*0.3, Z: targetModelPos.Z + playerWidth*0.3},
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y + playerWidth*0.3, Z: targetModelPos.Z - playerWidth*0.3},
		{X: targetModelPos.X + playerHeight*0.6, Y: targetModelPos.Y - playerWidth*0.3, Z: targetModelPos.Z - playerWidth*0.3},
	}

	return samplePoints
}

// generateTargetSamplePoints creates multiple points on the target player's body
// to check visibility from different angles. This is crucial for partial visibility.
func generateTargetSamplePoints(targetPos r3.Vector) []r3.Vector {
	// Standard player dimensions in CS2 units
	const (
		playerHeight = 72.0
		playerWidth  = 32.0
	)

	// Generate points at different heights and positions
	points := []r3.Vector{
		// Center position (default)
		targetPos,

		// Head level (top)
		{X: targetPos.X, Y: targetPos.Y, Z: targetPos.Z + playerHeight*0.85},

		// Chest level (upper body)
		{X: targetPos.X, Y: targetPos.Y, Z: targetPos.Z + playerHeight*0.65},

		// Waist level (mid body)
		{X: targetPos.X, Y: targetPos.Y, Z: targetPos.Z + playerHeight*0.45},

		// Legs (lower body)
		{X: targetPos.X, Y: targetPos.Y, Z: targetPos.Z + playerHeight*0.25},

		// Add points with horizontal offsets to catch side visibility
		// Right side
		{X: targetPos.X + playerWidth*0.35, Y: targetPos.Y, Z: targetPos.Z + playerHeight*0.6},
		// Left side
		{X: targetPos.X - playerWidth*0.35, Y: targetPos.Y, Z: targetPos.Z + playerHeight*0.6},
		// Front
		{X: targetPos.X, Y: targetPos.Y + playerWidth*0.35, Z: targetPos.Z + playerHeight*0.6},
		// Back
		{X: targetPos.X, Y: targetPos.Y - playerWidth*0.35, Z: targetPos.Z + playerHeight*0.6},

		// Corners (diagonal offsets) at chest height
		{X: targetPos.X + playerWidth*0.3, Y: targetPos.Y + playerWidth*0.3, Z: targetPos.Z + playerHeight*0.6},
		{X: targetPos.X + playerWidth*0.3, Y: targetPos.Y - playerWidth*0.3, Z: targetPos.Z + playerHeight*0.6},
		{X: targetPos.X - playerWidth*0.3, Y: targetPos.Y + playerWidth*0.3, Z: targetPos.Z + playerHeight*0.6},
		{X: targetPos.X - playerWidth*0.3, Y: targetPos.Y - playerWidth*0.3, Z: targetPos.Z + playerHeight*0.6},
	}

	return points
}

// checkRayWithTransparency checks if a ray is blocked by non-transparent objects
// checkRayWithTransparency checks if a ray is blocked by non-transparent objects
func checkRayWithTransparency(origin, direction r3.Vector, node *BVHNode, maxDistance float64) bool {
	if node == nil {
		return false // No node, no blocking
	}

	slog.Info("Checking ray with transparency",
		"origin", origin,
		"direction", direction,
		"maxDistance", maxDistance)

	// Initialize variables for tracking hit information
	currentOrigin := origin
	remainingDistance := maxDistance

	// Debug counters
	hitCount := 0
	transparentHitCount := 0

	// Names of materials hit during ray casting
	materialsHit := make([]string, 0)

	// Continue tracing the ray through transparent objects
	for {
		hitPos := r3.Vector{}
		blocked, material, triangleIndex := rayIntersectsBVHClosestHit(currentOrigin, direction, node, &hitPos, remainingDistance)

		hitCount++

		// If no hit or hit is beyond our range, we're done
		if !blocked {
			slog.Info("Ray passed through scene without hitting anything",
				"hitCount", hitCount,
				"transparentHits", transparentHitCount,
				"materialsHit", materialsHit)
			return false // No blocking
		}

		// Track materials hit
		materialsHit = append(materialsHit, material.Name)

		// Calculate hit distance for this intersection
		hitDistance := currentOrigin.Sub(hitPos).Norm()

		// Use a small epsilon (e.g. 0.1) to handle floating point comparisons
		if hitDistance >= maxDistance-0.1 {
			slog.Info("Hit is beyond or at target distance, considering target visible",
				"hitDistance", hitDistance,
				"maxDistance", maxDistance,
				"difference", maxDistance-hitDistance)
			return false // Not blocked - hit is beyond target
		}

		// Track the material's transparency status
		materialIsTransparent := material.IsTransparent || material.Opacity < 0.99

		// Special handling for the window bars - we know from the debug that this is a transparent material
		// This checks if the material name contains "fence" or "grate" in a case-insensitive way
		if strings.Contains(strings.ToLower(material.Name), "fence") ||
			strings.Contains(strings.ToLower(material.Name), "grate") {
			materialIsTransparent = true
		}

		if materialIsTransparent {
			transparentHitCount++
			slog.Info("Hit transparent material",
				"materialName", material.Name,
				"opacity", material.Opacity,
				"hitPos", hitPos,
				"hitDistance", hitDistance,
				"triangleIndex", triangleIndex)

			// Adjust the remaining distance and move the origin forward
			remainingDistance -= hitDistance
			if remainingDistance <= 0 {
				slog.Info("Reached maximum distance after transparent hits",
					"transparentHits", transparentHitCount,
					"hitCount", hitCount)
				return false // Reached maximum distance
			}

			// Move slightly beyond the hit point to avoid self-intersection
			const epsilon = 0.001
			currentOrigin = hitPos.Add(direction.Mul(epsilon))

			// Continue the loop to check for the next hit
			continue
		}

		// For curbs001 specifically, if we're close to the target, we might want to
		// consider it as not blocking the view as it could just be the ground
		// This is still general as it checks the distance to the target
		if (material.Name == "curbs001" || material.Name == "floor") && hitDistance > (maxDistance*0.9) {
			// We're very close to the target at this point, and hit what's likely the ground
			// It's possible the target is just standing on this surface
			slog.Info("Hit ground material near target, considering as non-blocking",
				"materialName", material.Name,
				"hitDistance", hitDistance,
				"maxDistance", maxDistance,
				"percentOfMax", (hitDistance/maxDistance)*100)
			return false
		}

		slog.Info("Hit opaque material, ray blocked",
			"materialName", material.Name,
			"opacity", material.Opacity,
			"hitPos", hitPos,
			"hitDistance", hitDistance,
			"distanceRatio", hitDistance/maxDistance,
			"totalHits", hitCount,
			"transparentHits", transparentHitCount,
			"triangleIndex", triangleIndex,
			"allMaterialsHit", materialsHit)

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
