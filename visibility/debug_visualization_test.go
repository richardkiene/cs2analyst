package visibility

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/stretchr/testify/require"
)

// TestDebugCoordinateVisualization tests the debug visualization with coordinate axes
func TestDebugCoordinateVisualization(t *testing.T) {
	// Create output directory
	outDir := "export/debug_visualization"
	os.MkdirAll(outDir, 0755)

	// Import the map model
	mapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, err := ImportGLTFMapModel(mapFilePath, "de_mirage")
	require.NoError(t, err, "Failed to import map model")

	// Import the player model
	playerFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerModel, err := ImportGLTFPlayerModel(playerFilePath)
	require.NoError(t, err, "Failed to import player model")

	// Define test positions in CS2 coordinates
	testPositions := []struct {
		name     string
		position r3.Vector
	}{
		{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}},
		{"BenchMark", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"BSite", r3.Vector{X: 130.04, Y: 130.04, Z: -39.97}},
		// Add some positions that correspond to known landmarks in the map
		{"Palace", r3.Vector{X: -2389.67, Y: 705.69, Z: -39.97}},
		{"AMain", r3.Vector{X: -476.13, Y: -142.99, Z: -39.97}},
		{"Mid", r3.Vector{X: 130.04, Y: -1052.98, Z: -103.97}},
	}

	// Test visualization for each position
	for _, pos := range testPositions {
		t.Run(pos.name, func(t *testing.T) {
			outputPath := filepath.Join(outDir, pos.name+".obj")

			// Create the visualization with axes
			err := VisualizePositionWithDebugPoints(mapModel, playerModel, pos.position, outputPath)
			require.NoError(t, err, "Failed to create debug visualization")

			t.Logf("Created debug visualization for %s at %s", pos.name, outputPath)

			// Verify the files were created
			_, err = os.Stat(outputPath[:len(outputPath)-4] + "_with_axes.obj")
			require.NoError(t, err, "Debug visualization OBJ file should exist")

			_, err = os.Stat(outputPath[:len(outputPath)-4] + "_with_axes.mtl")
			require.NoError(t, err, "Debug visualization MTL file should exist")
		})
	}

	// Test visualization with transformed positions
	t.Run("TestWithTransformedPositions", func(t *testing.T) {
		// Create the coordinate transformer
		coords := NewDefaultSource2Coordinates()

		for _, pos := range testPositions {
			// Transform to model space
			modelPos := coords.CS2ToModelSpace(pos.position)

			// Create visualization with explicit model position
			outputPath := filepath.Join(outDir, pos.name+"_explicit.obj")

			// Create visualization showing both coordinate interpretations
			err := ExportDebugVisualization(mapModel, playerModel, pos.position, modelPos, outputPath)
			require.NoError(t, err, "Failed to create explicit debug visualization")

			t.Logf("Created explicit debug visualization for %s at %s", pos.name, outputPath)
			t.Logf("  CS2 Position: %v", pos.position)
			t.Logf("  Model Position: %v", modelPos)
		}
	})
}

// TestVerifyAxisFix tests the fixed axis visualization
func TestVerifyAxisFix(t *testing.T) {
	// Create output directory
	outDir := "export/verify_axis_fix"
	os.MkdirAll(outDir, 0755)

	// Import models
	mapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, err := ImportGLTFMapModel(mapFilePath, "de_mirage")
	require.NoError(t, err, "Failed to import map model")

	playerFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerModel, err := ImportGLTFPlayerModel(playerFilePath)
	require.NoError(t, err, "Failed to import player model")

	// Define test positions
	positions := []struct {
		name string
		pos  r3.Vector
	}{
		{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}},
		{"Top_Mid", r3.Vector{X: 89.64, Y: -556.01, Z: -110.93}},
		{"A_Ticket", r3.Vector{X: -871.26, Y: -2319.52, Z: -106.42}},
		{"A_PalaceElbo", r3.Vector{X: 16.8408145904541, Y: -2324.8759765625, Z: -39.96875}},
		{"A_Plywood", r3.Vector{X: 130.04379272460938, Y: -1922.3170166015625, Z: -39.96875}},
		{"B_Arches", r3.Vector{X: -1515.4642333984375, Y: 216.43341064453125, Z: -166.96875}},
		{"B_Apps", r3.Vector{X: -1652.548828125, Y: 746.81103515625, Z: -47.96875}},
	}

	// Test each position
	for _, pos := range positions {
		t.Run(pos.name, func(t *testing.T) {
			// Transform position to model space
			coords := NewDefaultSource2Coordinates() // Now using the updated transformer
			modelPos := coords.CS2ToModelSpace(pos.pos)

			// Log original and transformed position
			t.Logf("CS2 Position: %v", pos.pos)
			t.Logf("Model Position: %v", modelPos)

			// Export visualization with coordinate axes
			outputPath := filepath.Join(outDir, pos.name+".obj")
			err := ExportDebugVisualization(mapModel, playerModel, pos.pos, modelPos, outputPath)
			require.NoError(t, err, "Failed to export debug visualization")

			// Verify the files exist
			axesObjPath := outputPath[:len(outputPath)-4] + "_with_axes.obj"
			axesMtlPath := outputPath[:len(outputPath)-4] + "_with_axes.mtl"

			_, err = os.Stat(axesObjPath)
			require.NoError(t, err, "Axes OBJ file should exist")

			_, err = os.Stat(axesMtlPath)
			require.NoError(t, err, "Axes MTL file should exist")

			t.Logf("Created debug visualization at %s", axesObjPath)
		})
	}
}
