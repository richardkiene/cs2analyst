package visibility

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// Source2Coordinates provides utility functions for working with Source2 coordinate systems
type Source2Coordinates struct {
	// Scaling factor from CS2 units to meters (standard is ~39.37 units per meter)
	UnitScale float64

	// Whether to apply axis swapping for the map coordinates
	SwapAxes bool

	// Whether to invert specific axes
	InvertX, InvertY, InvertZ bool
}

// NewDefaultSource2Coordinates creates a default Source2 coordinate transformer
func NewDefaultSource2Coordinates() *Source2Coordinates {
	return &Source2Coordinates{
		UnitScale: 1.0,  // No scaling for now
		SwapAxes:  true, // We need axis swapping based on our analysis
		InvertX:   false,
		InvertY:   false,
		InvertZ:   false,
	}
}

// CS2ToModelSpace transforms CS2 game coordinates to model space
// This is the key transformation for positioning players in the visualization
func (sc *Source2Coordinates) CS2ToModelSpace(cs2Pos r3.Vector) r3.Vector {
	// Scale the position from CS2 units to meters
	scaled := r3.Vector{
		X: cs2Pos.X * sc.UnitScale,
		Y: cs2Pos.Y * sc.UnitScale,
		Z: cs2Pos.Z * sc.UnitScale,
	}

	var result r3.Vector

	if sc.SwapAxes {
		// Transform from Source2 to GLTF model space
		// Source2: X=forward/East, Y=left/North, Z=up
		// GLTF: X=right, Y=up, Z=forward
		result = r3.Vector{
			X: scaled.Z, // CS2's Z (up) -> X
			Y: scaled.X, // CS2's X (forward/East) -> Y
			Z: scaled.Y, // CS2's Y (left/North) -> Z
		}
	} else {
		// No axis swapping, just use the scaled coordinates
		result = scaled
	}

	// Apply axis inversions if needed
	if sc.InvertX {
		result.X = -result.X
	}
	if sc.InvertY {
		result.Y = -result.Y
	}
	if sc.InvertZ {
		result.Z = -result.Z
	}

	return result
}

// ModelSpaceToCS2 transforms model space coordinates back to CS2 game coordinates
// This is the inverse of CS2ToModelSpace
func (sc *Source2Coordinates) ModelSpaceToCS2(modelPos r3.Vector) r3.Vector {
	// First undo any inversions
	uninverted := r3.Vector{
		X: modelPos.X,
		Y: modelPos.Y,
		Z: modelPos.Z,
	}

	if sc.InvertX {
		uninverted.X = -uninverted.X
	}
	if sc.InvertY {
		uninverted.Y = -uninverted.Y
	}
	if sc.InvertZ {
		uninverted.Z = -uninverted.Z
	}

	var unswapped r3.Vector

	if sc.SwapAxes {
		// Undo the axis swapping:
		// Model: X <- CS2's Z (up), Y <- CS2's X (forward), Z <- CS2's Y (left)
		// So: CS2's X <- Y, CS2's Y <- Z, CS2's Z <- X
		unswapped = r3.Vector{
			X: uninverted.Y, // Y -> CS2's X (forward/East)
			Y: uninverted.Z, // Z -> CS2's Y (left/North)
			Z: uninverted.X, // X -> CS2's Z (up)
		}
	} else {
		unswapped = uninverted
	}

	// Scale back from meters to CS2 units
	return r3.Vector{
		X: unswapped.X / sc.UnitScale,
		Y: unswapped.Y / sc.UnitScale,
		Z: unswapped.Z / sc.UnitScale,
	}
}

func (sc *Source2Coordinates) ApplyCS2Rotation(viewAngleX, viewAngleY float32, direction r3.Vector) r3.Vector {
	// Convert angles to radians
	yawRad := float64(viewAngleX) * math.Pi / 180.0
	pitchRad := float64(viewAngleY) * math.Pi / 180.0

	// In Source2: X=forward/East, Y=left/North, Z=up
	// Yaw rotates around Z axis (0=north, 90=east, 180=south, 270=west)
	// Pitch rotates around Y axis after yaw (0=horizontal, positive=down, negative=up)

	// Compute the forward vector in Source2 coordinates
	forward := r3.Vector{
		X: math.Sin(yawRad) * math.Cos(pitchRad),
		Y: math.Cos(yawRad) * math.Cos(pitchRad),
		Z: -math.Sin(pitchRad), // Negative because pitch is positive downward
	}

	// Transform to model space
	return sc.CS2ToModelSpace(forward)
}

// transformPlayerTickToModelSpace transforms a PlayerTickData to match the coordinate
// system used by the map model (after GLTF transformation)
func transformPlayerTickToModelSpace(player types.PlayerTickData, mapModel *MapModel) types.PlayerTickData {
	// Create a copy of the player data to avoid modifying the original
	transformedPlayer := player

	// Create a coordinate transformer
	coords := NewDefaultSource2Coordinates()

	// Transform the position using our coordinate transformer
	transformedPlayer.Position = coords.CS2ToModelSpace(player.Position)

	return transformedPlayer
}

// GetAdjustedEyePosition calculates the eye position based on the player's position and height
func GetAdjustedEyePosition(shooter types.PlayerTickData, playerModel *Model, mapModel *MapModel) r3.Vector {
	// Standard CS2 eye heights in game units
	eyeHeight := 64.0 // Standing height
	if shooter.IsCrouched {
		eyeHeight = 46.0 // Crouching height
	}

	// Create a coordinate transformer
	coords := NewDefaultSource2Coordinates()

	// Calculate the eye position in Source2 coordinates first
	eyePosSource2 := r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y,
		Z: shooter.Position.Z + eyeHeight,
	}

	// Transform the complete eye position to model space
	return coords.CS2ToModelSpace(eyePosSource2)
}

// DebugCoordinateTransformation provides a detailed analysis of the coordinate transformation
// This is a diagnostic function to help pinpoint coordinate system issues
func DebugCoordinateTransformation(mapModel *MapModel, playerModel *Model, testPoints []r3.Vector) {
	slog.Info("===== COORDINATE TRANSFORMATION DEBUG =====")

	// Log map model information
	mapSize := r3.Vector{
		X: mapModel.BaseModel.max.X - mapModel.BaseModel.min.X,
		Y: mapModel.BaseModel.max.Y - mapModel.BaseModel.min.Y,
		Z: mapModel.BaseModel.max.Z - mapModel.BaseModel.min.Z,
	}

	slog.Info("Map model bounds:",
		"min", mapModel.BaseModel.min,
		"max", mapModel.BaseModel.max,
		"size", mapSize)

	// Create the coordinate transformer
	coords := NewDefaultSource2Coordinates()

	// Process each test point
	slog.Info("Testing coordinate transformations for reference points:")
	for i, cs2Pos := range testPoints {
		// Transform to model space
		modelPos := coords.CS2ToModelSpace(cs2Pos)

		// Transform back to validate the round-trip conversion
		roundTripCS2 := coords.ModelSpaceToCS2(modelPos)

		slog.Info(fmt.Sprintf("Test point %d:", i),
			"cs2_original", cs2Pos,
			"model_space", modelPos,
			"cs2_roundtrip", roundTripCS2,
			"roundtrip_error", r3.Vector{
				X: cs2Pos.X - roundTripCS2.X,
				Y: cs2Pos.Y - roundTripCS2.Y,
				Z: cs2Pos.Z - roundTripCS2.Z,
			})

		// Export a debug object for this specific point
		ExportMapWithPlayerAtCS2Position(mapModel, playerModel, cs2Pos,
			fmt.Sprintf("export/debug_point_%d.obj", i))
	}

	// Test view direction transformations
	slog.Info("Testing view angle transformations:")
	for _, angle := range []float32{0, 90, 180, 270} {
		// Test horizontal rotation (yaw)
		dir := r3.Vector{X: 1, Y: 0, Z: 0} // Forward in Source2
		rotated := coords.ApplyCS2Rotation(angle, 0, dir)

		slog.Info(fmt.Sprintf("Yaw angle %.1f degrees:", float64(angle)),
			"source2_dir", dir,
			"model_space_dir", rotated)
	}

	slog.Info("===== END COORDINATE TRANSFORMATION DEBUG =====")
}
