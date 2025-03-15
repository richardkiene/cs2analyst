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
		armLength    = 26.0
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

		// Head points - more detailed coverage
		targetPos.Add(up.Mul(playerHeight)),                            // Top of head
		targetPos.Add(up.Mul(playerHeight * 0.95)),                     // Upper head
		targetPos.Add(up.Mul(playerHeight * 0.90)),                     // Eye level
		targetPos.Add(up.Mul(playerHeight * 0.90)).Add(right.Mul(5)),   // Right side of head
		targetPos.Add(up.Mul(playerHeight * 0.90)).Sub(right.Mul(5)),   // Left side of head
		targetPos.Add(up.Mul(playerHeight * 0.90)).Add(forward.Mul(5)), // Front of head
		targetPos.Add(up.Mul(playerHeight * 0.90)).Sub(forward.Mul(5)), // Back of head

		// Neck area
		targetPos.Add(up.Mul(playerHeight * 0.70)),

		// Shoulder points
		targetPos.Add(up.Mul(playerHeight * 0.65)).Add(right.Mul(playerWidth * 0.4)), // Right shoulder
		targetPos.Add(up.Mul(playerHeight * 0.65)).Sub(right.Mul(playerWidth * 0.4)), // Left shoulder

		// Upper chest
		targetPos.Add(up.Mul(playerHeight * 0.6)),
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(right.Mul(playerWidth * 0.25)),
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(right.Mul(playerWidth * 0.25)),

		// Mid chest level (upper body)
		targetPos.Add(up.Mul(playerHeight * 0.55)),
		targetPos.Add(up.Mul(playerHeight * 0.55)).Add(right.Mul(playerWidth * 0.35)),
		targetPos.Add(up.Mul(playerHeight * 0.55)).Sub(right.Mul(playerWidth * 0.35)),

		// Lower chest
		targetPos.Add(up.Mul(playerHeight * 0.5)),

		// Waist level (mid body)
		targetPos.Add(up.Mul(playerHeight * 0.45)),
		targetPos.Add(up.Mul(playerHeight * 0.45)).Add(right.Mul(playerWidth * 0.35)),
		targetPos.Add(up.Mul(playerHeight * 0.45)).Sub(right.Mul(playerWidth * 0.35)),

		// Hips
		targetPos.Add(up.Mul(playerHeight * 0.4)),

		// Upper legs (lower body)
		targetPos.Add(up.Mul(playerHeight * 0.35)),
		targetPos.Add(up.Mul(playerHeight * 0.35)).Add(right.Mul(playerWidth * 0.2)),
		targetPos.Add(up.Mul(playerHeight * 0.35)).Sub(right.Mul(playerWidth * 0.2)),

		// Mid legs
		targetPos.Add(up.Mul(playerHeight * 0.25)),

		// Lower legs
		targetPos.Add(up.Mul(playerHeight * 0.15)),

		// Feet
		targetPos.Add(up.Mul(playerHeight * 0.05)),

		// Arms - Extended right arm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Add(right.Mul(playerWidth*0.4 + armLength*0.25)), // Right upper arm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Add(right.Mul(playerWidth*0.4 + armLength*0.5)),  // Right mid arm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Add(right.Mul(playerWidth*0.4 + armLength*0.75)), // Right forearm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Add(right.Mul(playerWidth*0.4 + armLength)),      // Right hand

		// Arms - Extended left arm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Sub(right.Mul(playerWidth*0.4 + armLength*0.25)), // Left upper arm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Sub(right.Mul(playerWidth*0.4 + armLength*0.5)),  // Left mid arm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Sub(right.Mul(playerWidth*0.4 + armLength*0.75)), // Left forearm
		targetPos.Add(up.Mul(playerHeight * 0.65)).Sub(right.Mul(playerWidth*0.4 + armLength)),      // Left hand

		// Gun position (typically held in front)
		targetPos.Add(up.Mul(playerHeight * 0.55)).Add(forward.Mul(playerWidth * 0.6)),

		// Front, back, and sides at key heights
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(forward.Mul(playerWidth * 0.35)),  // Front at chest height
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(forward.Mul(playerWidth * 0.35)),  // Back at chest height
		targetPos.Add(up.Mul(playerHeight * 0.45)).Add(forward.Mul(playerWidth * 0.35)), // Front at waist
		targetPos.Add(up.Mul(playerHeight * 0.45)).Sub(forward.Mul(playerWidth * 0.35)), // Back at waist

		// Diagonal points at key heights
		// Chest level diagonals
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(right.Mul(playerWidth * 0.3)).Add(forward.Mul(playerWidth * 0.3)), // Front-right
		targetPos.Add(up.Mul(playerHeight * 0.6)).Add(right.Mul(playerWidth * 0.3)).Sub(forward.Mul(playerWidth * 0.3)), // Back-right
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(right.Mul(playerWidth * 0.3)).Add(forward.Mul(playerWidth * 0.3)), // Front-left
		targetPos.Add(up.Mul(playerHeight * 0.6)).Sub(right.Mul(playerWidth * 0.3)).Sub(forward.Mul(playerWidth * 0.3)), // Back-left

		// Waist level diagonals
		targetPos.Add(up.Mul(playerHeight * 0.45)).Add(right.Mul(playerWidth * 0.3)).Add(forward.Mul(playerWidth * 0.3)), // Front-right
		targetPos.Add(up.Mul(playerHeight * 0.45)).Add(right.Mul(playerWidth * 0.3)).Sub(forward.Mul(playerWidth * 0.3)), // Back-right
		targetPos.Add(up.Mul(playerHeight * 0.45)).Sub(right.Mul(playerWidth * 0.3)).Add(forward.Mul(playerWidth * 0.3)), // Front-left
		targetPos.Add(up.Mul(playerHeight * 0.45)).Sub(right.Mul(playerWidth * 0.3)).Sub(forward.Mul(playerWidth * 0.3)), // Back-left

		// Head level diagonals
		targetPos.Add(up.Mul(playerHeight * 0.9)).Add(right.Mul(4)).Add(forward.Mul(4)), // Front-right of head
		targetPos.Add(up.Mul(playerHeight * 0.9)).Add(right.Mul(4)).Sub(forward.Mul(4)), // Back-right of head
		targetPos.Add(up.Mul(playerHeight * 0.9)).Sub(right.Mul(4)).Add(forward.Mul(4)), // Front-left of head
		targetPos.Add(up.Mul(playerHeight * 0.9)).Sub(right.Mul(4)).Sub(forward.Mul(4)), // Back-left of head
	}

	return points
}

// CastRayToTarget casts a ray from origin to target and checks for intersections
// This unified function can be used by both ray casting and visualization
func CastRayToTarget(origin, direction r3.Vector, node *BVHNode, maxDistance float64, debugEnabled bool) (bool, r3.Vector, MaterialProperties) {
	hitPos := r3.Vector{}
	blocked, material, _ := rayIntersectsBVHClosestHit(origin, direction, node, &hitPos, maxDistance, debugEnabled)

	return blocked, hitPos, material
}
