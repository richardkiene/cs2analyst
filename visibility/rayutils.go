// visibility/rayutils.go

package visibility

import (
	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// GetEyePosition calculates eye position consistently for both visualization and ray casting
func GetEyePosition(shooter types.PlayerTickData) r3.Vector {
	eyeHeight := 64.0 // Standing height
	if shooter.IsCrouched {
		eyeHeight = 46.0 // Crouching height
	}

	return r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y,
		Z: shooter.Position.Z + eyeHeight,
	}
}

// GenerateTargetSamplePoints creates sample points around a target position
// This function is used by both ray casting and visualization
func GenerateTargetSamplePoints(targetPos r3.Vector, forward r3.Vector) []r3.Vector {
	// Standard player dimensions in model space
	const (
		playerHeight = 72.0
		playerWidth  = 32.0
	)

	// Calculate up and right vectors based on forward vector
	// Assuming X is up in model space
	up := r3.Vector{X: 1, Y: 0, Z: 0}
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

// CastRayToTarget casts a ray from origin to target and checks for intersections
// This unified function can be used by both ray casting and visualization
func CastRayToTarget(origin, direction r3.Vector, node *BVHNode, maxDistance float64) (bool, r3.Vector, MaterialProperties) {
	hitPos := r3.Vector{}
	blocked, material, _ := rayIntersectsBVHClosestHit(origin, direction, node, &hitPos, maxDistance)

	return blocked, hitPos, material
}
