package visibility

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/stretchr/testify/assert"
)

// parseObjSections splits an OBJ file into sections keyed by object name (lines beginning with "o ").
func parseObjSections(obj string) map[string][]r3.Vector {
	sections := make(map[string][]r3.Vector)
	var currentSection string
	scanner := bufio.NewScanner(strings.NewReader(obj))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "o ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentSection = parts[1]
				sections[currentSection] = []r3.Vector{}
			}
		} else if strings.HasPrefix(line, "v ") && currentSection != "" {
			parts := strings.Fields(line)
			if len(parts) < 4 {
				continue
			}
			x, _ := strconv.ParseFloat(parts[1], 64)
			y, _ := strconv.ParseFloat(parts[2], 64)
			z, _ := strconv.ParseFloat(parts[3], 64)
			sections[currentSection] = append(sections[currentSection], r3.Vector{X: x, Y: y, Z: z})
		}
	}
	return sections
}

// computeDirectionForCone assumes that in the FOV cone section the first vertex is the apex
// and the next four vertices are the far-plane corners in this order: TR, TL, BR, BL
func computeDirectionForCone(verts []r3.Vector) r3.Vector {
	if len(verts) < 5 {
		return r3.Vector{}
	}
	apex := verts[0]

	// Instead of averaging all corners, take the center point of the far plane
	// by averaging diagonally opposite corners - this gives us the true center
	farCenter := verts[1].Add(verts[4]).Mul(0.5) // TR + BL / 2

	// The direction from apex to far center is our true forward vector
	return farCenter.Sub(apex).Normalize()
}

// computeDirectionForRay expects exactly two vertices.
func computeDirectionForRay(verts []r3.Vector) r3.Vector {
	if len(verts) < 2 {
		return r3.Vector{}
	}
	return verts[1].Sub(verts[0]).Normalize()
}

// computeCentroidDirection returns the normalized vector from the origin to the centroid of the vertices.
func computeCentroidDirection(verts []r3.Vector) r3.Vector {
	if len(verts) == 0 {
		return r3.Vector{}
	}
	var sum r3.Vector
	for _, v := range verts {
		sum = sum.Add(v)
	}
	centroid := sum.Mul(1.0 / float64(len(verts)))
	return centroid.Normalize()
}

func TestObjFileDirectionConsistency(t *testing.T) {
	// Use actual game coordinates from debug output
	shooter := types.PlayerTickData{
		Position:   r3.Vector{X: -1165.968, Y: 578.252, Z: -79.969},
		ViewAngleX: 0.17,
		ViewAngleY: 0.22,
		IsAlive:    true,
	}

	target := types.PlayerTickData{
		Position: r3.Vector{X: -438.731, Y: 591.773, Z: -80.431},
		IsAlive:  true,
	}

	mapModel := CreateTestPlayerModel()
	playerModel := CreateTestPlayerModel()

	eyePos := GetEyePosition(shooter)

	// Get both forward vector and vector to target for comparison
	forward := shooter.ForwardVector()
	toTarget := target.Position.Sub(shooter.Position).Normalize()

	t.Logf("Shooter forward vector: %+v", forward)
	t.Logf("Eye position: %+v", eyePos)
	t.Logf("Vector to target: %+v", toTarget)

	// Add a debug print to see what direction we're passing
	err := CreateShooterCentricFOVUsingTargetDistance(
		84144,
		mapModel,
		playerModel,
		shooter,
		target,
		0.0,
		true,
		nil,
		nil,
		false,
	)
	assert.NoError(t, err)

	data, err := os.ReadFile("debug_visibility/shooter_centric_fov_tick_84144.obj")
	assert.NoError(t, err)

	sections := parseObjSections(string(data))

	// We expect the FOV cone to point towards the target, not in the forward direction
	expected := toTarget

	if coneVerts, ok := sections["fov_cone"]; ok && len(coneVerts) >= 5 {
		coneDir := computeDirectionForCone(coneVerts)
		t.Logf("FOV Cone Direction: %+v", coneDir)
		assertVectorsEqual(t, expected, coneDir, 0.05, "FOV cone direction")
	}

	if rayVerts, ok := sections["debug_ray"]; ok && len(rayVerts) >= 2 {
		rayDir := computeDirectionForRay(rayVerts)
		t.Logf("Debug Ray Direction: %+v", rayDir)
		t.Logf("Ray start: %+v", rayVerts[0])
		t.Logf("Ray end: %+v", rayVerts[1])
		assertVectorsEqual(t, expected, rayDir, 0.05, "Debug ray direction")
	}
}

func TestFOVDirectionConsistency(t *testing.T) {
	// Test cases that pair positions with expected directions
	tests := []struct {
		name    string
		shooter types.PlayerTickData
		target  types.PlayerTickData
		// We specify both forward and toTarget to ensure they're different
		// This helps catch cases where we accidentally use the wrong one
		wantForward  r3.Vector // What direction the player is facing
		wantToTarget r3.Vector // Direction from shooter to target
		useForward   bool      // Changed from shouldMatch string to bool for clarity
	}{
		{
			name: "Real gameplay scenario - FOV should match target direction",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: -1165.968, Y: 578.252, Z: -79.969},
				ViewAngleX: 0.17,
				ViewAngleY: 0.22,
				IsAlive:    true,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -438.731, Y: 591.773, Z: -80.431},
				IsAlive:  true,
			},
			wantForward:  r3.Vector{X: 0.002967, Y: 0.999988, Z: -0.003840}, // Nearly pure Y (north)
			wantToTarget: r3.Vector{X: 0.999827, Y: 0.018589, Z: -0.000635}, // Nearly pure X (east)
			useForward:   false,
		},
		{
			name: "Simple east-facing scenario - FOV should match forward",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 90, // Looking east
				ViewAngleY: 0,
				IsAlive:    true,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 0, Y: 100, Z: 0}, // Target to the north
				IsAlive:  true,
			},
			wantForward:  r3.Vector{X: 1, Y: 0, Z: 0}, // East
			wantToTarget: r3.Vector{X: 0, Y: 1, Z: 0}, // North
			useForward:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapModel := CreateTestPlayerModel()
			playerModel := CreateTestPlayerModel()

			// Generate the debug visualization
			err := CreateShooterCentricFOVUsingTargetDistance(
				84144,
				mapModel,
				playerModel,
				tt.shooter,
				tt.target,
				0.0,
				true,
				nil,
				nil,
				tt.useForward,
			)
			assert.NoError(t, err)

			// Read and parse the OBJ file
			data, err := os.ReadFile("debug_visibility/shooter_centric_fov_tick_84144.obj")
			assert.NoError(t, err)

			sections := parseObjSections(string(data))

			// Get actual forward and toTarget vectors
			actualForward := tt.shooter.ForwardVector()
			actualToTarget := tt.target.Position.Sub(tt.shooter.Position).Normalize()

			// Verify these match their expected values
			assertVectorsEqual(t, tt.wantForward, actualForward, 0.05, "Forward vector incorrect")
			assertVectorsEqual(t, tt.wantToTarget, actualToTarget, 0.05, "ToTarget vector incorrect")

			// Get cone direction from OBJ
			coneVerts, ok := sections["fov_cone"]
			assert.True(t, ok, "OBJ must contain fov_cone section")
			coneDir := computeDirectionForCone(coneVerts)

			// Get ray direction from OBJ
			rayVerts, ok := sections["debug_ray"]
			assert.True(t, ok, "OBJ must contain debug_ray section")
			rayDir := computeDirectionForRay(rayVerts)

			// Check that cone and ray match the expected direction
			expected := tt.wantToTarget
			if tt.useForward {
				expected = tt.wantForward
			}
			// Verify both cone and ray match the expected direction
			assertVectorsEqual(t, expected, coneDir, 0.05, "FOV cone direction mismatch")
			assertVectorsEqual(t, expected, rayDir, 0.05, "Debug ray direction mismatch")

			// Explicitly verify they do NOT match the wrong direction
			wrongExpected := tt.wantForward
			if tt.useForward {
				wrongExpected = tt.wantToTarget
			}

			// These should fail if cone matches wrong direction
			assert.False(t, vectorsNearlyEqual(coneDir, wrongExpected, 0.05),
				"FOV cone matched wrong direction")
			assert.False(t, vectorsNearlyEqual(rayDir, wrongExpected, 0.05),
				"Debug ray matched wrong direction")
		})
	}
}

// Helper function to check if vectors are nearly equal
func vectorsNearlyEqual(a, b r3.Vector, tolerance float64) bool {
	return math.Abs(a.X-b.X) < tolerance &&
		math.Abs(a.Y-b.Y) < tolerance &&
		math.Abs(a.Z-b.Z) < tolerance
}
