package types

import (
	"math"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/stretchr/testify/assert"
)

// TestForwardVectorCoordinateSystem verifies that ForwardVector follows Source2 conventions
func TestForwardVectorCoordinateSystem(t *testing.T) {
	tests := []struct {
		name       string
		viewAngleX float32 // yaw (rotation around Z)
		viewAngleY float32 // pitch (rotation around Y)
		want       r3.Vector
		desc       string
	}{
		{
			name:       "Looking east (positive X)",
			viewAngleX: 90,
			viewAngleY: 0,
			want:       r3.Vector{X: 1, Y: 0, Z: 0}, // East is +X
			desc:       "When looking east (90°), forward vector should point along positive X",
		},
		{
			name:       "Looking north (positive Y)",
			viewAngleX: 0,
			viewAngleY: 0,
			want:       r3.Vector{X: 0, Y: 1, Z: 0}, // North is +Y
			desc:       "When looking north (0°), forward vector should point along positive Y",
		},
		{
			name:       "Looking west (negative X)",
			viewAngleX: -90,
			viewAngleY: 0,
			want:       r3.Vector{X: -1, Y: 0, Z: 0}, // West is -X
			desc:       "When looking west (-90°), forward vector should point along negative X",
		},
		{
			name:       "Looking south (negative Y)",
			viewAngleX: 180,
			viewAngleY: 0,
			want:       r3.Vector{X: 0, Y: -1, Z: 0}, // South is -Y
			desc:       "When looking south (180°), forward vector should point along negative Y",
		},
		{
			name:       "Looking northeast (45 degrees)",
			viewAngleX: 45,
			viewAngleY: 0,
			want: r3.Vector{
				X: 1 / math.Sqrt(2), // Equal X and Y components
				Y: 1 / math.Sqrt(2),
				Z: 0,
			},
			desc: "45° between north and east should have equal X and Y components",
		},
		{
			name:       "Looking up",
			viewAngleX: 0,
			viewAngleY: -90,
			want:       r3.Vector{X: 0, Y: 0, Z: 1}, // Up is +Z
			desc:       "Looking up (-90° pitch) should point along positive Z",
		},
		{
			name:       "Looking down",
			viewAngleX: 0,
			viewAngleY: 90,
			want:       r3.Vector{X: 0, Y: 0, Z: -1}, // Down is -Z
			desc:       "Looking down (90° pitch) should point along negative Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := PlayerTickData{
				ViewAngleX: tt.viewAngleX,
				ViewAngleY: tt.viewAngleY,
			}
			got := player.ForwardVector()
			assertVectorsEqual(t, tt.want, got, 0.0001, tt.desc)

			// Additional validation for right-handed coordinate system
			if tt.viewAngleX == 0 && tt.viewAngleY == 0 {
				// When looking north (+Y), right should be east (+X)
				right := got.Cross(r3.Vector{X: 0, Y: 0, Z: 1})
				assertVectorsEqual(t, r3.Vector{X: 1, Y: 0, Z: 0}, right, 0.0001, "Right vector should point east")
			}
		})
	}
}

// TestFieldOfViewCoordinateSystem verifies that FOV calculations respect Source2 coordinates
func TestFieldOfViewCoordinateSystem(t *testing.T) {
	standardEyeHeight := 64.0
	tests := []struct {
		name    string
		player  PlayerTickData
		target  r3.Vector
		eyePos  r3.Vector
		wantFOV bool
		desc    string
	}{
		{
			name: "Target east of player (positive X)",
			player: PlayerTickData{
				ViewAngleX: 90, // Looking east
				ViewAngleY: 0,
			},
			target:  r3.Vector{X: 100, Y: 0, Z: standardEyeHeight},
			eyePos:  r3.Vector{X: 0, Y: 0, Z: standardEyeHeight},
			wantFOV: true,
			desc:    "Target to the east should be in FOV when looking east",
		},
		{
			name: "Target north of player (positive Y)",
			player: PlayerTickData{
				ViewAngleX: 0, // Looking north
				ViewAngleY: 0,
			},
			target:  r3.Vector{X: 0, Y: 100, Z: standardEyeHeight},
			eyePos:  r3.Vector{X: 0, Y: 0, Z: standardEyeHeight},
			wantFOV: true,
			desc:    "Target to the north should be in FOV when looking north",
		},
		{
			name: "Target northeast of player",
			player: PlayerTickData{
				ViewAngleX: 45, // Looking northeast
				ViewAngleY: 0,
			},
			target:  r3.Vector{X: 70.71, Y: 70.71, Z: standardEyeHeight}, // 45° position
			eyePos:  r3.Vector{X: 0, Y: 0, Z: standardEyeHeight},
			wantFOV: true,
			desc:    "Target at 45° should be in FOV when looking northeast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.player.IsInFieldOfViewFromEye(tt.target, tt.eyePos)
			assert.Equal(t, tt.wantFOV, got, tt.desc)
		})
	}
}

func TestIsInFieldOfViewFromEye(t *testing.T) {
	tests := []struct {
		name        string
		player      PlayerTickData
		target      r3.Vector
		eyePos      r3.Vector
		wantInFOV   bool
		description string
	}{
		{
			name: "Target directly east",
			player: PlayerTickData{
				ViewAngleX: 90, // Looking east
				ViewAngleY: 0,  // Level
			},
			target:      r3.Vector{X: 100, Y: 0, Z: 64},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   true,
			description: "When looking east, target to the east should be in FOV",
		},
		{
			name: "Target directly north",
			player: PlayerTickData{
				ViewAngleX: 0, // Looking north
				ViewAngleY: 0, // Level
			},
			target:      r3.Vector{X: 0, Y: 100, Z: 64},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   true,
			description: "When looking north, target to the north should be in FOV",
		},
		{
			name: "Target directly up",
			player: PlayerTickData{
				ViewAngleX: 0,   // Looking north
				ViewAngleY: -90, // Looking up
			},
			target:      r3.Vector{X: 0, Y: 0, Z: 164},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   true,
			description: "When looking up, target above should be in FOV",
		},
		{
			name: "Target behind player",
			player: PlayerTickData{
				ViewAngleX: 0, // Looking north
				ViewAngleY: 0, // Level
			},
			target:      r3.Vector{X: 0, Y: -100, Z: 64},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   false,
			description: "Target behind player should not be in FOV",
		},
		{
			name: "Target at 45 degrees horizontal",
			player: PlayerTickData{
				ViewAngleX: 0, // Looking north
				ViewAngleY: 0, // Level
			},
			target:      r3.Vector{X: 100, Y: 100, Z: 64},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   true,
			description: "Target at 45 degrees should be in FOV (within 90 degree horizontal FOV)",
		},
		{
			name: "Target at 45 degrees vertical",
			player: PlayerTickData{
				ViewAngleX: 0,   // Looking north
				ViewAngleY: -45, // Looking up at 45 degrees
			},
			target:      r3.Vector{X: 0, Y: 100, Z: 164},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   true,
			description: "Target at 45 degrees up should be in FOV (within 74 degree vertical FOV)",
		},
		{
			name: "Target just outside horizontal FOV",
			player: PlayerTickData{
				ViewAngleX: 0, // Looking north
				ViewAngleY: 0, // Level
			},
			target:      r3.Vector{X: 100, Y: -10, Z: 64},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   false,
			description: "Target just outside 90 degree horizontal FOV should not be visible",
		},
		{
			name: "Target just outside vertical FOV",
			player: PlayerTickData{
				ViewAngleX: 0, // Looking north
				ViewAngleY: 0, // Level
			},
			target:      r3.Vector{X: 0, Y: 100, Z: 264},
			eyePos:      r3.Vector{X: 0, Y: 0, Z: 64},
			wantInFOV:   false,
			description: "Target just outside 74 degree vertical FOV should not be visible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.player.IsInFieldOfViewFromEye(tt.target, tt.eyePos)
			assert.Equal(t, tt.wantInFOV, got, tt.description)
		})
	}
}

// Helper functions

func assertVectorsEqual(t *testing.T, want, got r3.Vector, tolerance float64, msg string) {
	// Normalize both vectors for comparison
	want = want.Normalize()
	got = got.Normalize()

	assert.InDelta(t, want.X, got.X, tolerance, msg+" (X component)")
	assert.InDelta(t, want.Y, got.Y, tolerance, msg+" (Y component)")
	assert.InDelta(t, want.Z, got.Z, tolerance, msg+" (Z component)")
}

func degToRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}
