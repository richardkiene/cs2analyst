package visibility

import (
	"math"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/stretchr/testify/assert"
)

// TestRayFOVConeConsistency verifies that the direction from the shooter's eye
// (using GetEyePosition) plus the forward direction (from ForwardVector)
// produces the same ray direction as is used for the FOV cone.
// For example, if the shooter is facing east (ViewAngleX=90, ViewAngleY=0),
// then ForwardVector() should return approximately (1,0,0) and the ray from
// eyePos to (eyePos + forward*distance) should be (1,0,0).
func TestRayFOVConeConsistency(t *testing.T) {
	// Create a shooter at the origin who is facing east.
	shooter := types.PlayerTickData{
		Position:   r3.Vector{X: 0, Y: 0, Z: 0},
		ViewAngleX: 90, // Facing east, so expected forward = (1,0,0)
		ViewAngleY: 0,
		IsAlive:    true,
	}
	model := CreateTestPlayerModel()
	eyePos := GetEyePosition(shooter, model)
	forward := shooter.ForwardVector() // expected (1,0,0)

	// Compute a debug ray: from eyePos to eyePos + forward * distance.
	const distance = 100.0
	rayEnd := eyePos.Add(forward.Mul(distance))
	rayDir := rayEnd.Sub(eyePos).Normalize()

	// Our expectation is that the forward direction is (1,0,0).
	expected := r3.Vector{X: 1, Y: 0, Z: 0}
	tol := 0.01
	assert.InDelta(t, expected.X, rayDir.X, tol, "Ray direction X")
	assert.InDelta(t, expected.Y, rayDir.Y, tol, "Ray direction Y")
	assert.InDelta(t, expected.Z, rayDir.Z, tol, "Ray direction Z")

	// --- Now simulate the FOV cone computation ---
	// In WriteFOVCone, the cone is built by taking the eyePos and adding forward * CONE_LENGTH.
	// (In our exported OBJ, the cone is written as vertices with the first vertex at the eye
	// and the next four vertices approximating the far plane.)
	// Here, we compute the cone direction as the normalized vector from the eye to the average
	// of the four far-plane vertices.
	const (
		HORIZONTAL_FOV = 90.0
		VERTICAL_FOV   = 74.0
		CONE_LENGTH    = 200.0
	)
	hFovRad := (HORIZONTAL_FOV / 2.0) * (math.Pi / 180.0)
	vFovRad := (VERTICAL_FOV / 2.0) * (math.Pi / 180.0)
	baseWidth := CONE_LENGTH * math.Tan(hFovRad)
	baseHeight := CONE_LENGTH * math.Tan(vFovRad)

	// Compute the far-plane point.
	endPoint := eyePos.Add(forward.Mul(CONE_LENGTH))
	// Compute right and up vectors in Source2 (without extra rotations).
	var up r3.Vector
	if math.Abs(forward.Z) > 0.99 {
		if forward.Z > 0 {
			up = r3.Vector{X: 0, Y: 1, Z: 0}
		} else {
			up = r3.Vector{X: 0, Y: -1, Z: 0}
		}
	} else {
		up = r3.Vector{X: 0, Y: 0, Z: 1}
	}
	right := forward.Cross(up).Normalize()
	up = right.Cross(forward).Normalize()

	topRight := endPoint.Add(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	topLeft := endPoint.Sub(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	bottomRight := endPoint.Add(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))
	bottomLeft := endPoint.Sub(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))

	// Compute the average of the far-plane vertices.
	avgFar := topRight.Add(topLeft).Add(bottomRight).Add(bottomLeft).Mul(0.25)
	coneDir := avgFar.Sub(eyePos).Normalize()

	// Compare the coneDir to our expected forward.
	assert.InDelta(t, expected.X, coneDir.X, tol, "FOV cone direction X")
	assert.InDelta(t, expected.Y, coneDir.Y, tol, "FOV cone direction Y")
	assert.InDelta(t, expected.Z, coneDir.Z, tol, "FOV cone direction Z")
}

// TestMapGeometryOrientation creates a dummy map triangle that is in front of the shooter
// and verifies that its centroid direction is along the expected forward axis.
func TestMapGeometryOrientation(t *testing.T) {
	// Shooter at origin, facing east (i.e. forward = (1,0,0) in Source2 if ViewAngleX=90).
	shooter := types.PlayerTickData{
		Position:   r3.Vector{X: 0, Y: 0, Z: 0},
		ViewAngleX: 90,
		ViewAngleY: 0,
		IsAlive:    true,
	}
	// Create a dummy map triangle all with X=210.
	tri := types.Triangle{
		V1: r3.Vector{X: 210, Y: -10, Z: 0},
		V2: r3.Vector{X: 210, Y: 10, Z: 0},
		V3: r3.Vector{X: 210, Y: 0, Z: 0},
	}
	// Its centroid should be (210,0,0) so the direction from shooter is (1,0,0).
	centroid := tri.V1.Add(tri.V2).Add(tri.V3).Mul(1.0 / 3.0)
	dir := centroid.Sub(shooter.Position).Normalize()
	expected := r3.Vector{X: 1, Y: 0, Z: 0}
	tol := 0.01
	assert.InDelta(t, expected.X, dir.X, tol, "Map triangle direction X")
	assert.InDelta(t, expected.Y, dir.Y, tol, "Map triangle direction Y")
	assert.InDelta(t, expected.Z, dir.Z, tol, "Map triangle direction Z")
}

// TestVisibilityFOVConsistency verifies that IsInFieldOfViewFromEye uses the same forward vector.
// For a shooter facing east (ViewAngleX=90, ViewAngleY=0), a target placed at (100, 0, 0) should be in FOV.
func TestVisibilityFOVConsistency(t *testing.T) {
	shooter := types.PlayerTickData{
		Position:   r3.Vector{X: 0, Y: 0, Z: 0},
		ViewAngleX: 90, // Facing east; forward should be (1,0,0)
		ViewAngleY: 0,
		IsAlive:    true,
	}
	target := types.PlayerTickData{
		Position: r3.Vector{X: 100, Y: 0, Z: 0},
		IsAlive:  true,
	}
	playerModel := CreateTestPlayerModel()
	eyePos := GetEyePosition(shooter, playerModel)
	assert.True(t, shooter.IsInFieldOfViewFromEye(target.Position, eyePos), "Target directly in front should be in FOV")
}

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
	playerModel := CreateTestPlayerModel()

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

			assertVectorsEqual(t, want, got, 0.001, "TestRotationConsistency")
		})
	}
}

// TestCanSeeTargetCoordinateSystem verifies that CanSeeTarget respects Source2's coordinate system
func TestCanSeeTargetCoordinateSystem(t *testing.T) {
	// Create test models
	playerModel := CreateTestPlayerModel()
	mapModel := createVisTestMapModel()

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
	playerModel := CreateTestPlayerModel()

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

// TestEyeDirectionConsistency verifies that the horizontal direction from the shooter’s feet to the eye
// matches the horizontal component of the shooter’s ForwardVector() when there is zero pitch.
func TestEyeDirectionConsistency(t *testing.T) {
	// Create a model where the eye height is perfectly above the feet
	model := createVisTestMapModel()
	model.BaseModel.min = r3.Vector{X: -16, Y: -16, Z: 0}
	model.BaseModel.max = r3.Vector{X: 16, Y: 16, Z: 72}

	// Create a shooter at the origin with no pitch (level view) and facing east
	shooter := types.PlayerTickData{
		Position:   r3.Vector{X: 0, Y: 0, Z: 0},
		ViewAngleX: 90, // Looking east
		ViewAngleY: 0,  // Level view
		IsAlive:    true,
	}

	eyePos := GetEyePosition(shooter, &model.BaseModel)

	// Since GetEyePosition only adds Z height and doesn't affect X/Y,
	// the horizontal components of the eye offset should be zero
	offset := eyePos.Sub(shooter.Position)
	if offset.X != 0 || offset.Y != 0 {
		t.Errorf("Eye position should be directly above feet, got offset X=%.6f, Y=%.6f",
			offset.X, offset.Y)
	}

	// Get the shooter's forward vector and verify it points east
	forward := shooter.ForwardVector()
	expectedForward := r3.Vector{X: 1, Y: 0, Z: 0} // East is +X

	tol := 0.0001
	if math.Abs(forward.X-expectedForward.X) > tol ||
		math.Abs(forward.Y-expectedForward.Y) > tol ||
		math.Abs(forward.Z-expectedForward.Z) > tol {
		t.Errorf("Forward vector should point east (1,0,0), got (%.6f, %.6f, %.6f)",
			forward.X, forward.Y, forward.Z)
	}
}

// Helper functions

// CreateTestPlayerModel returns a dummy player model with bounds chosen so that
// GetEyePosition returns a standard eye height (e.g. ~64 units).
func CreateTestPlayerModel() *Model {
	model := NewModel()
	model.min = r3.Vector{X: -16, Y: -16, Z: 0}
	model.max = r3.Vector{X: 16, Y: 16, Z: 72}

	// Add visibility points for center of hitboxes
	// These points should be local to the model (relative to model origin)
	model.visibilityPoints = []r3.Vector{
		{X: 0, Y: 0, Z: 36},  // Center of player model (waist level)
		{X: 0, Y: 0, Z: 64},  // Head level
		{X: 0, Y: 8, Z: 36},  // Right side
		{X: 0, Y: -8, Z: 36}, // Left side
	}

	// Add a simple triangle for the model
	model.triangles = []types.Triangle{
		{
			V1: r3.Vector{X: -8, Y: -8, Z: 0},
			V2: r3.Vector{X: 8, Y: -8, Z: 0},
			V3: r3.Vector{X: 0, Y: 8, Z: 72},
		},
	}

	return model
}

func createVisTestMapModel() *MapModel {
	model := NewMapModel()
	// Add some basic geometry for testing
	model.BaseModel.triangles = []types.Triangle{
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

	assertVectorsEqual(t, want.Normalize(), got.Normalize(), 0.001, "validateForwardVector")
}

func validateCoordinatePoint(t *testing.T, point r3.Vector) {
	// Verify point follows Source2 coordinate system rules
	assert.False(t, math.IsNaN(point.X), "X coordinate should not be NaN")
	assert.False(t, math.IsNaN(point.Y), "Y coordinate should not be NaN")
	assert.False(t, math.IsNaN(point.Z), "Z coordinate should not be NaN")
}

func assertVectorsEqual(t *testing.T, want, got r3.Vector, tolerance float64, assertionMsg string) {
	assert.InDelta(t, want.X, got.X, tolerance, assertionMsg+" X component mismatch")
	assert.InDelta(t, want.Y, got.Y, tolerance, assertionMsg+" Y component mismatch")
	assert.InDelta(t, want.Z, got.Z, tolerance, assertionMsg+" Z component mismatch")
}
