package types

import (
	"math"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/stretchr/testify/assert"
)

// Test that ForwardVector returns the expected direction in Source2.
// For example, if ViewAngleX=90 and ViewAngleY=0, then we expect forward = (1,0,0).
func TestForwardVectorEast(t *testing.T) {
	shooter := PlayerTickData{
		ViewAngleX: 90,
		ViewAngleY: 0,
	}
	got := shooter.ForwardVector()
	want := r3.Vector{X: 1, Y: 0, Z: 0}
	assertVectorsEqual(t, want, got, 0.0001, "Forward vector for shooter looking east")
}

// Test that when the shooter is looking north (ViewAngleX=0, ViewAngleY=0) the right vector is computed correctly.
// In Source2, if forward is (0,1,0) then right should be (1,0,0).
func TestRightVectorConsistency(t *testing.T) {
	shooter := PlayerTickData{
		ViewAngleX: 0,
		ViewAngleY: 0,
	}
	forward := shooter.ForwardVector() // Expected: (0,1,0)
	up := r3.Vector{X: 0, Y: 0, Z: 1}  // Global up in Source2
	right := forward.Cross(up).Normalize()
	expectedRight := r3.Vector{X: 1, Y: 0, Z: 0}
	assertVectorsEqual(t, expectedRight, right, 0.0001, "Right vector when shooter looks north")
}

func TestRightVectorCalculation(t *testing.T) {
	// Shooter looking north: (ViewAngleX=0° means facing north (+Y)).
	shooter := PlayerTickData{
		ViewAngleX: 0,
		ViewAngleY: 0,
	}
	forward := shooter.ForwardVector() // expected to be (0, 1, 0)
	up := r3.Vector{X: 0, Y: 0, Z: 1}  // global up in Source2
	right := forward.Cross(up).Normalize()
	// In Source2: if forward is (0,1,0) then right should be (1,0,0)
	expectedRight := r3.Vector{X: 1, Y: 0, Z: 0}
	assertVectorsEqual(t, expectedRight, right, 0.0001, "Right vector for shooter looking north")
}

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

// Helper functions

func assertVectorsEqual(t *testing.T, want, got r3.Vector, tolerance float64, msg string) {
	assert.InDelta(t, want.X, got.X, tolerance, msg+" (X component)")
	assert.InDelta(t, want.Y, got.Y, tolerance, msg+" (Y component)")
	assert.InDelta(t, want.Z, got.Z, tolerance, msg+" (Z component)")
}
