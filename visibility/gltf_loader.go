package visibility

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"

	"github.com/golang/geo/r3"
	"github.com/qmuntal/gltf"
	"github.com/richardkiene/cs2analyst/types"
)

// ImportGLTFPlayerModel loads a GLTF file and transforms it into a Model
// appropriate for use with the CS2 (Source2) engine coordinate system.
func ImportGLTFPlayerModel(filePath string) (*Model, error) {
	// Create a new empty model
	model := NewModel()

	// Load the GLTF document
	doc, err := gltf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GLTF file: %v", err)
	}

	slog.Info("Loading GLTF model", "path", filePath, "nodes", len(doc.Nodes), "meshes", len(doc.Meshes))

	// Process each mesh in the GLTF file
	for _, mesh := range doc.Meshes {
		for _, primitive := range mesh.Primitives {
			// Extract triangles from the primitive's geometry
			triangles, err := extractTrianglesFromPrimitive(doc, *primitive, filepath.Dir(filePath))
			if err != nil {
				return nil, fmt.Errorf("failed to extract triangles: %v", err)
			}

			// Transform triangles to Source2 coordinate system and add to model
			for _, tri := range triangles {
				transformedTri := transformTriangleToSource2(tri)
				model.AddTriangleForTest(transformedTri)
			}
		}
	}

	// Process nodes to extract hitboxes
	hitboxes, err := extractHitboxes(doc)
	if err != nil {
		slog.Warn("Failed to extract hitboxes", "error", err)
		// Continue without hitboxes - they're not strictly required
	} else {
		// Transform hitboxes to Source2 coordinate system
		for i := range hitboxes {
			hitboxes[i] = transformHitboxToSource2(hitboxes[i])
		}
		// Set hitboxes on the model
		model.hitboxes = hitboxes
	}

	// Build the spatial acceleration structures
	err = buildSpatialStructures(model)
	if err != nil {
		return nil, fmt.Errorf("failed to build spatial structures: %v", err)
	}

	slog.Info("GLTF model loaded", "triangles", len(model.TrianglesRaw()))
	return model, nil
}

// readIndices reads indices from GLTF buffers
// ImportGLTFMapModel loads a GLTF file for a map and transforms it into a Model
// appropriate for use with the CS2 (Source2) engine coordinate system.
// Maps don't have hitboxes but may contain additional map-specific information.
func ImportGLTFMapModel(filePath string, mapName string) (*MapModel, error) {
	// Create a new empty model
	mapModel := NewMapModel()

	// Load the GLTF document
	doc, err := gltf.Open(filePath)
	fmt.Print(doc.Asset)
	if err != nil {
		return nil, fmt.Errorf("failed to open GLTF file %s -- %v", filePath, err)
	}

	slog.Info("Loading GLTF map model", "path", filePath, "mapName", mapName,
		"nodes", len(doc.Nodes), "meshes", len(doc.Meshes))

	// Process each mesh in the GLTF file
	for _, mesh := range doc.Meshes {
		for _, primitive := range mesh.Primitives {
			// Extract triangles from the primitive's geometry
			triangles, err := extractTrianglesFromPrimitive(doc, *primitive, filepath.Dir(filePath))
			if err != nil {
				return nil, fmt.Errorf("failed to extract triangles: %v", err)
			}

			// Transform triangles to Source2 coordinate system and add to model
			for _, tri := range triangles {
				transformedTri := transformTriangleToSource2(tri)
				mapModel.BaseModel.AddTriangleForTest(transformedTri)
			}
		}
	}

	// Extract map-specific information
	err = extractMapInformation(doc, mapModel, filepath.Dir(filePath))
	if err != nil {
		slog.Warn("Failed to extract all map information", "error", err)
		// Continue without complete map info - it's not strictly required
	}

	// Build the spatial acceleration structures
	err = buildSpatialStructures(&mapModel.BaseModel)
	if err != nil {
		return nil, fmt.Errorf("failed to build spatial structures: %v", err)
	}

	// For maps, we might want to add additional optimizations for the spatial structure
	optimizeMapSpatialStructure(mapModel)

	slog.Info("GLTF map model loaded", "triangles", len(mapModel.BaseModel.TrianglesRaw()),
		"mapName", mapName)
	return mapModel, nil
}

// extractMapInformation processes the GLTF document to extract map-specific information
// such as bombsites, spawn points, nav mesh data, etc.
func extractMapInformation(doc *gltf.Document, mapModel *MapModel, mapModelDir string) error {
	// First look for map-specific data in extensions or extras
	for _, node := range doc.Nodes {
		// Look for named nodes that might contain map-specific information
		if node.Name == "" {
			continue
		}

		// Look for bombsites (nodes named "bombsite_a", "bombsite_b", etc.)
		if len(node.Name) > 9 && node.Name[:9] == "bombsite_" {
			// Extract bombsite information
			// This is simplified - in practice you'd extract precise bounds
			processBombsiteNode(*node, mapModel)
		}

		// Look for spawn points (nodes named "spawn_t_1", "spawn_ct_1", etc.)
		if len(node.Name) > 6 && node.Name[:6] == "spawn_" {
			processSpawnPointNode(*node, mapModel)
		}

		// Look for nav mesh data (nodes named "nav_mesh" or similar)
		if node.Name == "nav_mesh" || node.Name == "navmesh" {
			processNavMeshNode(doc, *node, mapModel, mapModelDir)
		}

		// Look for named areas (nodes with names like "area_mid", "area_long", etc.)
		if len(node.Name) > 5 && node.Name[:5] == "area_" {
			processNamedAreaNode(*node, mapModel)
		}
	}

	// Try to determine map scale and adjust the model's grid size accordingly
	determineMapScale(mapModel)

	return nil
}

// processBombsiteNode extracts bombsite information from a GLTF node
func processBombsiteNode(node gltf.Node, mapModel *MapModel) {
	// Extract position if available
	if node.Translation != [3]float64{} {
		pos := r3.Vector{
			X: float64(node.Translation[0]),
			Y: float64(node.Translation[1]),
			Z: float64(node.Translation[2]),
		}

		// Transform position to Source2 coordinates
		transformedPos := transformVertexToSource2(pos)

		// Store bombsite position in the model
		// Note: You would need to add a field to your Model struct for this
		// model.bombsites = append(model.bombsites, transformedPos)

		slog.Info("Found bombsite", "name", node.Name, "position", transformedPos)
	}
}

// processSpawnPointNode extracts spawn point information from a GLTF node
func processSpawnPointNode(node gltf.Node, mapModel *MapModel) {
	// Extract position if available
	if node.Translation != [3]float64{} {
		pos := r3.Vector{
			X: float64(node.Translation[0]),
			Y: float64(node.Translation[1]),
			Z: float64(node.Translation[2]),
		}

		// Transform position to Source2 coordinates
		transformedPos := transformVertexToSource2(pos)

		// Determine team based on name (e.g., "spawn_t_1" for Terrorist)
		isT := len(node.Name) > 7 && node.Name[6] == 't'
		team := "CT"
		if isT {
			team = "T"
		}

		// Store spawn point in the model
		// Note: You would need to add a field to your Model struct for this
		// model.spawnPoints = append(model.spawnPoints, SpawnPoint{
		//     Position: transformedPos,
		//     Team:     team,
		// })

		slog.Info("Found spawn point", "name", node.Name, "team", team, "position", transformedPos)
	}
}

// processNavMeshNode extracts navigation mesh information from a GLTF node
func processNavMeshNode(doc *gltf.Document, node gltf.Node, mapModel *MapModel, mapModelDir string) {
	// Skip if node doesn't have a mesh
	if node.Mesh == nil {
		return
	}

	// Process the mesh as potential nav mesh data
	mesh := doc.Meshes[*node.Mesh]
	for _, primitive := range mesh.Primitives {
		// Check if we have POSITION attribute
		posIdx, ok := primitive.Attributes["POSITION"]
		if !ok {
			continue
		}

		// Extract positions
		positions, err := readPositions(doc, *doc.Accessors[posIdx], mapModelDir)
		if err != nil {
			slog.Warn("Failed to read navmesh positions", "error", err)
			continue
		}

		// TODO: would process the nav mesh data
		// TODO: This might involve extracting walkable surfaces, computing connecting paths, etc.

		slog.Info("Processed nav mesh data", "vertices", len(positions))

		// Store nav mesh in the model
		mapModel.navMeshVertices = positions
	}
}

// processNamedAreaNode extracts named area information from a GLTF node
func processNamedAreaNode(node gltf.Node, model *MapModel) {
	// Extract position and bounds if available
	if node.Translation != [3]float64{} {
		pos := r3.Vector{
			X: float64(node.Translation[0]),
			Y: float64(node.Translation[1]),
			Z: float64(node.Translation[2]),
		}

		// Transform position to Source2 coordinates
		transformedPos := transformVertexToSource2(pos)

		// Extract area name from the node name (e.g., "area_mid" -> "mid")
		areaName := node.Name[5:] // Skip "area_" prefix

		// Store named area in the model
		model.namedAreas = append(model.namedAreas, NamedArea{
			Name:     areaName,
			Position: transformedPos,
		})

		slog.Info("Found named area", "name", areaName, "position", transformedPos)
	}
}

// determineMapScale analyzes the model geometry to determine an appropriate scale
// and adjust the grid size accordingly
func determineMapScale(mapModel *MapModel) {
	// Get the overall size of the map
	size := r3.Vector{
		X: mapModel.BaseModel.max.X - mapModel.BaseModel.min.X,
		Y: mapModel.BaseModel.max.Y - mapModel.BaseModel.min.Y,
		Z: mapModel.BaseModel.max.Z - mapModel.BaseModel.min.Z,
	}

	// Calculate diagonal size
	diagonalSize := math.Sqrt(size.X*size.X + size.Y*size.Y + size.Z*size.Z)

	// Adjust grid size based on map size
	// This is a heuristic - you may want to tune these values
	if diagonalSize > 10000 {
		mapModel.BaseModel.gridSize = 128.0 // Very large map
	} else if diagonalSize > 5000 {
		mapModel.BaseModel.gridSize = 64.0 // Large map
	} else if diagonalSize > 2000 {
		mapModel.BaseModel.gridSize = 32.0 // Medium map
	} else {
		mapModel.BaseModel.gridSize = 16.0 // Small map
	}

	slog.Info("Map scale determined",
		"size", size,
		"diagonal", diagonalSize,
		"gridSize", mapModel.BaseModel.gridSize)
}

// optimizeMapSpatialStructure applies additional optimizations to the spatial structure
// that are specific to maps (as opposed to character models)
func optimizeMapSpatialStructure(mapModel *MapModel) {
	// Count triangles per grid cell
	trianglesPerCell := make(map[GridKey]int)
	cellsWithTriangles := 0
	emptyGridCells := 0

	for key, triangles := range mapModel.BaseModel.sectors {
		count := len(triangles)
		trianglesPerCell[key] = count

		if count > 0 {
			cellsWithTriangles++
		} else {
			emptyGridCells++
		}
	}

	// Calculate statistics
	maxTriangles := 0
	totalTriangles := 0
	for _, count := range trianglesPerCell {
		totalTriangles += count
		if count > maxTriangles {
			maxTriangles = count
		}
	}

	// Log optimization stats
	if cellsWithTriangles > 0 {
		avgTriangles := float64(totalTriangles) / float64(cellsWithTriangles)
		slog.Info("Map spatial structure optimized",
			"totalCells", cellsWithTriangles+emptyGridCells,
			"cellsWithTriangles", cellsWithTriangles,
			"emptyGridCells", emptyGridCells,
			"avgTrianglesPerCell", avgTriangles,
			"maxTrianglesInCell", maxTriangles)
	}

	// Here you could implement additional optimizations like:
	// - Adaptive grid cell sizes in different parts of the map
	// - Selective BVH construction for dense areas
	// - Out-of-core techniques for very large maps
}

// extractTrianglesFromPrimitive extracts triangles from a GLTF primitive
func extractTrianglesFromPrimitive(doc *gltf.Document, primitive gltf.Primitive, modelDir string) ([]types.Triangle, error) {
	var triangles []types.Triangle

	// We need positions and indices to extract triangles
	if primitive.Indices == nil {
		return nil, fmt.Errorf("primitive has no indices")
	}

	// Get position accessor
	posAccessorIdx, ok := primitive.Attributes["POSITION"]
	if !ok {
		return nil, fmt.Errorf("primitive has no POSITION attribute")
	}
	posAccessor := doc.Accessors[posAccessorIdx]

	// Get vertex positions by reading the buffer data directly
	positions, err := readPositions(doc, *posAccessor, modelDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read positions: %v", err)
	}

	// Get indices by reading the buffer data directly
	indices, err := readIndices(doc, *doc.Accessors[*primitive.Indices], modelDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read indices: %v", err)
	}

	// Process triangles based on indices
	for i := 0; i < len(indices); i += 3 {
		if i+2 < len(indices) {
			// Get the three vertices of the triangle
			idx1, idx2, idx3 := indices[i], indices[i+1], indices[i+2]

			// Ensure indices are within bounds
			if int(idx1) >= len(positions) || int(idx2) >= len(positions) || int(idx3) >= len(positions) {
				continue // Skip invalid triangles
			}

			// Create vectors for each vertex
			v1 := r3.Vector{
				X: positions[idx1][0],
				Y: positions[idx1][1],
				Z: positions[idx1][2],
			}

			v2 := r3.Vector{
				X: positions[idx2][0],
				Y: positions[idx2][1],
				Z: positions[idx2][2],
			}

			v3 := r3.Vector{
				X: positions[idx3][0],
				Y: positions[idx3][1],
				Z: positions[idx3][2],
			}

			// Create and add triangle
			triangle := types.Triangle{V1: v1, V2: v2, V3: v3}
			triangles = append(triangles, triangle)
		}
	}

	return triangles, nil
}

// readPositions reads vertex positions from GLTF buffers
func readPositions(doc *gltf.Document, accessor gltf.Accessor, modelDir string) ([][3]float64, error) {
	// Validate accessor
	if accessor.Type != gltf.AccessorVec3 {
		return nil, fmt.Errorf("position accessor must be VEC3")
	}
	if accessor.ComponentType != gltf.ComponentFloat {
		return nil, fmt.Errorf("position components must be floats")
	}

	// Get buffer view
	bufferView := doc.BufferViews[*accessor.BufferView]

	// Get buffer
	buffer := doc.Buffers[bufferView.Buffer]

	// Get buffer data
	var data []byte
	if buffer.URI != "" {
		// For external URI (assuming local file)
		if buffer.URI[:5] == "data:" {
			// Data URI - not handling in this simplified version
			return nil, fmt.Errorf("data URIs not supported")
		} else {
			// File URI
			var err error
			data, err = os.ReadFile(filepath.Join(modelDir, buffer.URI))
			if err != nil {
				return nil, fmt.Errorf("failed to read buffer file: %v", err)
			}
		}
	} else {
		// Embedded buffer
		data = buffer.Data
	}

	// Extract the relevant portion of the buffer
	offset := bufferView.ByteOffset + accessor.ByteOffset
	stride := bufferView.ByteStride
	if stride == 0 {
		// When stride is 0, it means tightly packed (i.e., stride = size of component type * num components)
		stride = 3 * 4 // 3 components (vec3) * 4 bytes (float32)
	}

	// Create the positions array
	positions := make([][3]float64, accessor.Count)

	// Read each position
	for i := 0; i < int(accessor.Count); i++ {
		idx := uint32(offset) + uint32(i)*uint32(stride)
		if idx+11 >= uint32(len(data)) {
			return nil, fmt.Errorf("buffer overrun when reading positions")
		}

		// Read 3 float32 values and convert to float64
		for j := 0; j < 3; j++ {
			// Read 4 bytes for float32
			bytes := data[idx+uint32(j*4) : idx+uint32(j*4+4)]

			// Convert bytes to float32
			bits := uint32(bytes[0]) | uint32(bytes[1])<<8 | uint32(bytes[2])<<16 | uint32(bytes[3])<<24
			float := math.Float32frombits(bits)

			// Store as float64
			positions[i][j] = float64(float)
		}
	}

	return positions, nil
}

// readIndices reads indices from GLTF buffers
func readIndices(doc *gltf.Document, accessor gltf.Accessor, modelDir string) ([]uint32, error) {
	// Get buffer view
	bufferView := doc.BufferViews[*accessor.BufferView]

	// Get buffer
	buffer := doc.Buffers[bufferView.Buffer]

	// Get buffer data
	var data []byte
	if buffer.URI != "" {
		// For external URI (assuming local file)
		if buffer.URI[:5] == "data:" {
			// Data URI - not handling in this simplified version
			return nil, fmt.Errorf("data URIs not supported")
		} else {
			// File URI
			var err error
			data, err = os.ReadFile(filepath.Join(modelDir, buffer.URI))
			if err != nil {
				return nil, fmt.Errorf("failed to read buffer file: %v", err)
			}
		}
	} else {
		// Embedded buffer
		data = buffer.Data
	}

	// Extract the relevant portion of the buffer
	offset := bufferView.ByteOffset + accessor.ByteOffset
	bytesPerComponent := 0

	// Determine bytes per component based on component type
	switch accessor.ComponentType {
	case gltf.ComponentUshort:
		bytesPerComponent = 2
	case gltf.ComponentUint:
		bytesPerComponent = 4
	case gltf.ComponentUbyte:
		bytesPerComponent = 1
	default:
		return nil, fmt.Errorf("unsupported index component type: %d", accessor.ComponentType)
	}

	stride := bufferView.ByteStride
	if stride == 0 {
		// When stride is 0, it means tightly packed
		stride = bytesPerComponent
	}

	// Create the indices array
	indices := make([]uint32, accessor.Count)

	// Read each index
	for i := 0; i < int(accessor.Count); i++ {
		idx := uint32(offset) + uint32(i)*uint32(stride)
		if idx+uint32(bytesPerComponent) > uint32(len(data)) {
			return nil, fmt.Errorf("buffer overrun when reading indices")
		}

		// Read index based on component type
		var val uint32
		switch accessor.ComponentType {
		case gltf.ComponentUshort:
			// Read 2 bytes for uint16
			bytes := data[idx : idx+2]
			val = uint32(bytes[0]) | uint32(bytes[1])<<8
		case gltf.ComponentUint:
			// Read 4 bytes for uint32
			bytes := data[idx : idx+4]
			val = uint32(bytes[0]) | uint32(bytes[1])<<8 | uint32(bytes[2])<<16 | uint32(bytes[3])<<24
		case gltf.ComponentUbyte:
			// Read 1 byte for uint8
			val = uint32(data[idx])
		}

		indices[i] = val
	}

	return indices, nil
}

// transformTriangleToSource2 converts a triangle from standard GLTF coordinates to Source2 coordinates
// GLTF standard: Y up, Z forward, X right
// Source2: Z up, X forward/East, Y left/North
func transformTriangleToSource2(tri types.Triangle) types.Triangle {
	// Transform each vertex
	v1 := transformVertexToSource2(tri.V1)
	v2 := transformVertexToSource2(tri.V2)
	v3 := transformVertexToSource2(tri.V3)

	return types.Triangle{V1: v1, V2: v2, V3: v3}
}

// transformVertexToSource2 converts a vertex from standard GLTF coordinates to Source2 coordinates
func transformVertexToSource2(v r3.Vector) r3.Vector {
	// GLTF -> Source2 transformation:
	// Source2.X = GLTF.Z (GLTF Z forward becomes Source2 X forward/East)
	// Source2.Y = -GLTF.X (GLTF X right becomes Source2 Y left/North, so negate)
	// Source2.Z = GLTF.Y (GLTF Y up becomes Source2 Z up)
	return r3.Vector{
		X: v.Z,  // Forward/East in Source2 is Z in GLTF
		Y: -v.X, // Left/North in Source2 is negative X in GLTF
		Z: v.Y,  // Up in Source2 is Y in GLTF
	}
}

// extractHitboxes extracts hitboxes from the GLTF model
func extractHitboxes(doc *gltf.Document) ([]Hitbox, error) {
	var hitboxes []Hitbox

	// Look for nodes with mesh references and bounding boxes
	for i, node := range doc.Nodes {
		if node.Mesh != nil {
			// Create a basic hitbox from the node's bounding box or mesh
			hitbox, err := createHitboxFromNode(doc, *node, i)
			if err != nil {
				continue // Skip nodes where we can't create hitboxes
			}
			hitboxes = append(hitboxes, hitbox)
		}
	}

	return hitboxes, nil
}

// createHitboxFromNode creates a hitbox from a GLTF node
func createHitboxFromNode(doc *gltf.Document, node gltf.Node, nodeIndex int) (Hitbox, error) {
	mesh := doc.Meshes[*node.Mesh]

	// Initialize with extreme values
	minBounds := r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
	maxBounds := r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

	// Process each primitive to find overall bounds
	for _, primitive := range mesh.Primitives {
		posAccessorIdx, ok := primitive.Attributes["POSITION"]
		if !ok {
			continue
		}
		posAccessor := doc.Accessors[posAccessorIdx]

		// Update bounds from accessor min/max if available
		if posAccessor.Min != nil && posAccessor.Max != nil {
			min := r3.Vector{
				X: float64(posAccessor.Min[0]),
				Y: float64(posAccessor.Min[1]),
				Z: float64(posAccessor.Min[2]),
			}
			max := r3.Vector{
				X: float64(posAccessor.Max[0]),
				Y: float64(posAccessor.Max[1]),
				Z: float64(posAccessor.Max[2]),
			}

			// Update global bounds
			minBounds.X = math.Min(minBounds.X, min.X)
			minBounds.Y = math.Min(minBounds.Y, min.Y)
			minBounds.Z = math.Min(minBounds.Z, min.Z)

			maxBounds.X = math.Max(maxBounds.X, max.X)
			maxBounds.Y = math.Max(maxBounds.Y, max.Y)
			maxBounds.Z = math.Max(maxBounds.Z, max.Z)
		}
	}

	// Create the hitbox - use a default box type
	name := fmt.Sprintf("hitbox_%d", nodeIndex)
	if node.Name != "" {
		name = node.Name
	}

	hitbox := Hitbox{
		Name:      name,
		BoneName:  name, // Simplification - in practice you'd want actual bone mapping
		MinBounds: minBounds,
		MaxBounds: maxBounds,
		Type:      "Box", // Default to box type
	}

	// Generate vertices for the box
	vertices := []r3.Vector{
		{X: minBounds.X, Y: minBounds.Y, Z: minBounds.Z},
		{X: minBounds.X, Y: minBounds.Y, Z: maxBounds.Z},
		{X: minBounds.X, Y: maxBounds.Y, Z: minBounds.Z},
		{X: minBounds.X, Y: maxBounds.Y, Z: maxBounds.Z},
		{X: maxBounds.X, Y: minBounds.Y, Z: minBounds.Z},
		{X: maxBounds.X, Y: minBounds.Y, Z: maxBounds.Z},
		{X: maxBounds.X, Y: maxBounds.Y, Z: minBounds.Z},
		{X: maxBounds.X, Y: maxBounds.Y, Z: maxBounds.Z},
	}
	hitbox.Vertices = vertices

	return hitbox, nil
}

// transformHitboxToSource2 transforms a hitbox from GLTF to Source2 coordinates
func transformHitboxToSource2(hitbox Hitbox) Hitbox {
	// Transform bounds
	minBounds := transformVertexToSource2(hitbox.MinBounds)
	maxBounds := transformVertexToSource2(hitbox.MaxBounds)

	// Ensure min is actually min and max is actually max after transformation
	// since the coordinate transformation can swap components
	correctMinBounds := r3.Vector{
		X: math.Min(minBounds.X, maxBounds.X),
		Y: math.Min(minBounds.Y, maxBounds.Y),
		Z: math.Min(minBounds.Z, maxBounds.Z),
	}

	correctMaxBounds := r3.Vector{
		X: math.Max(minBounds.X, maxBounds.X),
		Y: math.Max(minBounds.Y, maxBounds.Y),
		Z: math.Max(minBounds.Z, maxBounds.Z),
	}

	// Transform vertices
	var transformedVertices []r3.Vector
	for _, vertex := range hitbox.Vertices {
		transformedVertices = append(transformedVertices, transformVertexToSource2(vertex))
	}

	// Create transformed hitbox
	return Hitbox{
		Name:      hitbox.Name,
		BoneName:  hitbox.BoneName,
		MinBounds: correctMinBounds,
		MaxBounds: correctMaxBounds,
		Vertices:  transformedVertices,
		Type:      hitbox.Type,
	}
}

// buildSpatialStructures creates spatial acceleration structures for the model
func buildSpatialStructures(model *Model) error {
	triangles := model.TrianglesRaw()

	// Skip if no triangles
	if len(triangles) == 0 {
		return fmt.Errorf("no triangles in model")
	}

	// Build the spatial grid
	return buildSpatialGrid(model, triangles)
}

// buildSpatialGrid organizes triangles in a spatial grid for faster lookups
func buildSpatialGrid(model *Model, triangles []types.Triangle) error {
	// Calculate model bounds
	min := r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
	max := r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

	for _, tri := range triangles {
		// Update min/max for each vertex
		min.X = math.Min(min.X, math.Min(tri.V1.X, math.Min(tri.V2.X, tri.V3.X)))
		min.Y = math.Min(min.Y, math.Min(tri.V1.Y, math.Min(tri.V2.Y, tri.V3.Y)))
		min.Z = math.Min(min.Z, math.Min(tri.V1.Z, math.Min(tri.V2.Z, tri.V3.Z)))

		max.X = math.Max(max.X, math.Max(tri.V1.X, math.Max(tri.V2.X, tri.V3.X)))
		max.Y = math.Max(max.Y, math.Max(tri.V1.Y, math.Max(tri.V2.Y, tri.V3.Y)))
		max.Z = math.Max(max.Z, math.Max(tri.V1.Z, math.Max(tri.V2.Z, tri.V3.Z)))
	}

	// Add some padding
	const padding = 1.0
	min.X -= padding
	min.Y -= padding
	min.Z -= padding
	max.X += padding
	max.Y += padding
	max.Z += padding

	// Set model bounds
	model.min = min
	model.max = max

	// Use the default grid size from the model
	gridSize := model.gridSize

	// Add triangles to grid cells they intersect
	for _, tri := range triangles {
		// Find grid cells this triangle might touch
		minCellX := int(math.Floor(math.Min(math.Min(tri.V1.X, tri.V2.X), tri.V3.X) / gridSize))
		minCellY := int(math.Floor(math.Min(math.Min(tri.V1.Y, tri.V2.Y), tri.V3.Y) / gridSize))
		minCellZ := int(math.Floor(math.Min(math.Min(tri.V1.Z, tri.V2.Z), tri.V3.Z) / gridSize))

		maxCellX := int(math.Floor(math.Max(math.Max(tri.V1.X, tri.V2.X), tri.V3.X) / gridSize))
		maxCellY := int(math.Floor(math.Max(math.Max(tri.V1.Y, tri.V2.Y), tri.V3.Y) / gridSize))
		maxCellZ := int(math.Floor(math.Max(math.Max(tri.V1.Z, tri.V2.Z), tri.V3.Z) / gridSize))

		// Add triangle to each cell it intersects
		for x := minCellX; x <= maxCellX; x++ {
			for y := minCellY; y <= maxCellY; y++ {
				for z := minCellZ; z <= maxCellZ; z++ {
					key := GridKey{x: x, y: y, z: z}
					model.sectors[key] = append(model.sectors[key], tri)
				}
			}
		}
	}

	return nil
}
