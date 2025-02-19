package visibility

import (
	"math"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/stretchr/testify/assert"
)

// TestCoordinateSystemConsistency verifies that the coordinate system matches Source2's expectations
func TestCoordinateSystemConsistency(t *testing.T) {
	tests := []struct {
		name    string
		shooter types.PlayerTickData
		target  types.PlayerTickData
		wantFOV bool // whether target should be in shooter's FOV
		desc    string
	}{
		{
			name: "Target directly east (positive X)",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 90, // looking east
				ViewAngleY: 0,  // level view
				IsAlive:    true,
				IsCrouched: false,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 100, Y: 0, Z: 0},
				IsAlive:  true,
			},
			wantFOV: true,
			desc:    "When player faces east (90°), positive X targets should be in FOV",
		},
		{
			name: "Target directly north (positive Y)",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 0, // looking north
				ViewAngleY: 0, // level view
				IsAlive:    true,
				IsCrouched: false,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 0, Y: 100, Z: 0},
				IsAlive:  true,
			},
			wantFOV: true,
			desc:    "When player faces north (0°), positive Y targets should be in FOV",
		},
		{
			name: "Target directly up (positive Z)",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 0,   // looking north
				ViewAngleY: -90, // looking straight up
				IsAlive:    true,
				IsCrouched: false,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 0, Y: 0, Z: 100},
				IsAlive:  true,
			},
			wantFOV: true,
			desc:    "When player looks up (-90°), positive Z targets should be in FOV",
		},
	}

	// Create a simple player model for testing
	playerModel := createTestPlayerModel()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get eye position (this is important for coordinate system verification)
			eyePos := GetEyePosition(tt.shooter, playerModel)

			// Test if target is in FOV
			inFOV := tt.shooter.IsInFieldOfViewFromEye(tt.target.Position, eyePos)
			assert.Equal(t, tt.wantFOV, inFOV, tt.desc)

			// Verify coordinate transformations
			forward := tt.shooter.ForwardVector()
			validateForwardVector(t, float64(tt.shooter.ViewAngleX), float64(tt.shooter.ViewAngleY), forward)
		})
	}
}

// TestRotationConsistency ensures that rotation angles follow Source2 conventions
func TestRotationConsistency(t *testing.T) {
	tests := []struct {
		name       string
		viewAngleX float64 // yaw
		viewAngleY float64 // pitch
		wantDir    r3.Vector
	}{
		{
			name:       "Looking east",
			viewAngleX: 90,
			viewAngleY: 0,
			wantDir:    r3.Vector{X: 1, Y: 0, Z: 0},
		},
		{
			name:       "Looking north",
			viewAngleX: 0,
			viewAngleY: 0,
			wantDir:    r3.Vector{X: 0, Y: 1, Z: 0},
		},
		{
			name:       "Looking west",
			viewAngleX: -90,
			viewAngleY: 0,
			wantDir:    r3.Vector{X: -1, Y: 0, Z: 0},
		},
		{
			name:       "Looking south",
			viewAngleX: 180,
			viewAngleY: 0,
			wantDir:    r3.Vector{X: 0, Y: -1, Z: 0},
		},
		{
			name:       "Looking up",
			viewAngleX: 0,
			viewAngleY: -90,
			wantDir:    r3.Vector{X: 0, Y: 0, Z: 1},
		},
		{
			name:       "Looking down",
			viewAngleX: 0,
			viewAngleY: 90,
			wantDir:    r3.Vector{X: 0, Y: 0, Z: -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := types.PlayerTickData{
				ViewAngleX: float32(tt.viewAngleX),
				ViewAngleY: float32(tt.viewAngleY),
			}
			got := player.ForwardVector().Normalize()
			want := tt.wantDir.Normalize()

			assertVectorsEqual(t, want, got, 0.001)
		})
	}
}

// TestCanSeeTargetCoordinateSystem verifies that CanSeeTarget respects Source2's coordinate system
func TestCanSeeTargetCoordinateSystem(t *testing.T) {
	// Create test models
	playerModel := createTestPlayerModel()
	mapModel := createTestMapModel()

	tests := []struct {
		name    string
		shooter types.PlayerTickData
		target  types.PlayerTickData
		wantSee bool
		desc    string
	}{
		{
			name: "Clear line of sight along X axis",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 64}, // Standing height
				ViewAngleX: 90,                           // Looking east
				ViewAngleY: 0,
				IsAlive:    true,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 100, Y: 0, Z: 64},
				IsAlive:  true,
			},
			wantSee: true,
			desc:    "Should see target when looking east with no obstacles",
		},
		{
			name: "Clear line of sight along Y axis",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 64},
				ViewAngleX: 0, // Looking north
				ViewAngleY: 0,
				IsAlive:    true,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 0, Y: 100, Z: 64},
				IsAlive:  true,
			},
			wantSee: true,
			desc:    "Should see target when looking north with no obstacles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canSee, hitPoints, _ := CanSeeTarget(tt.shooter, tt.target, playerModel, mapModel, 0)
			assert.Equal(t, tt.wantSee, canSee, tt.desc)

			if !canSee {
				// Verify that any hit points respect the coordinate system
				for _, hitPoint := range hitPoints {
					validateCoordinatePoint(t, hitPoint)
				}
			}
		})
	}
}

// TestGetEyePositionHeight ensures eye height calculations match Source2 conventions
func TestGetEyePositionHeight(t *testing.T) {
	playerModel := createTestPlayerModel()

	tests := []struct {
		name   string
		player types.PlayerTickData
		wantZ  float64
		desc   string
	}{
		{
			name: "Standing player eye height",
			player: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				IsCrouched: false,
				IsAlive:    true,
			},
			wantZ: 64.0, // Standard CS2 standing eye height
			desc:  "Standing eye height should be approximately 64 units",
		},
		{
			name: "Crouching player eye height",
			player: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				IsCrouched: true,
				IsAlive:    true,
			},
			wantZ: 46.0, // Standard CS2 crouching eye height
			desc:  "Crouching eye height should be approximately 46 units",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eyePos := GetEyePosition(tt.player, playerModel)
			assert.InDelta(t, tt.wantZ, eyePos.Z, 1.0, tt.desc)
		})
	}
}

// Helper functions

func createTestPlayerModel() *Model {
	model := NewModel()
	// Set reasonable bounds for a player model
	model.min = r3.Vector{X: -16, Y: -16, Z: 0}
	model.max = r3.Vector{X: 16, Y: 16, Z: 72}

	// Add required visibility points
	model.visibilityPoints = []r3.Vector{
		{X: 0, Y: 0, Z: 64},   // Head height
		{X: 0, Y: 0, Z: 48},   // Chest height
		{X: 0, Y: 0, Z: 32},   // Waist height
		{X: -16, Y: 0, Z: 36}, // Left side
		{X: 16, Y: 0, Z: 36},  // Right side
		{X: 0, Y: -16, Z: 36}, // Front
		{X: 0, Y: 16, Z: 36},  // Back
	}

	return model
}

func createTestMapModel() *Model {
	model := NewModel()
	// Add some basic geometry for testing
	model.triangles = []types.Triangle{
		// Ground plane
		{
			V1: r3.Vector{X: -1000, Y: -1000, Z: 0},
			V2: r3.Vector{X: 1000, Y: -1000, Z: 0},
			V3: r3.Vector{X: 1000, Y: 1000, Z: 0},
		},
	}
	return model
}

func validateForwardVector(t *testing.T, yaw, pitch float64, got r3.Vector) {
	// Convert angles to radians
	yawRad := degToRad(yaw)
	pitchRad := degToRad(pitch)

	// Calculate expected forward vector based on Source2 conventions:
	// Yaw 0° points north (+Y), 90° points east (+X)
	// Pitch -90° points up (+Z), 90° points down (-Z)
	want := r3.Vector{
		// When looking east (90°), X should be 1
		X: math.Sin(yawRad) * math.Cos(pitchRad),
		// When looking north (0°), Y should be 1
		Y: math.Cos(yawRad) * math.Cos(pitchRad),
		// When looking up (-90°), Z should be 1
		Z: -math.Sin(pitchRad),
	}.Normalize()

	assertVectorsEqual(t, want.Normalize(), got.Normalize(), 0.001)
}

func validateCoordinatePoint(t *testing.T, point r3.Vector) {
	// Verify point follows Source2 coordinate system rules
	assert.False(t, math.IsNaN(point.X), "X coordinate should not be NaN")
	assert.False(t, math.IsNaN(point.Y), "Y coordinate should not be NaN")
	assert.False(t, math.IsNaN(point.Z), "Z coordinate should not be NaN")
}

func assertVectorsEqual(t *testing.T, want, got r3.Vector, tolerance float64) {
	assert.InDelta(t, want.X, got.X, tolerance, "X component mismatch")
	assert.InDelta(t, want.Y, got.Y, tolerance, "Y component mismatch")
	assert.InDelta(t, want.Z, got.Z, tolerance, "Z component mismatch")
}
