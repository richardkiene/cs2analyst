package visibility

import (
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
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
		name        string
		pos         r3.Vector
		shooterData types.PlayerTickData
		targetData  types.PlayerTickData
	}{
		/*{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}, types.PlayerTickData{}, types.PlayerTickData{}},
		{"Top_Mid", r3.Vector{X: 89.64, Y: -556.01, Z: -110.93}, types.PlayerTickData{}, types.PlayerTickData{}},
		{"A_Ticket", r3.Vector{X: -871.26, Y: -2319.52, Z: -106.42}, types.PlayerTickData{}, types.PlayerTickData{}},
		{"A_PalaceElbow", r3.Vector{X: 16.8408145904541, Y: -2324.8759765625, Z: -39.96875}, types.PlayerTickData{}, types.PlayerTickData{}},
		{"A_Plywood", r3.Vector{X: 130.04379272460938, Y: -1922.3170166015625, Z: -39.96875}, types.PlayerTickData{}, types.PlayerTickData{}},
		{"B_Arches", r3.Vector{X: -1515.4642333984375, Y: 216.43341064453125, Z: -166.96875}, types.PlayerTickData{}, types.PlayerTickData{}},*/
		{
			"B_Apps",
			r3.Vector{
				X: -1652.548828125,
				Y: 746.81103515625,
				Z: -47.96875,
			},
			types.PlayerTickData{
				Position:   r3.Vector{X: -1652.548828125, Y: 746.81103515625, Z: -47.96875},
				ViewAngleX: -75.15026092529297,
				ViewAngleY: 10.731582641601562,
				IsAlive:    true,
				IsCrouched: false,
			},
			types.PlayerTickData{
				Position:   r3.Vector{X: -1515.4642333984375, Y: 216.43341064453125, Z: -166.96875},
				ViewAngleX: 106.509033203125,
				ViewAngleY: -12.48699951171875,
				IsAlive:    true,
				IsCrouched: false,
			},
		},
		{
			"CT_To_Ticket_Tick_29290_shooter",
			r3.Vector{
				X: -980.9349365234375,
				Y: -2327.1201171875,
				Z: -167.96875,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -980.9349365234375,
					Y: -2327.1201171875,
					Z: -167.96875,
				},
				ViewAngleX: 168.4791259765625,
				ViewAngleY: 9.140960693359375,
				IsAlive:    true,
				IsCrouched: false,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -1585.734130859375,
					Y: -2191.7490234375,
					Z: -253.405517578125,
				},
				ViewAngleX: -10.427734375,
				ViewAngleY: -6.9893646240234375,
				IsAlive:    true,
				IsCrouched: false,
			},
		},
		{
			"Unknown_Tick_160327",
			r3.Vector{
				X: -1865.9390869140625,
				Y: -624.790283203125,
				Z: -167.96875,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -1865.9390869140625,
					Y: -624.790283203125,
					Z: -167.96875,
				},
				ViewAngleX: 101.528076171875,
				ViewAngleY: -1.093475341796875,
				IsAlive:    true,
				IsCrouched: false,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -2182.810546875,
					Y: 827.809814453125,
					Z: -123.00994873046875,
				},
				ViewAngleX: -101.16519927978516,
				ViewAngleY: 3.62994384765625,
				IsAlive:    true,
				IsCrouched: false,
			},
		},
		{
			"Unknown_Tick_94276",
			r3.Vector{
				X: -1865.9390869140625,
				Y: -624.790283203125,
				Z: -167.96875,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -111.48406982421875,
					Y: -1497.803955078125,
					Z: -53.96875,
				},
				ViewAngleX: -134.33016967773438,
				ViewAngleY: 12.628097534179688,
				IsAlive:    true,
				IsCrouched: false,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -519.3766479492188,
					Y: -1885.4940185546875,
					Z: -179.96875,
				},
				ViewAngleX: -124.2502212524414,
				ViewAngleY: 0.7326507568359375,
				IsAlive:    true,
				IsCrouched: false,
			},
		},
		{
			"Tick_109075_Window_To_Bench",
			r3.Vector{
				X: -1149.90234375,
				Y: -612.816162109375,
				Z: -167.96875,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -1149.90234375,
					Y: -612.816162109375,
					Z: -167.96875,
				},
				ViewAngleX: -32.27027893066406,
				ViewAngleY: 14.440841674804688,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -856.7142333984375,
					Y: -788.9706420898438,
					Z: -221.96875,
				},
				ViewAngleX: 85.61611938476562,
				ViewAngleY: -4.375640869140625,
			},
		},
		{
			"Tick_109040_Window_To_Bench",
			r3.Vector{
				X: -1177.66,
				Y: -674.00,
				Z: -168.13,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -1177.66,
					Y: -674.00,
					Z: -168.13,
				},
				ViewAngleX: 12.69,
				ViewAngleY: 4.59,
			},
			types.PlayerTickData{
				Position: r3.Vector{
					X: -852.83,
					Y: -788.79,
					Z: -220.81,
				},
				ViewAngleX: 84.77,
				ViewAngleY: -3.16,
			},
		},
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

			var err error
			// We test if the viewangle properties have been set as a hack for now
			if pos.shooterData.ViewAngleX != 0 && pos.targetData.ViewAngleX != 0 {
				DebugShooterTargetVectors(&pos.shooterData, &pos.targetData)
				err = ExportDebugVisualization(mapModel, playerModel, pos.pos, modelPos, outputPath, &pos.shooterData, &pos.targetData)
			} else {
				err = ExportDebugVisualization(mapModel, playerModel, pos.pos, modelPos, outputPath)
			}

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

// Add this function to your code
func DebugShooterTargetVectors(shooterData, targetData *types.PlayerTickData) {
	// Log the original positions and angles
	slog.Info("Debugging shooter/target vectors",
		"shooterPos", shooterData.Position,
		"viewAngleX", shooterData.ViewAngleX,
		"viewAngleY", shooterData.ViewAngleY,
		"targetPos", targetData.Position)

	// Calculate and log the actual vector from shooter to target in CS2 coordinates
	toTarget := targetData.Position.Sub(shooterData.Position).Normalize()
	slog.Info("Vector calculations",
		"toTarget", toTarget,
		"forward", shooterData.ForwardVector())

	// Calculate the angle between the forward vector and the vector to target
	forward := shooterData.ForwardVector()
	dotProduct := forward.Dot(toTarget)
	// Clamp to avoid floating point errors outside [-1, 1]
	if dotProduct > 1.0 {
		dotProduct = 1.0
	} else if dotProduct < -1.0 {
		dotProduct = -1.0
	}
	angleBetween := math.Acos(dotProduct) * (180.0 / math.Pi)
	slog.Info("Angle between vectors",
		"degrees", angleBetween)

	// Now check what happens in model space
	coords := NewDefaultSource2Coordinates()
	modelShooterPos := coords.CS2ToModelSpace(shooterData.Position)
	modelTargetPos := coords.CS2ToModelSpace(targetData.Position)
	modelToTarget := modelTargetPos.Sub(modelShooterPos).Normalize()

	// Two ways to get the model forward vector
	modelForward1 := coords.CS2ToModelSpace(forward)
	modelForward2 := coords.ApplyCS2Rotation(shooterData.ViewAngleX, shooterData.ViewAngleY, r3.Vector{X: 1, Y: 0, Z: 0})

	slog.Info("Model space vectors",
		"modelToTarget", modelToTarget,
		"modelForward1", modelForward1,
		"modelForward2", modelForward2,
	)
}
