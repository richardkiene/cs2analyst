package visibility

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorrectPlayerPositioning tests the fixed coordinate transformation
func TestCorrectPlayerPositioning(t *testing.T) {
	// Import the map model
	testMapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, mapErr := ImportGLTFMapModel(testMapFilePath, "de_mirage")
	if mapErr != nil {
		t.Fatalf("Failed to import map model: %v", mapErr)
	}

	// Import the player model
	testModelFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerModel, modelErr := ImportGLTFPlayerModel(testModelFilePath)
	if modelErr != nil {
		t.Fatalf("Failed to import player model: %v", modelErr)
	}

	// Log model bounds for reference
	t.Logf("Map Model Bounds: Min: %v, Max: %v",
		mapModel.BaseModel.min, mapModel.BaseModel.max)
	t.Logf("Player Model Bounds: Min: %v, Max: %v",
		playerModel.min, playerModel.max)

	// Define test positions in CS2 coordinates
	testPositions := []struct {
		name     string
		position r3.Vector
	}{
		{"B Site Apartments", r3.Vector{X: 130.04379272460938, Y: 130.04379272460938, Z: -39.96875}},
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"Top Mid", r3.Vector{X: 0, Y: 0, Z: 0}}, // Origin/palm tree
	}

	// Create a coordinate transformer
	coords := NewDefaultSource2Coordinates()

	// Export a test file with multiple players, using our new transformer
	players := make([]*Model, len(testPositions))
	transformedPositions := make([]r3.Vector, len(testPositions))

	for i, pos := range testPositions {
		players[i] = playerModel

		// Apply our coordinate transformation using the new method
		transformedPos := coords.CS2ToModelSpace(pos.position)
		transformedPositions[i] = transformedPos

		t.Logf("CS2 position: %s - %v", pos.name, pos.position)
		t.Logf("Transformed position: %s - %v", pos.name, transformedPos)
	}

	// Export using our new transformer
	err := ExportCombinedModelToOBJ2(mapModel, players, transformedPositions, "export/fixed_positions.obj")
	assert.NoError(t, err, "Should be able to export with transformed positions")

	// Export using our helper function
	for _, pos := range testPositions {
		outputPath := fmt.Sprintf("export/fixed_%s.obj", sanitizeFilename(pos.name))
		err := ExportMapWithPlayerAtCS2Position(mapModel, playerModel, pos.position, outputPath)
		assert.NoError(t, err, "Should be able to export individual position")
	}
}

// TestCoordinateSystemAnalysis helps analyze the coordinate systems
func TestCoordinateSystemAnalysis(t *testing.T) {
	// Import the map model
	testMapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, mapErr := ImportGLTFMapModel(testMapFilePath, "de_mirage")
	if mapErr != nil {
		t.Fatalf("Failed to import map model: %v", mapErr)
	}

	// Log map bounds and dimensions
	mapSize := r3.Vector{
		X: mapModel.BaseModel.max.X - mapModel.BaseModel.min.X,
		Y: mapModel.BaseModel.max.Y - mapModel.BaseModel.min.Y,
		Z: mapModel.BaseModel.max.Z - mapModel.BaseModel.min.Z,
	}
	t.Logf("Map bounds: Min=%v, Max=%v", mapModel.BaseModel.min, mapModel.BaseModel.max)
	t.Logf("Map dimensions: %v", mapSize)

	// Define known positions for analysis
	knownCS2Positions := []struct {
		name     string
		position r3.Vector
	}{
		{"B Site Apartments", r3.Vector{X: 130.04379272460938, Y: 130.04379272460938, Z: -39.96875}},
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"Top Mid", r3.Vector{X: 0, Y: 0, Z: 0}}, // Origin
	}

	// Test different coordinate transformations
	transformOptions := []struct {
		name      string
		transform func(r3.Vector) r3.Vector
	}{
		{"Source2_Standard", func(v r3.Vector) r3.Vector {
			// Standard Source2 transformation (X=forward/East, Y=left/North, Z=up)
			const scale = 1.0 / 39.37
			return r3.Vector{
				X: v.Z * scale, // Z (up) -> X
				Y: v.X * scale, // X (forward) -> Y
				Z: v.Y * scale, // Y (left) -> Z
			}
		}},
		{"Source2_Inverted", func(v r3.Vector) r3.Vector {
			// Try with inverted X and Y
			const scale = 1.0 / 39.37
			return r3.Vector{
				X: v.Z * scale,  // Z (up) -> X
				Y: -v.X * scale, // -X (backward) -> Y
				Z: -v.Y * scale, // -Y (right) -> Z
			}
		}},
		{"Unity_Style", func(v r3.Vector) r3.Vector {
			// Unity-style transformation (X=right, Y=up, Z=forward)
			const scale = 1.0 / 39.37
			return r3.Vector{
				X: v.Y * scale, // Y (left) -> X (but inverted)
				Y: v.Z * scale, // Z (up) -> Y
				Z: v.X * scale, // X (forward) -> Z
			}
		}},
	}

	// Log results of different transformations
	for _, posData := range knownCS2Positions {
		t.Logf("CS2 position: %s - %v", posData.name, posData.position)

		for _, transformOption := range transformOptions {
			transformedPos := transformOption.transform(posData.position)
			t.Logf("  %s: %v", transformOption.name, transformedPos)
		}
	}
}

func TestExportMapModelToOBJ(t *testing.T) {
	// Create a temporary directory for test output
	tempDir, err := os.MkdirTemp("", "mapmodel_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create a simple test map model
	mapModel := createTestMapModel()

	// Export the map model to OBJ
	outputPath := filepath.Join(tempDir, "test_map.obj")
	err = ExportMapModelToOBJ(mapModel, outputPath)
	require.NoError(t, err, "ExportMapModelToOBJ should not return an error")

	// Verify the OBJ file was created
	_, err = os.Stat(outputPath)
	assert.NoError(t, err, "OBJ file should exist")

	// Verify the MTL file was created
	mtlPath := filepath.Join(tempDir, "test_map.mtl")
	_, err = os.Stat(mtlPath)
	assert.NoError(t, err, "MTL file should exist")

	// Check OBJ file contents
	objFile, err := os.Open(outputPath)
	require.NoError(t, err, "Should be able to open the OBJ file")
	defer objFile.Close()

	// Verify expected content in OBJ file
	scanner := bufio.NewScanner(objFile)

	// Track what we've found
	foundHeader := false
	foundMtlLib := false
	foundVertices := 0
	foundFaces := 0

	for scanner.Scan() {
		line := scanner.Text()

		// Check for header comment
		if strings.HasPrefix(line, "# CS2 Map Model exported to OBJ") {
			foundHeader = true
		}

		// Check for mtllib reference
		if strings.HasPrefix(line, "mtllib") {
			foundMtlLib = true
			assert.Contains(t, line, "test_map.mtl", "MTL reference should contain correct filename")
		}

		// Count vertices
		if strings.HasPrefix(line, "v ") {
			foundVertices++
		}

		// Count faces
		if strings.HasPrefix(line, "f ") {
			foundFaces++
		}
	}

	assert.True(t, foundHeader, "OBJ file should contain a header comment")
	assert.True(t, foundMtlLib, "OBJ file should reference the MTL file")
	assert.Equal(t, 6, foundVertices, "OBJ file should have 6 vertices (3 per triangle)")
	assert.Equal(t, 2, foundFaces, "OBJ file should have 2 faces")

	// Check MTL file contents
	mtlFile, err := os.Open(mtlPath)
	require.NoError(t, err, "Should be able to open the MTL file")
	defer mtlFile.Close()

	// Verify expected content in MTL file
	mtlScanner := bufio.NewScanner(mtlFile)

	foundOpaqueMaterial := false
	foundTransparentMaterial := false

	for mtlScanner.Scan() {
		line := mtlScanner.Text()

		// Check for material definitions
		if strings.HasPrefix(line, "newmtl") {
			if strings.Contains(line, "opaque") {
				foundOpaqueMaterial = true
			} else if strings.Contains(line, "transparent") {
				foundTransparentMaterial = true
			}
		}
	}

	assert.True(t, foundOpaqueMaterial, "MTL file should define an opaque material")
	assert.True(t, foundTransparentMaterial, "MTL file should define a transparent material")
}

func TestExportModelToOBJ(t *testing.T) {
	// Create a temporary directory for test output
	tempDir, err := os.MkdirTemp("", "playermodel_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create a simple test player model
	playerModel := createTestPlayerModel()

	// Export the player model to OBJ
	outputPath := filepath.Join(tempDir, "test_player.obj")
	err = ExportModelToOBJ(playerModel, outputPath)
	require.NoError(t, err, "ExportModelToOBJ should not return an error")

	// Verify the OBJ file was created
	_, err = os.Stat(outputPath)
	assert.NoError(t, err, "OBJ file should exist")

	// Verify the MTL file was created
	mtlPath := filepath.Join(tempDir, "test_player.mtl")
	_, err = os.Stat(mtlPath)
	assert.NoError(t, err, "MTL file should exist")

	// Check OBJ file contents
	objFile, err := os.Open(outputPath)
	require.NoError(t, err, "Should be able to open the OBJ file")
	defer objFile.Close()

	// Verify expected content in OBJ file
	scanner := bufio.NewScanner(objFile)

	// Track what we've found
	foundHeader := false
	foundMtlLib := false
	foundModelGroup := false
	foundHitboxGroup := false
	foundVertices := 0
	foundFaces := 0
	foundLines := 0 // For hitbox wireframes

	for scanner.Scan() {
		line := scanner.Text()

		// Check for header comment
		if strings.HasPrefix(line, "# CS2 Player Model exported to OBJ") {
			foundHeader = true
		}

		// Check for mtllib reference
		if strings.HasPrefix(line, "mtllib") {
			foundMtlLib = true
			assert.Contains(t, line, "test_player.mtl", "MTL reference should contain correct filename")
		}

		// Check for model group
		if line == "g model_mesh" {
			foundModelGroup = true
		}

		// Check for hitbox group
		if line == "g hitboxes" {
			foundHitboxGroup = true
		}

		// Count vertices
		if strings.HasPrefix(line, "v ") {
			foundVertices++
		}

		// Count faces
		if strings.HasPrefix(line, "f ") {
			foundFaces++
		}

		// Count lines (hitbox edges)
		if strings.HasPrefix(line, "l ") {
			foundLines++
		}
	}

	assert.True(t, foundHeader, "OBJ file should contain a header comment")
	assert.True(t, foundMtlLib, "OBJ file should reference the MTL file")
	assert.True(t, foundModelGroup, "OBJ file should have a model mesh group")
	assert.True(t, foundHitboxGroup, "OBJ file should have a hitboxes group")
	assert.Equal(t, 3, foundFaces, "OBJ file should have 3 faces")
	assert.Greater(t, foundLines, 0, "OBJ file should have hitbox wireframe lines")
}

func TestExportEmptyModels(t *testing.T) {
	// Create a temporary directory for test output
	tempDir, err := os.MkdirTemp("", "empty_models_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create empty models
	emptyMapModel := NewMapModel()
	emptyPlayerModel := NewModel()

	// Export the empty map model
	mapOutputPath := filepath.Join(tempDir, "empty_map.obj")
	err = ExportMapModelToOBJ(emptyMapModel, mapOutputPath)
	assert.NoError(t, err, "Exporting empty map model should not cause errors")

	// Export the empty player model
	playerOutputPath := filepath.Join(tempDir, "empty_player.obj")
	err = ExportModelToOBJ(emptyPlayerModel, playerOutputPath)
	assert.NoError(t, err, "Exporting empty player model should not cause errors")

	// Verify the files exist but have minimal content
	mapObjFile, err := os.Open(mapOutputPath)
	require.NoError(t, err)
	defer mapObjFile.Close()

	playerObjFile, err := os.Open(playerOutputPath)
	require.NoError(t, err)
	defer playerObjFile.Close()

	// Check map OBJ content (should just have header, no vertices or faces)
	mapScanner := bufio.NewScanner(mapObjFile)
	mapVertexCount := 0
	mapFaceCount := 0

	for mapScanner.Scan() {
		line := mapScanner.Text()
		if strings.HasPrefix(line, "v ") {
			mapVertexCount++
		}
		if strings.HasPrefix(line, "f ") {
			mapFaceCount++
		}
	}

	assert.Equal(t, 0, mapVertexCount, "Empty map should have no vertices")
	assert.Equal(t, 0, mapFaceCount, "Empty map should have no faces")

	// Check player OBJ content
	playerScanner := bufio.NewScanner(playerObjFile)
	playerVertexCount := 0
	playerFaceCount := 0

	for playerScanner.Scan() {
		line := playerScanner.Text()
		if strings.HasPrefix(line, "v ") {
			playerVertexCount++
		}
		if strings.HasPrefix(line, "f ") {
			playerFaceCount++
		}
	}

	assert.Equal(t, 0, playerVertexCount, "Empty player model should have no vertices")
	assert.Equal(t, 0, playerFaceCount, "Empty player model should have no faces")
}

func TestExportLargeMapModel(t *testing.T) {
	// Skip this test by default unless explicitly enabled, as it's more resource-intensive
	if testing.Short() {
		t.Skip("Skipping large map test in short mode")
	}

	// Create a temporary directory for test output
	tempDir, err := os.MkdirTemp("", "large_map_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create a larger test map model
	mapModel := createLargeMapModel(1000) // 1000 triangles

	// Export the map model to OBJ
	outputPath := filepath.Join(tempDir, "large_map.obj")
	err = ExportMapModelToOBJ(mapModel, outputPath)
	require.NoError(t, err, "ExportMapModelToOBJ should not return an error with a large map")

	// Verify the OBJ file was created
	objInfo, err := os.Stat(outputPath)
	assert.NoError(t, err, "OBJ file should exist")

	// Should have a reasonably sized file
	assert.Greater(t, objInfo.Size(), int64(1000), "OBJ file should have substantial content")
}

func TestExportMirageMapModel(t *testing.T) {
	testFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, err := ImportGLTFMapModel(testFilePath, "de_mirage")
	if err != nil {
		log.Fatalf("Failed to import map model: %v", err)
	}
	err = ExportMapModelToOBJ(mapModel, "export/mirage.obj")
	if err != nil {
		log.Fatalf("Failed to export map model: %v", err)
	}
}

func TestExportCTModel(t *testing.T) {
	testFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	model, err := ImportGLTFPlayerModel(testFilePath)
	if err != nil {
		log.Fatalf("Failed to import map model: %v", err)
	}
	err = ExportModelToOBJ(model, "export/ctm_sas.obj")
	if err != nil {
		log.Fatalf("Failed to export map model: %v", err)
	}
}

func TestExportModelsInPositionsOnMapModel(t *testing.T) {
	// Export a map with a single player at position
	testMapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, mapErr := ImportGLTFMapModel(testMapFilePath, "de_mirage")
	if mapErr != nil {
		log.Fatalf("Failed to import map model: %v", mapErr)
	}

	testModelFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerModel, modelErr := ImportGLTFPlayerModel(testModelFilePath)
	if modelErr != nil {
		log.Fatalf("Failed to import map model: %v", modelErr)
	}

	playerPosition := r3.Vector{X: 130.04379272460938, Y: 130.04379272460938, Z: -39.96875}

	// Export single player at position
	mperr := ExportMapWithPlayerAtPosition(mapModel, playerModel, playerPosition, "export/player_on_map.obj")
	if mperr != nil {
		log.Fatalf("Failed to export Map with Player at position: %v", mperr)
	}

	// Add this to your test function to see the actual bounds of your map model
	fmt.Printf("Map Model Bounds: Min: %v, Max: %v\n",
		mapModel.BaseModel.min, mapModel.BaseModel.max)

	// Try different positions to understand the coordinate mapping
	testPositions := []r3.Vector{
		{X: 0, Y: 0, Z: 0}, // Origin
		{X: 130.04379272460938, Y: 130.04379272460938, Z: -39.96875},   // Your original position
		{X: 130.04379272460938, Y: -130.04379272460938, Z: -39.96875},  // Try flipping Y
		{X: -130.04379272460938, Y: 130.04379272460938, Z: -39.96875},  // Try flipping X
		{X: -130.04379272460938, Y: -130.04379272460938, Z: -39.96875}, // Try flipping both
	}

	// Export multiple test positions
	players := make([]*Model, len(testPositions))
	for i := range testPositions {
		players[i] = playerModel
	}

	ExportCombinedModelToOBJ2(mapModel, players, testPositions, "export/position_test.obj")

}

func TestVerifyBenchmarkTransformation(t *testing.T) {
	// Load your models
	mapModel, _ := ImportGLTFMapModel("../input_models/de_mirage_model/de_mirage_d.gltf", "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel("../input_models/ctm_sas_model/ctm_sas.gltf")

	// Test positions
	testPositions := []struct {
		name   string
		cs2Pos r3.Vector
	}{
		{"B Site Apartments", r3.Vector{X: 130.04, Y: 130.04, Z: -39.97}},
		{"Bench Reference", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		// Add more reference points if available
	}

	// Create transformed positions
	players := make([]*Model, len(testPositions))
	positions := make([]r3.Vector, len(testPositions))

	for i, tp := range testPositions {
		players[i] = playerModel
		positions[i] = TransformCS2ToMapModelCoords(tp.cs2Pos)
		fmt.Printf("Position %s: CS2 coords: %v → Map coords: %v\n",
			tp.name, tp.cs2Pos, positions[i])
	}

	// Export and verify in Blender
	err := ExportCombinedModelToOBJ2(mapModel, players, positions, "export/benchmark_verification.obj")
	require.NoError(t, err)
}

func TestUnitConversionTransformation(t *testing.T) {
	// Load your models
	mapModel, _ := ImportGLTFMapModel("../input_models/de_mirage_model/de_mirage_d.gltf", "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel("../input_models/ctm_sas_model/ctm_sas.gltf")

	// Test positions - use three distinct points to triangulate the transformation
	testPositions := []struct {
		name   string
		cs2Pos r3.Vector
	}{
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"B Apartments", r3.Vector{X: 130.04, Y: 130.04, Z: -39.97}},
		// Add another reference point if available
	}

	// Create transformed positions
	players := make([]*Model, len(testPositions))
	positions := make([]r3.Vector, len(testPositions))

	for i, tp := range testPositions {
		players[i] = playerModel
		positions[i] = TransformCS2ToMapModelCoords(tp.cs2Pos)
		fmt.Printf("Position %s: CS2 units: %v → Blender meters: %v\n",
			tp.name, tp.cs2Pos, positions[i])
	}

	// Export and verify in Blender
	err := ExportCombinedModelToOBJ2(mapModel, players, positions, "export/unit_conversion_test.obj")
	require.NoError(t, err)
}

func TestMultipleTransformationApproaches(t *testing.T) {
	// Load your models
	mapModel, _ := ImportGLTFMapModel("../input_models/de_mirage_model/de_mirage_d.gltf", "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel("../input_models/ctm_sas_model/ctm_sas.gltf")

	// Test positions
	testPositions := []struct {
		name   string
		cs2Pos r3.Vector
	}{
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"B Apartments", r3.Vector{X: 130.04, Y: 130.04, Z: -39.97}},
	}

	// Define transform variations to try
	transformations := []struct {
		name      string
		transform func(r3.Vector) r3.Vector
	}{
		{"Standard", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: v.X / 39.37,
				Y: v.Y / 39.37,
				Z: v.Z / 39.37,
			}
		}},
		{"Invert_X", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: -v.X / 39.37,
				Y: v.Y / 39.37,
				Z: v.Z / 39.37,
			}
		}},
		{"Invert_Y", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: v.X / 39.37,
				Y: -v.Y / 39.37,
				Z: v.Z / 39.37,
			}
		}},
		{"Invert_Both", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: -v.X / 39.37,
				Y: -v.Y / 39.37,
				Z: v.Z / 39.37,
			}
		}},
		{"Swap_XY", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: v.Y / 39.37,
				Y: v.X / 39.37,
				Z: v.Z / 39.37,
			}
		}},
		{"Swap_Invert_X", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: -v.Y / 39.37,
				Y: v.X / 39.37,
				Z: v.Z / 39.37,
			}
		}},
		{"Swap_Invert_Y", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: v.Y / 39.37,
				Y: -v.X / 39.37,
				Z: v.Z / 39.37,
			}
		}},
		{"Swap_Invert_Both", func(v r3.Vector) r3.Vector {
			return r3.Vector{
				X: -v.Y / 39.37,
				Y: -v.X / 39.37,
				Z: v.Z / 39.37,
			}
		}},
	}

	// Export a separate model for each transformation approach
	for _, transform := range transformations {
		players := make([]*Model, len(testPositions))
		positions := make([]r3.Vector, len(testPositions))

		for i, tp := range testPositions {
			players[i] = playerModel
			positions[i] = transform.transform(tp.cs2Pos)
		}

		outputPath := fmt.Sprintf("export/transform_%s.obj", transform.name)
		err := ExportCombinedModelToOBJ2(mapModel, players, positions, outputPath)
		require.NoError(t, err)
	}
}

/*func TransformCS2ToMapModelCoords(gamePos r3.Vector) r3.Vector {
	// Convert from CS2 units to meters (1 meter = 39.37 units)
	const unitsToMeters = 1.0 / 39.37

	// Apply unit conversion first
	metersX := gamePos.X * unitsToMeters
	metersY := gamePos.Y * unitsToMeters
	metersZ := gamePos.Z * unitsToMeters

	// Then apply the offset from your known benchmark position
	// CS2 (units): (-2494.53, 283.32, -104.13)
	// Blender (meters): (7.6258, 62.822, -3.5725)

	// Calculate the remaining offset after unit conversion
	// These values will be much smaller after the unit conversion
	offsetX := 7.6258 - (-2494.53 * unitsToMeters)
	offsetY := 62.822 - (283.32 * unitsToMeters)
	offsetZ := -3.5725 - (-104.13 * unitsToMeters)

	return r3.Vector{
		X: metersX + offsetX,
		Y: metersY + offsetY,
		Z: metersZ + offsetZ,
	}
}*/

func TestExportModelsInPositionsOnMapModel2(t *testing.T) {
	// Load models
	mapModel, _ := ImportGLTFMapModel("../input_models/de_mirage_model/de_mirage_d.gltf", "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel("../input_models/ctm_sas_model/ctm_sas.gltf")

	// B site apartments position in CS2 coordinates
	bSiteApartments := r3.Vector{X: 130.04379272460938, Y: 130.04379272460938, Z: -39.96875}

	// Print model bounds information for debugging
	fmt.Printf("Map Model Bounds: Min: %v, Max: %v\n",
		mapModel.BaseModel.min, mapModel.BaseModel.max)
	fmt.Printf("Player Model Bounds: Min: %v, Max: %v\n",
		playerModel.min, playerModel.max)

	// Export with pre-converted position
	err := ExportMapWithPlayerAtPositionFixed(mapModel, playerModel, bSiteApartments, "export/player_converted_units.obj")
	//err := ExportMapWithPlayerAtCS2Position(mapModel, playerModel, bSiteApartments, "export/player_converted_units.obj")
	require.NoError(t, err)
}

// TestExportModelsInPositionsOnMapModel tests placing player models at specific CS2 positions
func TestExportModelsInPositionsOnMapModel3(t *testing.T) {
	// Import the map model
	testMapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, mapErr := ImportGLTFMapModel(testMapFilePath, "de_mirage")
	if mapErr != nil {
		t.Fatalf("Failed to import map model: %v", mapErr)
	}

	// Import the player model
	testModelFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerModel, modelErr := ImportGLTFPlayerModel(testModelFilePath)
	if modelErr != nil {
		t.Fatalf("Failed to import player model: %v", modelErr)
	}

	// Define test positions in CS2 coordinates
	testPositions := []struct {
		name     string
		position r3.Vector
	}{
		{"B Site Apartments", r3.Vector{X: 130.04379272460938, Y: 130.04379272460938, Z: -39.96875}},
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"Top Mid", r3.Vector{X: 0, Y: 0, Z: 0}}, // Origin/palm tree
	}

	// Export a test file with multiple players
	coords := NewDefaultSource2Coordinates()

	players := make([]*Model, len(testPositions))
	positions := make([]r3.Vector, len(testPositions))

	for i, pos := range testPositions {
		players[i] = playerModel
		// Transform using coordinate system:
		positions[i] = coords.CS2ToModelSpace(pos.position)

		t.Logf("Adding player at position: %s - %v", pos.name, pos.position)
		t.Logf("Transformed to: %v", positions[i])
	}

	t.Logf("Map Model Bounds: Min: %v, Max: %v",
		mapModel.BaseModel.min, mapModel.BaseModel.max)
	t.Logf("Player Model Bounds: Min: %v, Max: %v",
		playerModel.min, playerModel.max)

	// Export to a single OBJ file with all players
	err := ExportCombinedModelToOBJ2(mapModel, players, positions, "export/multiple_positions.obj")
	assert.NoError(t, err, "Should be able to export combined model")

	// Also export individual files for each position for easier testing
	for _, pos := range testPositions {
		outputPath := fmt.Sprintf("export/%s.obj", sanitizeFilename(pos.name))
		err := ExportMapWithPlayerAtPosition(mapModel, playerModel, pos.position, outputPath)
		assert.NoError(t, err, "Should be able to export individual position")
	}
}

// sanitizeFilename replaces unsafe characters in a filename
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "*", "_")
	name = strings.ReplaceAll(name, "?", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "<", "_")
	name = strings.ReplaceAll(name, ">", "_")
	name = strings.ReplaceAll(name, "|", "_")
	return strings.ToLower(name)
}

func TestDebugPlayerPositioning(t *testing.T) {
	// Import the map and player models
	testMapFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, _ := ImportGLTFMapModel(testMapFilePath, "de_mirage")

	testModelFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerModel, _ := ImportGLTFPlayerModel(testModelFilePath)

	// Define a known position
	cs2Position := r3.Vector{X: 130.04379, Y: 130.04379, Z: -39.96875}

	// Create the coordinate transformer
	coords := NewDefaultSource2Coordinates()

	// Get the transformed position
	transformedPosition := coords.CS2ToModelSpace(cs2Position)
	t.Logf("Original CS2 position: %v", cs2Position)
	t.Logf("Transformed position: %v", transformedPosition)

	// Create a temporary file for testing
	tempDir, _ := os.MkdirTemp("", "debug_positioning")
	defer os.RemoveAll(tempDir)
	testOutputPath := filepath.Join(tempDir, "debug_positioning.obj")

	// Export the file with manual positioning
	modelVertices := []r3.Vector{}

	// Add the map vertices first
	file, _ := os.Create(testOutputPath)
	defer file.Close()

	// Add a test vertex at the exact transformed position
	file.WriteString("# Debug positioning test\n")
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", transformedPosition.X, transformedPosition.Y, transformedPosition.Z))

	// Now check the first few vertices of the player model at this position
	for i := 0; i < 3; i++ {
		tri := playerModel.triangles[i]
		v1 := r3.Vector{
			X: tri.V1.X + transformedPosition.X,
			Y: tri.V1.Y + transformedPosition.Y,
			Z: tri.V1.Z + transformedPosition.Z,
		}
		modelVertices = append(modelVertices, v1)
		t.Logf("Player model vertex %d at position: %.6f, %.6f, %.6f", i*3+1, v1.X, v1.Y, v1.Z)
	}

	// Log the actual export function calls that would be used
	t.Logf("About to call ExportMapWithPlayerAtCS2Position with position: %v", cs2Position)

	// Try another way to export the model
	err := ExportMapWithPlayerAtCS2Position(mapModel, playerModel, cs2Position, filepath.Join("export", "debug_position.obj"))
	t.Logf("Export result: %v", err)
}

func TestScaledPlayerPositioning(t *testing.T) {
	// Import the models
	mapModel, _ := ImportGLTFMapModel(filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf"), "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel(filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf"))

	// Define test positions
	positions := []struct {
		name string
		pos  r3.Vector
	}{
		{"B Site", r3.Vector{X: 130.04379, Y: 130.04379, Z: -39.96875}},
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}},
	}

	// Create a Source2Coordinates with a larger scale factor
	coords := &Source2Coordinates{
		UnitScale: 1.0 / 5.0, // Much larger scale (default is 1/39.37)
		SwapAxes:  true,
	}

	// Transform and export
	players := make([]*Model, len(positions))
	scaledPositions := make([]r3.Vector, len(positions))

	for i, pos := range positions {
		players[i] = playerModel
		scaledPos := coords.CS2ToModelSpace(pos.pos)
		scaledPositions[i] = scaledPos
		t.Logf("Position %s: CS2=%v, Scaled=%v", pos.name, pos.pos, scaledPos)
	}

	// Export with scaled positions
	ExportCombinedModelToOBJ2(mapModel, players, scaledPositions, "export/scaled_positions.obj")
}

func TestExtremeScaling(t *testing.T) {
	// Import the models
	mapModel, _ := ImportGLTFMapModel(filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf"), "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel(filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf"))

	// Define test positions
	positions := []struct {
		name string
		pos  r3.Vector
	}{
		{"B Site", r3.Vector{X: 130.04379, Y: 130.04379, Z: -39.96875}},
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}},
	}

	// Try different scaling factors
	scales := []float64{100.0, 500.0, 1000.0, 2000.0}

	for _, scale := range scales {
		players := make([]*Model, len(positions))
		scaledPositions := make([]r3.Vector, len(positions))

		for i, pos := range positions {
			players[i] = playerModel

			// Apply a simpler scaling function with just scale and no axis swapping
			scaledPositions[i] = r3.Vector{
				X: pos.pos.Z / scale, // Use Z for X (up becomes right)
				Y: pos.pos.X / scale, // Use X for Y (forward becomes up)
				Z: pos.pos.Y / scale, // Use Y for Z (left becomes forward)
			}

			t.Logf("Position %s with scale %f: CS2=%v, Scaled=%v",
				pos.name, scale, pos.pos, scaledPositions[i])
		}

		// Export with scaled positions
		outputPath := fmt.Sprintf("export/extreme_scale_%v.obj", scale)
		ExportCombinedModelToOBJ2(mapModel, players, scaledPositions, outputPath)
	}
}

func TestAbsolutePositioning(t *testing.T) {
	// Import the models
	mapModel, _ := ImportGLTFMapModel(filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf"), "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel(filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf"))

	// Get map bounds
	mapSize := r3.Vector{
		X: mapModel.BaseModel.max.X - mapModel.BaseModel.min.X,
		Y: mapModel.BaseModel.max.Y - mapModel.BaseModel.min.Y,
		Z: mapModel.BaseModel.max.Z - mapModel.BaseModel.min.Z,
	}

	t.Logf("Map bounds: min=%v, max=%v, size=%v",
		mapModel.BaseModel.min, mapModel.BaseModel.max, mapSize)

	// Define absolute positions relative to the map geometry
	// Place models at corners and center of the map
	absolutePositions := []r3.Vector{
		// At map min
		mapModel.BaseModel.min,

		// At map max
		mapModel.BaseModel.max,

		// At map center
		{
			X: (mapModel.BaseModel.min.X + mapModel.BaseModel.max.X) / 2,
			Y: (mapModel.BaseModel.min.Y + mapModel.BaseModel.max.Y) / 2,
			Z: (mapModel.BaseModel.min.Z + mapModel.BaseModel.max.Z) / 2,
		},

		// Corners
		{X: mapModel.BaseModel.min.X, Y: mapModel.BaseModel.min.Y, Z: mapModel.BaseModel.max.Z},
		{X: mapModel.BaseModel.min.X, Y: mapModel.BaseModel.max.Y, Z: mapModel.BaseModel.min.Z},
		{X: mapModel.BaseModel.max.X, Y: mapModel.BaseModel.min.Y, Z: mapModel.BaseModel.min.Z},

		// Origin (0,0,0)
		{X: 0, Y: 0, Z: 0},
	}

	// Create players at these positions
	players := make([]*Model, len(absolutePositions))
	for i := range absolutePositions {
		players[i] = playerModel
		t.Logf("Absolute position %d: %v", i+1, absolutePositions[i])
	}

	// Export
	ExportCombinedModelToOBJ2(mapModel, players, absolutePositions, "export/absolute_positions.obj")
}

func TestDirectOBJWriting(t *testing.T) {
	// Create a simple file with known positions
	file, err := os.Create("export/debug_points.obj")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	// Write some metadata
	file.WriteString("# Debug points for coordinate testing\n\n")

	// Define some test points
	points := []struct {
		desc string
		pos  r3.Vector
	}{
		{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}},
		{"X=1", r3.Vector{X: 1, Y: 0, Z: 0}},
		{"Y=1", r3.Vector{X: 0, Y: 1, Z: 0}},
		{"Z=1", r3.Vector{X: 0, Y: 0, Z: 1}},
		{"Corner", r3.Vector{X: 10, Y: 10, Z: 10}},

		// Try CS2 coordinates with scaling
		{"CS2_A", r3.Vector{X: 130.04379 / 10.0, Y: 130.04379 / 10.0, Z: -39.96875 / 10.0}},
		{"CS2_B", r3.Vector{X: -2494.53 / 100.0, Y: 283.32 / 100.0, Z: -104.13 / 100.0}},

		// Try with coordinate swapping
		{"CS2_A_Swapped", r3.Vector{X: -39.96875 / 10.0, Y: 130.04379 / 10.0, Z: 130.04379 / 10.0}},
		{"CS2_B_Swapped", r3.Vector{X: -104.13 / 100.0, Y: -2494.53 / 100.0, Z: 283.32 / 100.0}},
	}

	// Write vertices
	for i, p := range points {
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f # %s\n", p.pos.X, p.pos.Y, p.pos.Z, p.desc))
		t.Logf("Point %d - %s: %v", i+1, p.desc, p.pos)
	}

	// Create lines (connect each point to the origin)
	for i := 2; i <= len(points); i++ {
		file.WriteString(fmt.Sprintf("l 1 %d\n", i))
	}

	t.Logf("Created debug_points.obj with %d points", len(points))
}

func ExportWithAbsoluteOffset(mapModel *MapModel, playerModel *Model, cs2Position r3.Vector, outputPath string) error {
	// Create a modified version of the position with an absolute offset
	// This is just for debugging purposes to get players visible in Blender
	coords := NewDefaultSource2Coordinates()

	// First do the regular transformation
	transformedPosition := coords.CS2ToModelSpace(cs2Position)

	// Then add a large absolute offset to make it visible in Blender
	// This offset is deliberately large to make the player stand out
	offsetPosition := r3.Vector{
		X: transformedPosition.X + 500.0, // Move 500 units in X
		Y: transformedPosition.Y + 500.0, // Move 500 units in Y
		Z: transformedPosition.Z + 100.0, // Move 100 units in Z
	}

	// Log the positions for debugging
	fmt.Printf("CS2 Position: %v\n", cs2Position)
	fmt.Printf("Transformed Position: %v\n", transformedPosition)
	fmt.Printf("Offset Position: %v\n", offsetPosition)

	// Use the offset position for export
	return ExportCombinedModelToOBJ2(mapModel, []*Model{playerModel}, []r3.Vector{offsetPosition}, outputPath)
}

func TestFixedExport(t *testing.T) {
	// Import the models
	mapModel, _ := ImportGLTFMapModel(filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf"), "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel(filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf"))

	// Define positions to test
	positions := []struct {
		name string
		pos  r3.Vector
	}{
		{"B Site", r3.Vector{X: 130.04379, Y: 130.04379, Z: -39.96875}},
		{"Bench", r3.Vector{X: -2494.53, Y: 283.32, Z: -104.13}},
		{"Origin", r3.Vector{X: 0, Y: 0, Z: 0}},
	}

	// Export each position
	for _, p := range positions {
		outputPath := fmt.Sprintf("export/fixed_%s.obj", strings.ReplaceAll(p.name, " ", "_"))
		err := ExportWithAbsoluteOffset(mapModel, playerModel, p.pos, outputPath)
		if err != nil {
			t.Errorf("Failed to export %s: %v", p.name, err)
		} else {
			t.Logf("Exported %s to %s", p.name, outputPath)
		}
	}
}

func TestAxisSwappedPositioning(t *testing.T) {
	// Import the models
	mapModel, _ := ImportGLTFMapModel(filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf"), "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel(filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf"))

	// Define test positions
	testPos := r3.Vector{X: 130.04379, Y: 130.04379, Z: -39.96875}

	// Create modified coordinate transformations
	transformOptions := []struct {
		name string
		fn   func(r3.Vector) r3.Vector
	}{
		{"Default", func(v r3.Vector) r3.Vector {
			return NewDefaultSource2Coordinates().CS2ToModelSpace(v)
		}},
		{"SwapXY", func(v r3.Vector) r3.Vector {
			pos := NewDefaultSource2Coordinates().CS2ToModelSpace(v)
			return r3.Vector{X: pos.Y, Y: pos.X, Z: pos.Z}
		}},
		{"SwapXZ", func(v r3.Vector) r3.Vector {
			pos := NewDefaultSource2Coordinates().CS2ToModelSpace(v)
			return r3.Vector{X: pos.Z, Y: pos.Y, Z: pos.X}
		}},
		{"SwapYZ", func(v r3.Vector) r3.Vector {
			pos := NewDefaultSource2Coordinates().CS2ToModelSpace(v)
			return r3.Vector{X: pos.X, Y: pos.Z, Z: pos.Y}
		}},
		{"AllNegative", func(v r3.Vector) r3.Vector {
			pos := NewDefaultSource2Coordinates().CS2ToModelSpace(v)
			return r3.Vector{X: -pos.X, Y: -pos.Y, Z: -pos.Z}
		}},
		{"Scale10x", func(v r3.Vector) r3.Vector {
			pos := NewDefaultSource2Coordinates().CS2ToModelSpace(v)
			return r3.Vector{X: pos.X * 10, Y: pos.Y * 10, Z: pos.Z * 10}
		}},
	}

	// Export with each transformation
	for _, option := range transformOptions {
		transformedPos := option.fn(testPos)
		t.Logf("Transform %s: %v -> %v", option.name, testPos, transformedPos)

		outputPath := fmt.Sprintf("export/transform_%s.obj", option.name)
		ExportCombinedModelToOBJ2(mapModel, []*Model{playerModel}, []r3.Vector{transformedPos}, outputPath)
	}
}

func ExportMapWithPlayerAtPositionFixed(mapModel *MapModel, playerModel *Model, position r3.Vector, outputPath string) error {
	// Check the player model's coordinates - it might not be centered at (0,0,0)
	playerCenter := r3.Vector{
		X: (playerModel.min.X + playerModel.max.X) / 2,
		Y: (playerModel.min.Y + playerModel.max.Y) / 2,
		Z: playerModel.min.Z, // Use min.Z to place feet on ground
	}

	// Calculate the offset needed to move the player from its center to the desired position
	offset := r3.Vector{
		X: position.X - playerCenter.X,
		Y: position.Y - playerCenter.Y,
		Z: position.Z - playerCenter.Z,
	}

	fmt.Printf("Player model bounds: Min=%v, Max=%v\n", playerModel.min, playerModel.max)
	fmt.Printf("Player center: %v\n", playerCenter)
	fmt.Printf("Target position: %v\n", position)
	fmt.Printf("Applied offset: %v\n", offset)

	// Use this offset when exporting
	return ExportCombinedModelToOBJ2(mapModel, []*Model{playerModel}, []r3.Vector{offset}, outputPath)
}

func TransformCS2ToMapModelCoords(gamePos r3.Vector) r3.Vector {
	// Unit conversion factor (approximate conversion from CS2 units to meters)
	const unitsToMeters = 1.0 / 39.37

	// We might need to swap or invert axes
	// Let's try a few different transformations

	// Option 1: Standard transform with unit conversion
	// return r3.Vector{
	//     X: gamePos.X * unitsToMeters + offsetX,
	//     Y: gamePos.Y * unitsToMeters + offsetY,
	//     Z: gamePos.Z * unitsToMeters + offsetZ,
	// }

	// Option 2: Swap X and Y
	// return r3.Vector{
	//     X: gamePos.Y * unitsToMeters + offsetX,
	//     Y: gamePos.X * unitsToMeters + offsetY,
	//     Z: gamePos.Z * unitsToMeters + offsetZ,
	// }

	// Option 3: Invert X or Y
	return r3.Vector{
		X: -gamePos.X*unitsToMeters + 70, // Try a rough offset to position in the map
		Y: -gamePos.Y*unitsToMeters + 50, // Try a rough offset to position in the map
		Z: gamePos.Z * unitsToMeters,     // Keep the working Z transformation
	}
}

func TestMultipleReferencePoints(t *testing.T) {
	// Load models
	mapModel, _ := ImportGLTFMapModel("../input_models/de_mirage_model/de_mirage_d.gltf", "de_mirage")
	playerModel, _ := ImportGLTFPlayerModel("../input_models/ctm_sas_model/ctm_sas.gltf")

	// Define multiple known positions across the map in CS2 coordinates
	// These should be spread out to cover different areas of the map
	// Format: location name, CS2 coordinates, relative location description
	knownPositions := []struct {
		name        string
		cs2Pos      r3.Vector
		description string
	}{
		{"B Site", r3.Vector{X: 130.04, Y: 130.04, Z: -39.97}, "center of B site"},
		{"T Spawn", r3.Vector{X: -1000, Y: -500, Z: -50}, "approximate T spawn"}, // Replace with actual coordinates
		{"CT Spawn", r3.Vector{X: 500, Y: 1000, Z: -50}, "approximate CT spawn"}, // Replace with actual coordinates
		{"A Site", r3.Vector{X: 200, Y: -200, Z: -40}, "center of A site"},       // Replace with actual coordinates
		{"Mid", r3.Vector{X: 0, Y: 100, Z: -45}, "center of mid area"},           // Replace with actual coordinates
	}

	// Define different scaling factors to try
	scalingFactors := []float64{1.0, 10.0, 50.0, 100.0, 500.0}

	// Try each scaling factor
	for _, scale := range scalingFactors {
		// Create a group of players for this scale factor
		players := make([]*Model, len(knownPositions))
		positions := make([]r3.Vector, len(knownPositions))

		for i, pos := range knownPositions {
			players[i] = playerModel

			// Apply a simple scaling transformation - no complex matrix math yet
			positions[i] = r3.Vector{
				X: pos.cs2Pos.X * scale / 39.37, // Scale and convert to meters
				Y: pos.cs2Pos.Y * scale / 39.37,
				Z: pos.cs2Pos.Z * scale / 39.37,
			}
		}

		// Export with this scaling factor
		outputPath := fmt.Sprintf("export/scale_factor_%v.obj", scale)
		err := ExportCombinedModelToOBJ2(mapModel, players, positions, outputPath)
		require.NoError(t, err)
	}
}

func ExportMapWithPlayerAtCS2PositionWithLargeOffset(mapModel *MapModel, playerModel *Model, cs2Position r3.Vector, outputPath string) error {
	// Get map dimensions for scaling reference
	mapSize := r3.Vector{
		X: mapModel.BaseModel.max.X - mapModel.BaseModel.min.X,
		Y: mapModel.BaseModel.max.Y - mapModel.BaseModel.min.Y,
		Z: mapModel.BaseModel.max.Z - mapModel.BaseModel.min.Z,
	}

	fmt.Printf("Map dimensions: %v\n", mapSize)

	// Try positioning with large offsets using map dimensions as reference
	// These multipliers will need adjustment based on results
	transformedPosition := r3.Vector{
		X: (cs2Position.X / 1000) * mapSize.X * 0.5, // Try using half map width as scale
		Y: (cs2Position.Y / 1000) * mapSize.Y * 0.5, // Try using half map height as scale
		Z: (cs2Position.Z / 100) * mapSize.Z * 0.2,  // Try using 20% of map height for Z
	}

	fmt.Printf("Original CS2 position: %v\n", cs2Position)
	fmt.Printf("Transformed with map scaling: %v\n", transformedPosition)

	// Export with the transformed position
	return ExportMapWithPlayerAtPosition(mapModel, playerModel, transformedPosition, outputPath)
}

// Helper function to create a simple test map model with two triangles
func createTestMapModel() *MapModel {
	mapModel := NewMapModel()

	// Add two triangles with different materials
	tri1 := types.Triangle{
		V1: r3.Vector{X: 0, Y: 0, Z: 0},
		V2: r3.Vector{X: 1, Y: 0, Z: 0},
		V3: r3.Vector{X: 0, Y: 1, Z: 0},
	}

	tri2 := types.Triangle{
		V1: r3.Vector{X: 1, Y: 1, Z: 0},
		V2: r3.Vector{X: 1, Y: 0, Z: 0},
		V3: r3.Vector{X: 0, Y: 1, Z: 0},
	}

	// Create two different materials
	opaqueMaterial := MaterialProperties{
		IsTransparent: false,
		Opacity:       1.0,
		Name:          "opaque_wall",
	}

	transparentMaterial := MaterialProperties{
		IsTransparent: true,
		Opacity:       0.5,
		Name:          "transparent_glass",
	}

	// Add triangles with materials
	mapModel.BaseModel.AddTriangleWithMaterial(tri1, opaqueMaterial)
	mapModel.BaseModel.AddTriangleWithMaterial(tri2, transparentMaterial)

	// Build basic spatial structures for the model
	err := buildSpatialStructures(&mapModel.BaseModel)
	if err != nil {
		// Just log the error but continue, as we don't need spatial structures for export
		// This could happen in test environment with incomplete model data
	}

	return mapModel
}

// Helper function to create a simple test player model with one triangle and hitboxes
func createTestPlayerModel() *Model {
	playerModel := NewModel()

	// Add a triangle
	tri := types.Triangle{
		V1: r3.Vector{X: 0, Y: 0, Z: 0},
		V2: r3.Vector{X: 1, Y: 0, Z: 0},
		V3: r3.Vector{X: 0, Y: 0, Z: 1},
	}

	material := MaterialProperties{
		IsTransparent: false,
		Opacity:       1.0,
		Name:          "player_material",
	}

	// Add triangle with material
	playerModel.AddTriangleWithMaterial(tri, material)

	// Add a hitbox
	hitbox := Hitbox{
		Name:      "head",
		BoneName:  "head_bone",
		Type:      "Box",
		MinBounds: r3.Vector{X: -0.5, Y: -0.5, Z: 1.5},
		MaxBounds: r3.Vector{X: 0.5, Y: 0.5, Z: 2.5},
		Vertices: []r3.Vector{
			{X: -0.5, Y: -0.5, Z: 1.5},
			{X: 0.5, Y: -0.5, Z: 1.5},
			{X: 0.5, Y: 0.5, Z: 1.5},
			{X: -0.5, Y: 0.5, Z: 1.5},
			{X: -0.5, Y: -0.5, Z: 2.5},
			{X: 0.5, Y: -0.5, Z: 2.5},
			{X: 0.5, Y: 0.5, Z: 2.5},
			{X: -0.5, Y: 0.5, Z: 2.5},
		},
	}

	// Add a capsule hitbox
	hitbox2 := Hitbox{
		Name:      "torso",
		BoneName:  "spine_bone",
		Type:      "Capsule",
		MinBounds: r3.Vector{X: -0.75, Y: -0.5, Z: 0.0},
		MaxBounds: r3.Vector{X: 0.75, Y: 0.5, Z: 1.5},
		Vertices: []r3.Vector{
			{X: 0, Y: 0, Z: 0.75},     // Center
			{X: 0, Y: 0, Z: 0.0},      // Bottom
			{X: 0, Y: 0, Z: 1.5},      // Top
			{X: 0.75, Y: 0, Z: 0.75},  // Right
			{X: -0.75, Y: 0, Z: 0.75}, // Left
			{X: 0, Y: 0.5, Z: 0.75},   // Front
			{X: 0, Y: -0.5, Z: 0.75},  // Back
		},
	}

	playerModel.hitboxes = append(playerModel.hitboxes, hitbox, hitbox2)

	// Build spatial structures
	err := buildSpatialStructures(playerModel)
	if err != nil {
		// Just log the error but continue
	}

	return playerModel
}

// Helper function to create a large map model with many triangles
func createLargeMapModel(numTriangles int) *MapModel {
	mapModel := NewMapModel()

	opaqueMaterial := MaterialProperties{
		IsTransparent: false,
		Opacity:       1.0,
		Name:          "wall_material",
	}

	transparentMaterial := MaterialProperties{
		IsTransparent: true,
		Opacity:       0.6,
		Name:          "glass_material",
	}

	// Create a grid of triangles
	for i := 0; i < numTriangles; i++ {
		// Generate a triangle with slight variations
		x := float64(i % 100)
		z := float64(i / 100)

		tri := types.Triangle{
			V1: r3.Vector{X: x, Y: 0, Z: z},
			V2: r3.Vector{X: x + 1, Y: 0, Z: z},
			V3: r3.Vector{X: x, Y: 0, Z: z + 1},
		}

		// Alternate between materials
		if i%2 == 0 {
			mapModel.BaseModel.AddTriangleWithMaterial(tri, opaqueMaterial)
		} else {
			mapModel.BaseModel.AddTriangleWithMaterial(tri, transparentMaterial)
		}
	}

	return mapModel
}
