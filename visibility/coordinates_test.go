package visibility

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoordinateTransformations performs comprehensive testing of the coordinate system transformations
func TestCoordinateTransformations(t *testing.T) {
	// Create output directory
	outDir := "export/coord_test"
	os.MkdirAll(outDir, 0755)

	// Import the map model
	mapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, err := ImportGLTFMapModel(mapFilePath, "de_mirage")
	require.NoError(t, err, "Failed to import map model")

	// Import the player model
	playerFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerModel, err := ImportGLTFPlayerModel(playerFilePath)
	require.NoError(t, err, "Failed to import player model")

	// Log model bounds for reference
	t.Logf("Map Model Bounds: Min: %v, Max: %v", mapModel.BaseModel.min, mapModel.BaseModel.max)
	t.Logf("Player Model Bounds: Min: %v, Max: %v", playerModel.min, playerModel.max)

	// Define known CS2 positions to test
	// Use positions that you know the correct locations of in the map
	testPositions := []struct {
		name     string
		cs2Pos   r3.Vector
		location string // Description of where this should be on the map
	}{
		{"B Site", r3.Vector{X: 130.04, Y: 130.04, Z: -39.97}, "center of B site"},
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}, "bench in T spawn"},
		{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}, "map origin point"},
		// Add more known positions as needed
	}

	// Test different transformation options
	testTransformers := []struct {
		name      string
		transform func(r3.Vector) r3.Vector
	}{
		{"Default", NewDefaultSource2Coordinates().CS2ToModelSpace},
		{"NoSwap", (&Source2Coordinates{UnitScale: 1.0 / 39.37, SwapAxes: false}).CS2ToModelSpace},
		{"InvertX", (&Source2Coordinates{UnitScale: 1.0 / 39.37, SwapAxes: true, InvertX: true}).CS2ToModelSpace},
		{"InvertY", (&Source2Coordinates{UnitScale: 1.0 / 39.37, SwapAxes: true, InvertY: true}).CS2ToModelSpace},
		{"InvertZ", (&Source2Coordinates{UnitScale: 1.0 / 39.37, SwapAxes: true, InvertZ: true}).CS2ToModelSpace},
		// Add more transformation options to test
	}

	// Test standard transformation
	t.Run("StandardTransformation", func(t *testing.T) {
		coords := NewDefaultSource2Coordinates()

		// For each test position, test the transformation and export a model
		for _, pos := range testPositions {
			modelPos := coords.CS2ToModelSpace(pos.cs2Pos)
			cs2PosRoundTrip := coords.ModelSpaceToCS2(modelPos)

			t.Logf("Position %s:", pos.name)
			t.Logf("  CS2: %v", pos.cs2Pos)
			t.Logf("  Model Space: %v", modelPos)
			t.Logf("  CS2 Round Trip: %v", cs2PosRoundTrip)

			// Verify round-trip conversion is reasonably accurate
			assert.InDeltaf(t, pos.cs2Pos.X, cs2PosRoundTrip.X, 0.001,
				"X coordinate round-trip should be accurate")
			assert.InDeltaf(t, pos.cs2Pos.Y, cs2PosRoundTrip.Y, 0.001,
				"Y coordinate round-trip should be accurate")
			assert.InDeltaf(t, pos.cs2Pos.Z, cs2PosRoundTrip.Z, 0.001,
				"Z coordinate round-trip should be accurate")

			// Export a model for visual verification
			outputPath := filepath.Join(outDir, fmt.Sprintf("standard_%s.obj",
				sanitizeFilename(pos.name)))

			err := ExportMapWithPlayerAtCS2Position(mapModel, playerModel, pos.cs2Pos, outputPath)
			assert.NoError(t, err, "Failed to export position %s", pos.name)
		}
	})

	// Test all transformers with all positions
	for _, transformer := range testTransformers {
		t.Run(transformer.name, func(t *testing.T) {
			// Create output subdirectory for this transformer
			transformerDir := filepath.Join(outDir, transformer.name)
			os.MkdirAll(transformerDir, 0755)

			for _, pos := range testPositions {
				// Apply the transformation
				modelPos := transformer.transform(pos.cs2Pos)

				t.Logf("%s - Position %s:", transformer.name, pos.name)
				t.Logf("  CS2: %v", pos.cs2Pos)
				t.Logf("  Model Space: %v", modelPos)

				// Export model
				outputPath := filepath.Join(transformerDir,
					fmt.Sprintf("%s.obj", sanitizeFilename(pos.name)))

				// Export with transformed position
				err := ExportCombinedModelToOBJ2(mapModel, []*Model{playerModel},
					[]r3.Vector{modelPos}, outputPath)

				assert.NoError(t, err, "Failed to export position %s with transformer %s",
					pos.name, transformer.name)
			}
		})
	}

	// Test varying scale factors
	scaleFactors := []float64{1.0 / 10.0, 1.0 / 39.37, 1.0 / 100.0}

	for _, scale := range scaleFactors {
		scaleStr := fmt.Sprintf("scale_%.4f", scale)
		t.Run(scaleStr, func(t *testing.T) {
			// Create transformer with this scale
			coords := &Source2Coordinates{
				UnitScale: scale,
				SwapAxes:  true,
			}

			// Create output subdirectory for this scale
			scaleDir := filepath.Join(outDir, scaleStr)
			os.MkdirAll(scaleDir, 0755)

			for _, pos := range testPositions {
				// Apply the transformation
				modelPos := coords.CS2ToModelSpace(pos.cs2Pos)

				t.Logf("Scale %f - Position %s:", scale, pos.name)
				t.Logf("  CS2: %v", pos.cs2Pos)
				t.Logf("  Model Space: %v", modelPos)

				// Export model
				outputPath := filepath.Join(scaleDir,
					fmt.Sprintf("%s.obj", sanitizeFilename(pos.name)))

				// Export with transformed position
				err := ExportCombinedModelToOBJ2(mapModel, []*Model{playerModel},
					[]r3.Vector{modelPos}, outputPath)

				assert.NoError(t, err)
			}
		})
	}

	// Extract model vertices for additional inspection
	t.Run("ModelVertices", func(t *testing.T) {
		// Get a single triangle from player model for reference
		if len(playerModel.triangles) > 0 {
			tri := playerModel.triangles[0]
			t.Logf("Player model first triangle:")
			t.Logf("  V1: %v", tri.V1)
			t.Logf("  V2: %v", tri.V2)
			t.Logf("  V3: %v", tri.V3)

			// Get a few triangles from map model
			if len(mapModel.BaseModel.triangles) > 0 {
				t.Logf("Map model first triangle:")
				tri := mapModel.BaseModel.triangles[0]
				t.Logf("  V1: %v", tri.V1)
				t.Logf("  V2: %v", tri.V2)
				t.Logf("  V3: %v", tri.V3)
			}
		}
	})
}
