package visibility

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/geo/r3"
	"github.com/qmuntal/gltf"
	"github.com/richardkiene/cs2analyst/types"
)

// MaterialProperties stores material properties relevant for visibility checks
type MaterialProperties struct {
	IsTransparent   bool    // Whether the material allows seeing through it
	Opacity         float64 // 0.0 = fully transparent, 1.0 = fully opaque
	RefractionIndex float64 // For transparent materials
	Name            string  // Material name
}

// Triangle with material properties
type MaterialTriangle struct {
	Triangle types.Triangle
	Material MaterialProperties
}

// Node for building the BVH tree
type BVHNode struct {
	bbox      AABB
	left      *BVHNode
	right     *BVHNode
	triangles []types.Triangle
	materials []MaterialProperties // Materials corresponding to triangles
}

type AABB struct {
	Min, Max r3.Vector
}

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

	// Extract materials first
	materials := extractMaterials(doc)

	// Process each mesh in the GLTF file
	var allTriangles []types.Triangle
	var triangleMaterials []MaterialProperties

	for _, mesh := range doc.Meshes {
		for _, primitive := range mesh.Primitives {
			// Extract triangles from the primitive's geometry
			triangles, matProps, err := extractTrianglesFromPrimitive(doc, *primitive, materials, filepath.Dir(filePath))
			if err != nil {
				slog.Warn("Failed to extract triangles from primitive", "error", err)
				continue // Continue with other primitives instead of failing completely
			}

			// Transform triangles to Source2 coordinate system and add to model
			for i, tri := range triangles {
				transformedTri := transformTriangleToSource2(tri)
				allTriangles = append(allTriangles, transformedTri)
				triangleMaterials = append(triangleMaterials, matProps[i])
			}
		}
	}

	// Add all triangles to the model
	for i, tri := range allTriangles {
		model.AddTriangleWithMaterial(tri, triangleMaterials[i])
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

// ImportGLTFMapModel loads a GLTF file for a map and transforms it into a Model
// appropriate for use with the CS2 (Source2) engine coordinate system.
func ImportGLTFMapModel(filePath string, mapName string) (*MapModel, error) {
	// Create a new empty model
	mapModel := NewMapModel()

	// Load the GLTF document
	doc, err := gltf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GLTF file %s: %v", filePath, err)
	}

	slog.Info("Loading GLTF map model", "path", filePath, "mapName", mapName,
		"nodes", len(doc.Nodes), "meshes", len(doc.Meshes))

	// Extract materials first
	materials := extractMaterials(doc)

	// Process each mesh in the GLTF file
	var allTriangles []types.Triangle
	var triangleMaterials []MaterialProperties

	for _, mesh := range doc.Meshes {
		for _, primitive := range mesh.Primitives {
			// Extract triangles from the primitive's geometry
			triangles, matProps, err := extractTrianglesFromPrimitive(doc, *primitive, materials, filepath.Dir(filePath))
			if err != nil {
				slog.Warn("Failed to extract triangles from primitive", "error", err, "mesh", mesh.Name)
				continue // Skip problematic primitives instead of failing
			}

			// Transform triangles to Source2 coordinate system and add to model
			for i, tri := range triangles {
				transformedTri := transformTriangleToSource2(tri)
				allTriangles = append(allTriangles, transformedTri)
				triangleMaterials = append(triangleMaterials, matProps[i])
			}
		}
	}

	// Add all triangles to the map model
	for i, tri := range allTriangles {
		mapModel.BaseModel.AddTriangleWithMaterial(tri, triangleMaterials[i])
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

// extractMaterials processes GLTF materials and converts them to our MaterialProperties
func extractMaterials(doc *gltf.Document) []MaterialProperties {
	materials := make([]MaterialProperties, len(doc.Materials))

	for i, material := range doc.Materials {
		// Default material properties
		matProps := MaterialProperties{
			IsTransparent:   false,
			Opacity:         1.0,
			RefractionIndex: 1.0,
			Name:            material.Name,
		}

		// Extract transparency/opacity from the material if available
		if material.PBRMetallicRoughness != nil && material.PBRMetallicRoughness.BaseColorFactor != nil {
			// GLTF uses the alpha channel of baseColorFactor for opacity
			if len(material.PBRMetallicRoughness.BaseColorFactor) >= 4 {
				opacity := material.PBRMetallicRoughness.BaseColorFactor[3]
				matProps.Opacity = opacity

				// Consider materials with opacity < 1.0 as transparent
				if opacity < 0.99 {
					matProps.IsTransparent = true
				}
			}
		}

		// Check alpha mode
		if material.AlphaMode == gltf.AlphaBlend || material.AlphaMode == gltf.AlphaMask {
			matProps.IsTransparent = true
		}

		// Check for special material names that might indicate windows, grates, etc.
		lowerName := material.Name
		if material.Name != "" {
			for _, term := range []string{
				"glass", "window", "grate", "fence", "transparent", "water",
				"chain", "net", "screen", "mesh", "grid", "bars", "rail",
				"railing", "cage", "grille", "lattice", "mirror", "reflective",
				"crystal", "clear",
			} {
				if contains(lowerName, term) {
					matProps.IsTransparent = true
					break
				}
			}
		}

		// Manually mark specific known materials as transparent
		if contains(material.Name, "residwall04a") || // Mirage specific
			contains(material.Name, "urban_fence") ||
			contains(material.Name, "chainlink_fence") ||
			contains(material.Name, "wire_mesh") {
			matProps.IsTransparent = true
			matProps.Opacity = 0.7
		}

		// Special handling for Mirage materials
		if contains(material.Name, "mirage") &&
			(contains(material.Name, "window") ||
				contains(material.Name, "fence") ||
				contains(material.Name, "grate")) {
			matProps.IsTransparent = true
			matProps.Opacity = 0.7
		}

		// Store the material properties
		materials[i] = matProps
	}

	slog.Info("Materials processed",
		"totalMaterials", len(materials),
		"transparentMaterials", countTransparentMaterials(materials))

	return materials
}

// isTransparentMaterial checks if a material name suggests it should be treated as transparent
// This is a more comprehensive check to catch fence/grate/glass materials that might not be
// properly marked as transparent in the GLTF
func isTransparentMaterial(materialName string) bool {
	transparentKeywords := []string{
		"fence", "grate", "glass", "window", "transparent",
		"chain", "net", "screen", "mesh", "grid", "bars",
		"rail", "railing", "cage", "grille", "lattice",
		"mirror", "reflective", "shiny",
	}

	// Mirage-specific materials that are known to be transparent
	knownTransparentMaterials := map[string]bool{
		"residwall04a":    true, // Known semi-transparent wall
		"urban_fence_001": true, // Fence material
		"chainlink_fence": true, // Chain link fence
		"wire_mesh_fence": true, // Wire mesh fence
		"metal_railing":   true, // Metal railing
		"palace_window":   true, // Palace window
		"market_window":   true, // Market window
	}

	// Check if it's a known transparent material
	if knownTransparentMaterials[materialName] {
		return true
	}

	// Check for keywords in the material name
	lowercaseName := strings.ToLower(materialName)
	for _, keyword := range transparentKeywords {
		if strings.Contains(lowercaseName, keyword) {
			return true
		}
	}

	// Special check for Mirage-specific materials with semi-transparency
	// These might not have clear transparent keywords in their names
	if strings.Contains(lowercaseName, "mirage") &&
		(strings.Contains(lowercaseName, "window") ||
			strings.Contains(lowercaseName, "fence") ||
			strings.Contains(lowercaseName, "grate")) {
		return true
	}

	return false
}

// countTransparentMaterials counts how many materials are marked as transparent
func countTransparentMaterials(materials []MaterialProperties) int {
	count := 0
	for _, material := range materials {
		if material.IsTransparent {
			count++
		}
	}
	return count
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}

// extractTrianglesFromPrimitive extracts triangles from a GLTF primitive with material properties
func extractTrianglesFromPrimitive(doc *gltf.Document, primitive gltf.Primitive, materials []MaterialProperties, modelDir string) ([]types.Triangle, []MaterialProperties, error) {
	var triangles []types.Triangle
	var triangleMaterials []MaterialProperties

	// We need positions and indices to extract triangles
	if primitive.Indices == nil {
		return nil, nil, fmt.Errorf("primitive has no indices")
	}

	// Get position accessor
	posAccessorIdx, ok := primitive.Attributes["POSITION"]
	if !ok {
		return nil, nil, fmt.Errorf("primitive has no POSITION attribute")
	}
	posAccessor := doc.Accessors[posAccessorIdx]

	// Ensure the accessor is valid
	if posAccessor == nil {
		return nil, nil, fmt.Errorf("invalid position accessor")
	}

	// Get vertex positions by reading the buffer data directly
	positions, err := readPositions(doc, *posAccessor, modelDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read positions: %v", err)
	}

	// Get indices by reading the buffer data directly
	indices, err := readIndices(doc, *doc.Accessors[*primitive.Indices], modelDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read indices: %v", err)
	}

	// Determine the material for this primitive
	var material MaterialProperties
	if primitive.Material != nil && int(*primitive.Material) < len(materials) {
		material = materials[*primitive.Material]
	} else {
		// Default material
		material = MaterialProperties{
			IsTransparent:   false,
			Opacity:         1.0,
			RefractionIndex: 1.0,
			Name:            "default",
		}
	}

	slog.Debug("Extracting triangles from primitive", "material", material.Name, "transparent", material.IsTransparent)

	// Process triangles based on indices
	for i := 0; i < len(indices); i += 3 {
		if i+2 < len(indices) {
			// Get the three vertices of the triangle
			idx1, idx2, idx3 := indices[i], indices[i+1], indices[i+2]

			// Ensure indices are within bounds
			if int(idx1) >= len(positions) || int(idx2) >= len(positions) || int(idx3) >= len(positions) {
				slog.Warn("Triangle indices out of bounds, skipping",
					"idx1", idx1, "idx2", idx2, "idx3", idx3, "positions_len", len(positions))
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

			// Validate that vertices form a proper triangle
			if isValidTriangle(v1, v2, v3) {
				// Create and add triangle with material
				triangle := types.Triangle{V1: v1, V2: v2, V3: v3}
				triangles = append(triangles, triangle)
				triangleMaterials = append(triangleMaterials, material)
			} else {
				// TODO: Uncomment for debugging
				//slog.Debug("Skipping degenerate triangle",
				//"v1", v1, "v2", v2, "v3", v3)
			}
		}
	}

	return triangles, triangleMaterials, nil
}

// isValidTriangle checks if three vertices form a valid triangle (not degenerate)
func isValidTriangle(v1, v2, v3 r3.Vector) bool {
	const epsilon = 1e-6

	// Check that vertices are not coincident
	if vectorsEqual(v1, v2, epsilon) || vectorsEqual(v1, v3, epsilon) || vectorsEqual(v2, v3, epsilon) {
		return false
	}

	// Check that vertices are not collinear
	edge1 := v2.Sub(v1)
	edge2 := v3.Sub(v1)
	cross := edge1.Cross(edge2)

	// If cross product is near zero, vertices are collinear
	return cross.Norm() > epsilon
}

// vectorsEqual checks if two vectors are approximately equal
func vectorsEqual(a, b r3.Vector, epsilon float64) bool {
	return math.Abs(a.X-b.X) < epsilon &&
		math.Abs(a.Y-b.Y) < epsilon &&
		math.Abs(a.Z-b.Z) < epsilon
}

// readPositions reads vertex positions from GLTF buffers
func readPositions(doc *gltf.Document, accessor gltf.Accessor, modelDir string) ([][3]float64, error) {
	// Validate accessor
	if accessor.Type != gltf.AccessorVec3 {
		return nil, fmt.Errorf("position accessor must be VEC3, got %v", accessor.Type)
	}
	if accessor.ComponentType != gltf.ComponentFloat {
		return nil, fmt.Errorf("position components must be floats, got %v", accessor.ComponentType)
	}

	// Get buffer view
	if accessor.BufferView == nil {
		return nil, fmt.Errorf("accessor has no buffer view")
	}

	bufferView := doc.BufferViews[*accessor.BufferView]

	// Get buffer
	buffer := doc.Buffers[bufferView.Buffer]

	// Get buffer data
	var data []byte
	if buffer.URI != "" {
		// For external URI (assuming local file)
		if strings.HasPrefix(buffer.URI, "data:") {
			// Data URI - not handling in this simplified version
			return nil, fmt.Errorf("data URIs not supported")
		} else {
			// File URI
			var err error
			bufferPath := filepath.Join(modelDir, buffer.URI)
			data, err = os.ReadFile(bufferPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read buffer file %s: %v", bufferPath, err)
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
			return nil, fmt.Errorf("buffer overrun when reading positions at index %d (offset %d, stride %d, len(data) %d)",
				i, offset, stride, len(data))
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
	if accessor.BufferView == nil {
		return nil, fmt.Errorf("accessor has no buffer view")
	}

	bufferView := doc.BufferViews[*accessor.BufferView]

	// Get buffer
	buffer := doc.Buffers[bufferView.Buffer]

	// Get buffer data
	var data []byte
	if buffer.URI != "" {
		// For external URI (assuming local file)
		if strings.HasPrefix(buffer.URI, "data:") {
			// Data URI - not handling in this simplified version
			return nil, fmt.Errorf("data URIs not supported")
		} else {
			// File URI
			var err error
			bufferPath := filepath.Join(modelDir, buffer.URI)
			data, err = os.ReadFile(bufferPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read buffer file %s: %v", bufferPath, err)
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
			return nil, fmt.Errorf("buffer overrun when reading indices at index %d (offset %d, stride %d, len(data) %d)",
				i, offset, stride, len(data))
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

// Enhanced hitbox extraction for more accurate collision detection
func extractHitboxes(doc *gltf.Document) ([]Hitbox, error) {
	var hitboxes []Hitbox

	// Look for nodes with mesh references and extract hitboxes
	for i, node := range doc.Nodes {
		if node.Mesh == nil {
			continue
		}

		mesh := doc.Meshes[*node.Mesh]
		meshName := mesh.Name
		if meshName == "" {
			meshName = fmt.Sprintf("mesh_%d", i)
		}

		// Check if this mesh is marked as a hitbox in node extras or extensions
		isHitbox := isNodeHitbox(*node)

		// Process the node based on its type
		if isHitbox || contains(meshName, "hitbox") || contains(meshName, "collision") {
			// This is explicitly a hitbox node
			hitbox, err := createDetailedHitboxFromNode(doc, *node, i, *mesh)
			if err != nil {
				slog.Warn("Failed to create detailed hitbox", "node", node.Name, "error", err)
				continue
			}
			hitboxes = append(hitboxes, hitbox)
		} else {
			// For regular mesh nodes, create a simplified hitbox
			// Analyze geometry to determine best hitbox type
			hitbox, err := createHitboxFromNode(doc, *node, i)
			if err != nil {
				slog.Warn("Failed to create basic hitbox", "node", node.Name, "error", err)
				continue
			}
			hitboxes = append(hitboxes, hitbox)
		}
	}

	// If no hitboxes were found, try to create a convex hull for the entire model
	if len(hitboxes) == 0 {
		convexHitbox, err := createConvexHullHitbox(doc)
		if err == nil {
			hitboxes = append(hitboxes, convexHitbox)
		}
	}

	return hitboxes, nil
}

// isNodeHitbox checks if a node is explicitly marked as a hitbox
func isNodeHitbox(node gltf.Node) bool {
	// Check node name first
	if node.Name != "" {
		lowerName := strings.ToLower(node.Name)
		if strings.Contains(lowerName, "hitbox") ||
			strings.Contains(lowerName, "collision") ||
			strings.Contains(lowerName, "physics") {
			return true
		}
	}

	// Check extras (optional GLTF metadata)
	// This is a simplified check - in a full implementation, you would check for specific
	// custom properties in the extras field

	return false
}

// createDetailedHitboxFromNode creates a detailed hitbox from a GLTF node
// This creates hitboxes that better match the actual geometry
func createDetailedHitboxFromNode(doc *gltf.Document, node gltf.Node, nodeIndex int, mesh gltf.Mesh) (Hitbox, error) {
	// Determine hitbox type based on node/mesh properties
	hitboxType := determineHitboxType(node, mesh, *doc)

	// Initialize with extreme values
	minBounds := r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
	maxBounds := r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

	var vertices []r3.Vector

	// Process each primitive to collect all vertices
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

		// Get the actual vertices for more detailed hitbox creation
		if primitive.Indices != nil {
			positions, err := readPositionsForAccessor(doc, posAccessor)
			if err == nil {
				for _, pos := range positions {
					vertices = append(vertices, r3.Vector{
						X: pos[0],
						Y: pos[1],
						Z: pos[2],
					})
				}
			}
		}
	}

	// Create hitbox with appropriate name
	name := node.Name
	if name == "" {
		name = fmt.Sprintf("hitbox_%d", nodeIndex)
	}

	hitbox := Hitbox{
		Name:      name,
		BoneName:  name, // In a better implementation, map to actual bones
		MinBounds: minBounds,
		MaxBounds: maxBounds,
		Type:      hitboxType,
	}

	// Generate appropriate vertices based on hitbox type
	if len(vertices) > 0 {
		if hitboxType == "Box" {
			// For box, use the 8 corners
			hitbox.Vertices = generateBoxVertices(minBounds, maxBounds)
		} else if hitboxType == "Sphere" {
			// For sphere, use center and radius
			center := r3.Vector{
				X: (minBounds.X + maxBounds.X) / 2,
				Y: (minBounds.Y + maxBounds.Y) / 2,
				Z: (minBounds.Z + maxBounds.Z) / 2,
			}

			// Add center vertex
			hitbox.Vertices = append(hitbox.Vertices, center)

			// Add vertices around the sphere
			radius := calculateRadius(minBounds, maxBounds)
			hitbox.Vertices = append(hitbox.Vertices, generateSphereVertices(center, radius, 8)...)
		} else if hitboxType == "Capsule" {
			// For capsule, use axis endpoints and radius
			hitbox.Vertices = generateCapsuleVertices(minBounds, maxBounds)
		} else if hitboxType == "Convex" {
			// For convex hull, use the actual vertices
			hitbox.Vertices = vertices
		}
	} else {
		// Fallback to box if we couldn't get vertices
		hitbox.Vertices = generateBoxVertices(minBounds, maxBounds)
		hitbox.Type = "Box"
	}

	return hitbox, nil
}

// Helper functions for hitbox generation

// calculateRadius calculates the radius for a sphere or capsule
func calculateRadius(min, max r3.Vector) float64 {
	// For a sphere, use the largest dimension divided by 2
	sizeX := max.X - min.X
	sizeY := max.Y - min.Y
	sizeZ := max.Z - min.Z

	return math.Max(math.Max(sizeX, sizeY), sizeZ) / 2.0
}

// generateBoxVertices creates vertices for the 8 corners of a box
func generateBoxVertices(min, max r3.Vector) []r3.Vector {
	return []r3.Vector{
		{X: min.X, Y: min.Y, Z: min.Z},
		{X: min.X, Y: min.Y, Z: max.Z},
		{X: min.X, Y: max.Y, Z: min.Z},
		{X: min.X, Y: max.Y, Z: max.Z},
		{X: max.X, Y: min.Y, Z: min.Z},
		{X: max.X, Y: min.Y, Z: max.Z},
		{X: max.X, Y: max.Y, Z: min.Z},
		{X: max.X, Y: max.Y, Z: max.Z},
	}
}

// generateSphereVertices creates vertices distributed around a sphere
func generateSphereVertices(center r3.Vector, radius float64, numPoints int) []r3.Vector {
	var vertices []r3.Vector

	// Golden ratio method for distributing points evenly on a sphere
	phi := math.Pi * (3.0 - math.Sqrt(5.0)) // Golden angle in radians

	for i := 0; i < numPoints; i++ {
		y := 1.0 - (float64(i)/float64(numPoints-1))*2.0 // y goes from 1 to -1
		radiusAtY := math.Sqrt(1.0 - y*y)                // radius at y

		theta := phi * float64(i) // Golden angle increment

		x := math.Cos(theta) * radiusAtY
		z := math.Sin(theta) * radiusAtY

		vertices = append(vertices, r3.Vector{
			X: center.X + x*radius,
			Y: center.Y + y*radius,
			Z: center.Z + z*radius,
		})
	}

	return vertices
}

// generateCapsuleVertices creates vertices for a capsule
func generateCapsuleVertices(min, max r3.Vector) []r3.Vector {
	var vertices []r3.Vector

	// Find the longest dimension to determine the capsule axis
	sizeX := max.X - min.X
	sizeY := max.Y - min.Y
	sizeZ := max.Z - min.Z

	var axisDirection int // 0 = X, 1 = Y, 2 = Z
	var radius float64

	if sizeX > sizeY && sizeX > sizeZ {
		axisDirection = 0
		radius = math.Max(sizeY, sizeZ) / 2.0
	} else if sizeY > sizeX && sizeY > sizeZ {
		axisDirection = 1
		radius = math.Max(sizeX, sizeZ) / 2.0
	} else {
		axisDirection = 2
		radius = math.Max(sizeX, sizeY) / 2.0
	}

	// Calculate center
	center := r3.Vector{
		X: (min.X + max.X) / 2.0,
		Y: (min.Y + max.Y) / 2.0,
		Z: (min.Z + max.Z) / 2.0,
	}

	// Calculate half-length of the cylinder part
	var halfLength float64
	switch axisDirection {
	case 0: // X-axis
		halfLength = sizeX/2.0 - radius
	case 1: // Y-axis
		halfLength = sizeY/2.0 - radius
	case 2: // Z-axis
		halfLength = sizeZ/2.0 - radius
	}

	// Add end points of the capsule axis
	var end1, end2 r3.Vector
	switch axisDirection {
	case 0: // X-axis
		end1 = r3.Vector{X: center.X - halfLength, Y: center.Y, Z: center.Z}
		end2 = r3.Vector{X: center.X + halfLength, Y: center.Y, Z: center.Z}
	case 1: // Y-axis
		end1 = r3.Vector{X: center.X, Y: center.Y - halfLength, Z: center.Z}
		end2 = r3.Vector{X: center.X, Y: center.Y + halfLength, Z: center.Z}
	case 2: // Z-axis
		end1 = r3.Vector{X: center.X, Y: center.Y, Z: center.Z - halfLength}
		end2 = r3.Vector{X: center.X, Y: center.Y, Z: center.Z + halfLength}
	}

	vertices = append(vertices, end1, end2)

	// Add hemisphere points at each end
	vertices = append(vertices, generateHemisphereVertices(end1, radius, axisDirection, -1, 6)...)
	vertices = append(vertices, generateHemisphereVertices(end2, radius, axisDirection, 1, 6)...)

	return vertices
}

// generateHemisphereVertices creates vertices for a hemisphere
func generateHemisphereVertices(center r3.Vector, radius float64, axisDirection, sign, numPoints int) []r3.Vector {
	var vertices []r3.Vector

	for i := 0; i < numPoints; i++ {
		phi := 2.0 * math.Pi * float64(i) / float64(numPoints)

		// For the default case (no remainder points), create a ring of points
		x := radius * math.Cos(phi)
		y := radius * math.Sin(phi)

		// Adjust based on axis direction
		var point r3.Vector
		switch axisDirection {
		case 0: // X-axis
			point = r3.Vector{
				X: center.X,
				Y: center.Y + x,
				Z: center.Z + y,
			}
		case 1: // Y-axis
			point = r3.Vector{
				X: center.X + x,
				Y: center.Y,
				Z: center.Z + y,
			}
		case 2: // Z-axis
			point = r3.Vector{
				X: center.X + x,
				Y: center.Y + y,
				Z: center.Z,
			}
		}
		vertices = append(vertices, point)
	}

	// Add the hemisphere tip
	var tip r3.Vector
	switch axisDirection {
	case 0: // X-axis
		tip = r3.Vector{X: center.X + float64(sign)*radius, Y: center.Y, Z: center.Z}
	case 1: // Y-axis
		tip = r3.Vector{X: center.X, Y: center.Y + float64(sign)*radius, Z: center.Z}
	case 2: // Z-axis
		tip = r3.Vector{X: center.X, Y: center.Y, Z: center.Z + float64(sign)*radius}
	}
	vertices = append(vertices, tip)

	return vertices
}

// determineHitboxType analyzes a node/mesh to determine the appropriate hitbox type
func determineHitboxType(node gltf.Node, mesh gltf.Mesh, doc gltf.Document) string {
	// Check node name for hints
	lowerName := strings.ToLower(node.Name)
	if strings.Contains(lowerName, "sphere") {
		return "Sphere"
	}
	if strings.Contains(lowerName, "capsule") {
		return "Capsule"
	}
	if strings.Contains(lowerName, "box") {
		return "Box"
	}
	if strings.Contains(lowerName, "convex") {
		return "Convex"
	}

	// Check mesh name for hints
	lowerMeshName := strings.ToLower(mesh.Name)
	if strings.Contains(lowerMeshName, "sphere") {
		return "Sphere"
	}
	if strings.Contains(lowerMeshName, "capsule") {
		return "Capsule"
	}
	if strings.Contains(lowerMeshName, "box") {
		return "Box"
	}
	if strings.Contains(lowerMeshName, "convex") {
		return "Convex"
	}

	// Analyze the dimensions - if one dimension is significantly larger, use capsule
	size := calculateSize(node, mesh, doc)
	maxDim := math.Max(math.Max(size.X, size.Y), size.Z)
	minDim := math.Min(math.Min(size.X, size.Y), size.Z)

	if maxDim > 2*minDim {
		return "Capsule"
	}

	// Default to box for most objects
	return "Box"
}

// calculateSize computes the dimensions of a node/mesh
func calculateSize(node gltf.Node, mesh gltf.Mesh, doc gltf.Document) r3.Vector {
	// If node has scale, use that
	if node.Scale != [3]float64{0, 0, 0} && node.Scale != [3]float64{1, 1, 1} {
		return r3.Vector{
			X: node.Scale[0],
			Y: node.Scale[1],
			Z: node.Scale[2],
		}
	}

	// Otherwise calculate from mesh bounds
	var minBounds, maxBounds r3.Vector
	minBounds = r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
	maxBounds = r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

	for _, primitive := range mesh.Primitives {
		if posIdx, ok := primitive.Attributes["POSITION"]; ok {
			// Use accessor min/max if available
			accessor := doc.Accessors[posIdx]
			if accessor.Min != nil && accessor.Max != nil {
				min := r3.Vector{
					X: float64(accessor.Min[0]),
					Y: float64(accessor.Min[1]),
					Z: float64(accessor.Min[2]),
				}
				max := r3.Vector{
					X: float64(accessor.Max[0]),
					Y: float64(accessor.Max[1]),
					Z: float64(accessor.Max[2]),
				}

				minBounds.X = math.Min(minBounds.X, min.X)
				minBounds.Y = math.Min(minBounds.Y, min.Y)
				minBounds.Z = math.Min(minBounds.Z, min.Z)

				maxBounds.X = math.Max(maxBounds.X, max.X)
				maxBounds.Y = math.Max(maxBounds.Y, max.Y)
				maxBounds.Z = math.Max(maxBounds.Z, max.Z)
			}
		}
	}

	return r3.Vector{
		X: maxBounds.X - minBounds.X,
		Y: maxBounds.Y - minBounds.Y,
		Z: maxBounds.Z - minBounds.Z,
	}
}

// readPositionsForAccessor extracts position data for a specific accessor
func readPositionsForAccessor(doc *gltf.Document, accessor *gltf.Accessor) ([][3]float64, error) {
	if accessor == nil {
		return nil, fmt.Errorf("accessor is nil")
	}

	if accessor.BufferView == nil {
		return nil, fmt.Errorf("accessor has no buffer view")
	}

	bufferView := doc.BufferViews[*accessor.BufferView]
	buffer := doc.Buffers[bufferView.Buffer]

	var data []byte
	if len(buffer.Data) > 0 {
		data = buffer.Data
	} else if buffer.URI != "" {
		// For external URIs, you would need to load the file
		// This is simplified and assumes data is embedded
		return nil, fmt.Errorf("external buffer URI not supported in this function")
	} else {
		return nil, fmt.Errorf("buffer has no data")
	}

	offset := bufferView.ByteOffset + accessor.ByteOffset
	stride := bufferView.ByteStride
	if stride == 0 {
		stride = 3 * 4 // 3 components (vec3) * 4 bytes (float32)
	}

	positions := make([][3]float64, accessor.Count)
	for i := 0; i < int(accessor.Count); i++ {
		idx := uint32(offset) + uint32(i)*uint32(stride)
		if idx+11 >= uint32(len(data)) {
			return nil, fmt.Errorf("buffer overrun at position %d", i)
		}

		for j := 0; j < 3; j++ {
			bytes := data[idx+uint32(j*4) : idx+uint32(j*4+4)]
			bits := uint32(bytes[0]) | uint32(bytes[1])<<8 | uint32(bytes[2])<<16 | uint32(bytes[3])<<24
			float := math.Float32frombits(bits)
			positions[i][j] = float64(float)
		}
	}

	return positions, nil
}

// createConvexHullHitbox creates a convex hull hitbox for the entire model
func createConvexHullHitbox(doc *gltf.Document) (Hitbox, error) {
	var vertices []r3.Vector

	// Extract all vertices from the model
	for meshIdx, mesh := range doc.Meshes {
		for primIdx, primitive := range mesh.Primitives {
			posAccessorIdx, ok := primitive.Attributes["POSITION"]
			if !ok {
				continue
			}

			// Get position accessor
			posAccessor := doc.Accessors[posAccessorIdx]
			if posAccessor == nil || posAccessor.BufferView == nil {
				continue
			}

			// Read positions
			positions, err := readPositionsForAccessor(doc, posAccessor)
			if err != nil {
				slog.Warn("Failed to read positions for convex hull",
					"mesh", meshIdx, "primitive", primIdx, "error", err)
				continue
			}

			// Add vertices
			for _, pos := range positions {
				vertices = append(vertices, r3.Vector{
					X: pos[0],
					Y: pos[1],
					Z: pos[2],
				})
			}
		}
	}

	if len(vertices) < 4 {
		return Hitbox{}, fmt.Errorf("not enough vertices for convex hull (minimum 4 required)")
	}

	// Compute bounding box
	minBounds := r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
	maxBounds := r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

	for _, v := range vertices {
		minBounds.X = math.Min(minBounds.X, v.X)
		minBounds.Y = math.Min(minBounds.Y, v.Y)
		minBounds.Z = math.Min(minBounds.Z, v.Z)

		maxBounds.X = math.Max(maxBounds.X, v.X)
		maxBounds.Y = math.Max(maxBounds.Y, v.Y)
		maxBounds.Z = math.Max(maxBounds.Z, v.Z)
	}

	// For simplicity, we'll take a subset of vertices for the convex hull (first 1000 max)
	// In a production implementation, you'd apply a convex hull algorithm to all vertices
	maxVerticesToUse := 1000
	if len(vertices) > maxVerticesToUse {
		// Select a subset of vertices that are well-distributed
		subset := make([]r3.Vector, 0, maxVerticesToUse)
		step := len(vertices) / maxVerticesToUse

		for i := 0; i < len(vertices); i += step {
			if len(subset) < maxVerticesToUse {
				subset = append(subset, vertices[i])
			} else {
				break
			}
		}

		vertices = subset
	}

	return Hitbox{
		Name:      "model_convex_hull",
		BoneName:  "root", // Use root as the reference bone
		MinBounds: minBounds,
		MaxBounds: maxBounds,
		Vertices:  vertices,
		Type:      "Convex",
	}, nil
}

// createHitboxFromNode creates a basic hitbox from a GLTF node
func createHitboxFromNode(doc *gltf.Document, node gltf.Node, nodeIndex int) (Hitbox, error) {
	if node.Mesh == nil {
		return Hitbox{}, fmt.Errorf("node has no mesh")
	}

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
		if posAccessor == nil {
			continue
		}

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

	// If we couldn't get valid bounds, return an error
	if minBounds.X > maxBounds.X {
		return Hitbox{}, fmt.Errorf("could not determine bounds for node %s", node.Name)
	}

	// Create the hitbox - use a default box type
	name := node.Name
	if name == "" {
		name = fmt.Sprintf("hitbox_%d", nodeIndex)
	}

	// Apply node transformation (if any) to bounds
	if node.Matrix != [16]float64{} && node.Matrix != [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} {
		// Apply full matrix transformation to the bounds
		transformedMin, transformedMax := transformBoundsByMatrix(minBounds, maxBounds, node.Matrix)
		minBounds, maxBounds = transformedMin, transformedMax
	} else {
		// Apply translation, rotation, scale individually if specified
		if node.Translation != [3]float64{} {
			minBounds.X += node.Translation[0]
			minBounds.Y += node.Translation[1]
			minBounds.Z += node.Translation[2]
			maxBounds.X += node.Translation[0]
			maxBounds.Y += node.Translation[1]
			maxBounds.Z += node.Translation[2]
		}

		if node.Scale != [3]float64{} && node.Scale != [3]float64{1, 1, 1} {
			center := r3.Vector{
				X: (minBounds.X + maxBounds.X) / 2,
				Y: (minBounds.Y + maxBounds.Y) / 2,
				Z: (minBounds.Z + maxBounds.Z) / 2,
			}

			halfSize := r3.Vector{
				X: (maxBounds.X - minBounds.X) / 2,
				Y: (maxBounds.Y - minBounds.Y) / 2,
				Z: (maxBounds.Z - minBounds.Z) / 2,
			}

			scaledHalfSize := r3.Vector{
				X: halfSize.X * node.Scale[0],
				Y: halfSize.Y * node.Scale[1],
				Z: halfSize.Z * node.Scale[2],
			}

			minBounds = r3.Vector{
				X: center.X - scaledHalfSize.X,
				Y: center.Y - scaledHalfSize.Y,
				Z: center.Z - scaledHalfSize.Z,
			}

			maxBounds = r3.Vector{
				X: center.X + scaledHalfSize.X,
				Y: center.Y + scaledHalfSize.Y,
				Z: center.Z + scaledHalfSize.Z,
			}
		}

		// Rotation is more complex - we would need to transform all 8 corners of the box
		// For simplicity, we'll just expand the bounding box slightly to account for rotation
		if node.Rotation != [4]float64{0, 0, 0, 1} {
			center := r3.Vector{
				X: (minBounds.X + maxBounds.X) / 2,
				Y: (minBounds.Y + maxBounds.Y) / 2,
				Z: (minBounds.Z + maxBounds.Z) / 2,
			}

			// Calculate radius of the bounding sphere
			radius := math.Sqrt(
				math.Pow(maxBounds.X-center.X, 2) +
					math.Pow(maxBounds.Y-center.Y, 2) +
					math.Pow(maxBounds.Z-center.Z, 2),
			)

			// Expand bounds to ensure rotated box fits
			minBounds = r3.Vector{X: center.X - radius, Y: center.Y - radius, Z: center.Z - radius}
			maxBounds = r3.Vector{X: center.X + radius, Y: center.Y + radius, Z: center.Z + radius}
		}
	}

	hitbox := Hitbox{
		Name:      name,
		BoneName:  name, // Simplification - in practice you'd want actual bone mapping
		MinBounds: minBounds,
		MaxBounds: maxBounds,
		Type:      "Box", // Default to box type
	}

	// Generate vertices for the box
	hitbox.Vertices = generateBoxVertices(minBounds, maxBounds)

	return hitbox, nil
}

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

func buildSpatialStructures(model *Model) error {
	triangles := model.triangles

	// Skip if no triangles
	if len(triangles) == 0 {
		return fmt.Errorf("no triangles in model")
	}

	// Check for invalid model bounds and fix if needed
	checkAndFixInvalidBounds(model)

	// First, build the BVH tree for the model
	root, err := buildBVH(model.triangles, model.materials, 0, len(model.triangles)-1, 0)
	if err != nil {
		return fmt.Errorf("failed to build BVH: %v", err)
	}
	model.bvh = root

	// Then, build the spatial grid as a secondary structure
	return buildSpatialGrid(model)
}

// checkAndFixInvalidBounds verifies the model's bounds are valid and fixes them if not
func checkAndFixInvalidBounds(model *Model) {
	// Check for extremely negative values which indicate invalid initialization
	if model.min.X < -1000000000 || model.min.Y < -1000000000 || model.min.Z < -1000000000 ||
		model.max.X > 1000000000 || model.max.Y > 1000000000 || model.max.Z > 1000000000 {

		slog.Warn("Invalid model bounds detected! Rebuilding from triangles...",
			"oldMin", model.min,
			"oldMax", model.max)

		// Reset bounds to extreme opposite values to ensure they get properly updated
		model.min = r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
		model.max = r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

		// Rebuild bounds from all triangles
		for _, tri := range model.triangles {
			// Update min bounds
			model.min.X = math.Min(model.min.X, math.Min(math.Min(tri.V1.X, tri.V2.X), tri.V3.X))
			model.min.Y = math.Min(model.min.Y, math.Min(math.Min(tri.V1.Y, tri.V2.Y), tri.V3.Y))
			model.min.Z = math.Min(model.min.Z, math.Min(math.Min(tri.V1.Z, tri.V2.Z), tri.V3.Z))

			// Update max bounds
			model.max.X = math.Max(model.max.X, math.Max(math.Max(tri.V1.X, tri.V2.X), tri.V3.X))
			model.max.Y = math.Max(model.max.Y, math.Max(math.Max(tri.V1.Y, tri.V2.Y), tri.V3.Y))
			model.max.Z = math.Max(model.max.Z, math.Max(math.Max(tri.V1.Z, tri.V2.Z), tri.V3.Z))
		}

		// Add a small padding
		const padding = 1.0
		model.min.X -= padding
		model.min.Y -= padding
		model.min.Z -= padding
		model.max.X += padding
		model.max.Y += padding
		model.max.Z += padding

		slog.Info("Model bounds rebuilt from triangles",
			"newMin", model.min,
			"newMax", model.max,
			"width", model.max.X-model.min.X,
			"height", model.max.Y-model.min.Y,
			"depth", model.max.Z-model.min.Z)
	}
}

func buildSpatialGrid(model *Model) error {
	triangles := model.triangles

	// Recheck bounds before building grid
	checkAndFixInvalidBounds(model)

	// Use the model's grid size
	gridSize := model.gridSize

	// Dynamic adjustment of grid size based on model size
	modelSize := model.max.Sub(model.min).Norm()
	if modelSize > 10000 {
		gridSize = 128.0 // Very large model
	} else if modelSize > 5000 {
		gridSize = 64.0 // Large model
	} else if modelSize > 2000 {
		gridSize = 32.0 // Medium model
	}
	model.gridSize = gridSize

	// Pre-allocate some cells to avoid frequent map resizing
	model.sectors = make(map[GridKey][]int, len(triangles)/4) // Rough estimate of cell count

	// Add triangles to grid cells they intersect
	for i, tri := range triangles {
		// Get triangle bounds
		triMin := r3.Vector{
			X: math.Min(math.Min(tri.V1.X, tri.V2.X), tri.V3.X),
			Y: math.Min(math.Min(tri.V1.Y, tri.V2.Y), tri.V3.Y),
			Z: math.Min(math.Min(tri.V1.Z, tri.V2.Z), tri.V3.Z),
		}
		triMax := r3.Vector{
			X: math.Max(math.Max(tri.V1.X, tri.V2.X), tri.V3.X),
			Y: math.Max(math.Max(tri.V1.Y, tri.V2.Y), tri.V3.Y),
			Z: math.Max(math.Max(tri.V1.Z, tri.V2.Z), tri.V3.Z),
		}

		// Convert to cell coordinates with boundary checks
		// Use a safe floor function that handles extreme values
		minCellX := safeFloor(triMin.X / gridSize)
		minCellY := safeFloor(triMin.Y / gridSize)
		minCellZ := safeFloor(triMin.Z / gridSize)
		maxCellX := safeFloor(triMax.X / gridSize)
		maxCellY := safeFloor(triMax.Y / gridSize)
		maxCellZ := safeFloor(triMax.Z / gridSize)

		// Limit the cell range to avoid excessive memory usage
		const maxCellRange = 1000 // Maximum number of cells in any dimension
		if maxCellX-minCellX > maxCellRange ||
			maxCellY-minCellY > maxCellRange ||
			maxCellZ-minCellZ > maxCellRange {
			slog.Warn("Triangle spans too many cells, limiting range",
				"triangle", i,
				"original_range_x", maxCellX-minCellX,
				"original_range_y", maxCellY-minCellY,
				"original_range_z", maxCellZ-minCellZ)

			// Limit the range while keeping the cell coordinates centered
			if maxCellX-minCellX > maxCellRange {
				center := (minCellX + maxCellX) / 2
				minCellX = center - maxCellRange/2
				maxCellX = center + maxCellRange/2
			}

			if maxCellY-minCellY > maxCellRange {
				center := (minCellY + maxCellY) / 2
				minCellY = center - maxCellRange/2
				maxCellY = center + maxCellRange/2
			}

			if maxCellZ-minCellZ > maxCellRange {
				center := (minCellZ + maxCellZ) / 2
				minCellZ = center - maxCellRange/2
				maxCellZ = center + maxCellRange/2
			}
		}

		// Improve grid accuracy by checking if the triangle actually intersects each cell
		for x := minCellX; x <= maxCellX; x++ {
			for y := minCellY; y <= maxCellY; y++ {
				for z := minCellZ; z <= maxCellZ; z++ {
					cellMin := r3.Vector{
						X: float64(x) * gridSize,
						Y: float64(y) * gridSize,
						Z: float64(z) * gridSize,
					}
					cellMax := r3.Vector{
						X: float64(x+1) * gridSize,
						Y: float64(y+1) * gridSize,
						Z: float64(z+1) * gridSize,
					}

					// Enhanced check: see if triangle actually intersects this cell
					if triangleIntersectsAABB(tri, cellMin, cellMax) {
						key := GridKey{x: x, y: y, z: z}
						model.sectors[key] = append(model.sectors[key], i) // Store triangle index, not the triangle itself
					}
				}
			}
		}
	}

	// Log statistics about the grid
	cellCount := len(model.sectors)
	totalRefs := 0
	for _, indices := range model.sectors {
		totalRefs += len(indices)
	}

	avgRefsPerCell := 0.0
	if cellCount > 0 {
		avgRefsPerCell = float64(totalRefs) / float64(cellCount)
	}

	slog.Info("Spatial grid built",
		"triangles", len(triangles),
		"cells", cellCount,
		"total_refs", totalRefs,
		"avg_refs_per_cell", avgRefsPerCell,
		"grid_size", gridSize)

	return nil
}

// safeFloor performs a floor operation that is safe for extreme values
func safeFloor(v float64) int {
	// Check for NaN
	if math.IsNaN(v) {
		return 0
	}

	// Check for extremely large positive values
	if v > float64(math.MaxInt32) {
		return math.MaxInt32
	}

	// Check for extremely large negative values
	if v < float64(math.MinInt32) {
		return math.MinInt32
	}

	return int(math.Floor(v))
}

func optimizeMapSpatialStructure(mapModel *MapModel) {
	// Count triangles per grid cell
	trianglesPerCell := make(map[GridKey]int)
	cellsWithTriangles := 0
	emptyGridCells := 0

	for key, indices := range mapModel.BaseModel.sectors {
		count := len(indices)
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

		slog.Info("Found bombsite", "name", node.Name, "position", transformedPos)
	}
}

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

		slog.Info("Found spawn point", "name", node.Name, "team", team, "position", transformedPos)
	}
}

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
		posAccessor := doc.Accessors[posIdx]
		positions, err := readPositionsForAccessor(doc, posAccessor)
		if err != nil {
			slog.Warn("Failed to read navmesh positions", "error", err)
			continue
		}

		slog.Info("Processed nav mesh data", "vertices", len(positions))

		// Store nav mesh in the model
		mapModel.navMeshVertices = positions
	}
}

func processNamedAreaNode(node gltf.Node, model *MapModel) {
	// Extract position if available
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

func determineMapScale(mapModel *MapModel) {
	// Check for invalid bounds first
	checkAndFixInvalidBounds(&mapModel.BaseModel)

	// Get the overall size of the map
	size := r3.Vector{
		X: mapModel.BaseModel.max.X - mapModel.BaseModel.min.X,
		Y: mapModel.BaseModel.max.Y - mapModel.BaseModel.min.Y,
		Z: mapModel.BaseModel.max.Z - mapModel.BaseModel.min.Z,
	}

	// Validate the size - if any dimension is negative or unreasonably large, reset
	if size.X <= 0 || size.Y <= 0 || size.Z <= 0 ||
		size.X > 100000 || size.Y > 100000 || size.Z > 100000 {
		slog.Warn("Invalid map size detected, using default scale",
			"invalidSize", size)

		// Set a reasonable default grid size
		mapModel.BaseModel.gridSize = 64.0
		return
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

func transformBoundsByMatrix(min, max r3.Vector, matrix [16]float64) (r3.Vector, r3.Vector) {
	// Create all 8 corners of the box
	corners := []r3.Vector{
		{X: min.X, Y: min.Y, Z: min.Z},
		{X: min.X, Y: min.Y, Z: max.Z},
		{X: min.X, Y: max.Y, Z: min.Z},
		{X: min.X, Y: max.Y, Z: max.Z},
		{X: max.X, Y: min.Y, Z: min.Z},
		{X: max.X, Y: min.Y, Z: max.Z},
		{X: max.X, Y: max.Y, Z: min.Z},
		{X: max.X, Y: max.Y, Z: max.Z},
	}

	// Transform each corner
	transformedMin := r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
	transformedMax := r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

	for _, corner := range corners {
		// Apply the transformation matrix
		transformedCorner := transformPointByMatrix(corner, matrix)

		// Update the transformed bounds
		transformedMin.X = math.Min(transformedMin.X, transformedCorner.X)
		transformedMin.Y = math.Min(transformedMin.Y, transformedCorner.Y)
		transformedMin.Z = math.Min(transformedMin.Z, transformedCorner.Z)

		transformedMax.X = math.Max(transformedMax.X, transformedCorner.X)
		transformedMax.Y = math.Max(transformedMax.Y, transformedCorner.Y)
		transformedMax.Z = math.Max(transformedMax.Z, transformedCorner.Z)
	}

	return transformedMin, transformedMax
}

func transformPointByMatrix(point r3.Vector, matrix [16]float64) r3.Vector {
	// Matrix layout (column-major):
	// [ 0  4  8 12 ]
	// [ 1  5  9 13 ]
	// [ 2  6 10 14 ]
	// [ 3  7 11 15 ]

	x := point.X*matrix[0] + point.Y*matrix[4] + point.Z*matrix[8] + matrix[12]
	y := point.X*matrix[1] + point.Y*matrix[5] + point.Z*matrix[9] + matrix[13]
	z := point.X*matrix[2] + point.Y*matrix[6] + point.Z*matrix[10] + matrix[14]
	w := point.X*matrix[3] + point.Y*matrix[7] + point.Z*matrix[11] + matrix[15]

	// Handle perspective divide if needed
	if math.Abs(w) > 1e-10 {
		return r3.Vector{X: x / w, Y: y / w, Z: z / w}
	}

	return r3.Vector{X: x, Y: y, Z: z}
}

func buildBVH(triangles []types.Triangle, materials []MaterialProperties, start, end, depth int) (*BVHNode, error) {
	if start > end {
		return nil, fmt.Errorf("invalid range: start > end")
	}

	// Create a node for this range of triangles
	node := &BVHNode{}

	// Calculate bounding box for all triangles in this range
	bbox := calculateBoundingBox(triangles, start, end)
	node.bbox = bbox

	numTriangles := end - start + 1

	// Base case: if few enough triangles or max depth reached, make a leaf node
	maxTrianglesPerLeaf := 8
	maxDepth := 20
	if numTriangles <= maxTrianglesPerLeaf || depth >= maxDepth {
		// Create leaf node with triangles
		node.triangles = make([]types.Triangle, numTriangles)
		node.materials = make([]MaterialProperties, numTriangles)
		for i := 0; i < numTriangles; i++ {
			node.triangles[i] = triangles[start+i]
			node.materials[i] = materials[start+i]
		}
		return node, nil
	}

	// Find the dominant axis of the bounding box
	size := r3.Vector{
		X: bbox.Max.X - bbox.Min.X,
		Y: bbox.Max.Y - bbox.Min.Y,
		Z: bbox.Max.Z - bbox.Min.Z,
	}

	axis := 0 // X-axis is default
	if size.Y > size.X && size.Y > size.Z {
		axis = 1 // Y-axis is dominant
	} else if size.Z > size.X && size.Z > size.Y {
		axis = 2 // Z-axis is dominant
	}

	// Sort triangles based on centroid along the chosen axis
	// For simplicity, we'll use a temporary slice for sorting
	type IndexedTriangle struct {
		index    int
		centroid float64 // centroid along chosen axis
	}

	sortData := make([]IndexedTriangle, numTriangles)
	for i := 0; i < numTriangles; i++ {
		tri := triangles[start+i]
		centroid := 0.0

		switch axis {
		case 0: // X-axis
			centroid = (tri.V1.X + tri.V2.X + tri.V3.X) / 3.0
		case 1: // Y-axis
			centroid = (tri.V1.Y + tri.V2.Y + tri.V3.Y) / 3.0
		case 2: // Z-axis
			centroid = (tri.V1.Z + tri.V2.Z + tri.V3.Z) / 3.0
		}

		sortData[i] = IndexedTriangle{index: start + i, centroid: centroid}
	}

	// Sort based on centroid
	sort.Slice(sortData, func(i, j int) bool {
		return sortData[i].centroid < sortData[j].centroid
	})

	// Rearrange triangles and materials based on sorted order
	tempTriangles := make([]types.Triangle, numTriangles)
	tempMaterials := make([]MaterialProperties, numTriangles)
	for i := 0; i < numTriangles; i++ {
		origIndex := sortData[i].index
		tempTriangles[i] = triangles[origIndex]
		tempMaterials[i] = materials[origIndex]
	}

	// Copy back the sorted triangles and materials
	for i := 0; i < numTriangles; i++ {
		triangles[start+i] = tempTriangles[i]
		materials[start+i] = tempMaterials[i]
	}

	// Split at the median
	mid := start + numTriangles/2

	// Recursively build left and right subtrees
	leftNode, err := buildBVH(triangles, materials, start, mid-1, depth+1)
	if err != nil {
		return nil, err
	}

	rightNode, err := buildBVH(triangles, materials, mid, end, depth+1)
	if err != nil {
		return nil, err
	}

	node.left = leftNode
	node.right = rightNode

	return node, nil
}

func calculateBoundingBox(triangles []types.Triangle, start, end int) AABB {
	// Initialize with extreme values
	min := r3.Vector{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}
	max := r3.Vector{X: -math.MaxFloat64, Y: -math.MaxFloat64, Z: -math.MaxFloat64}

	// Find min/max coordinates across all vertices in range
	for i := start; i <= end; i++ {
		tri := triangles[i]

		// Update for each vertex
		for _, v := range []r3.Vector{tri.V1, tri.V2, tri.V3} {
			min.X = math.Min(min.X, v.X)
			min.Y = math.Min(min.Y, v.Y)
			min.Z = math.Min(min.Z, v.Z)

			max.X = math.Max(max.X, v.X)
			max.Y = math.Max(max.Y, v.Y)
			max.Z = math.Max(max.Z, v.Z)
		}
	}

	// Add a small padding to avoid numerical precision issues
	const padding = 0.0001
	min.X -= padding
	min.Y -= padding
	min.Z -= padding
	max.X += padding
	max.Y += padding
	max.Z += padding

	return AABB{Min: min, Max: max}
}

func triangleIntersectsAABB(tri types.Triangle, min, max r3.Vector) bool {
	// First, simple bounds check
	triMin := r3.Vector{
		X: math.Min(math.Min(tri.V1.X, tri.V2.X), tri.V3.X),
		Y: math.Min(math.Min(tri.V1.Y, tri.V2.Y), tri.V3.Y),
		Z: math.Min(math.Min(tri.V1.Z, tri.V2.Z), tri.V3.Z),
	}
	triMax := r3.Vector{
		X: math.Max(math.Max(tri.V1.X, tri.V2.X), tri.V3.X),
		Y: math.Max(math.Max(tri.V1.Y, tri.V2.Y), tri.V3.Y),
		Z: math.Max(math.Max(tri.V1.Z, tri.V2.Z), tri.V3.Z),
	}

	// Quick early rejection test
	if triMax.X < min.X || triMin.X > max.X ||
		triMax.Y < min.Y || triMin.Y > max.Y ||
		triMax.Z < min.Z || triMin.Z > max.Z {
		return false
	}

	// For thin triangles that might pass through a cell without having vertices in it,
	// we need more detailed intersection tests. This is a simplified version.

	// Check if any vertex is inside the AABB
	if pointInAABB(tri.V1, min, max) ||
		pointInAABB(tri.V2, min, max) ||
		pointInAABB(tri.V3, min, max) {
		return true
	}

	// Check if any edge intersects the AABB
	edges := []struct{ a, b r3.Vector }{
		{tri.V1, tri.V2},
		{tri.V2, tri.V3},
		{tri.V3, tri.V1},
	}

	for _, edge := range edges {
		if lineIntersectsAABB(edge.a, edge.b, min, max) {
			return true
		}
	}

	// Check if the triangle intersects any of the 12 edges of the AABB
	aabbEdges := []struct{ a, b r3.Vector }{
		// Bottom face edges
		{r3.Vector{X: min.X, Y: min.Y, Z: min.Z}, r3.Vector{X: max.X, Y: min.Y, Z: min.Z}},
		{r3.Vector{X: min.X, Y: min.Y, Z: min.Z}, r3.Vector{X: min.X, Y: max.Y, Z: min.Z}},
		{r3.Vector{X: max.X, Y: min.Y, Z: min.Z}, r3.Vector{X: max.X, Y: max.Y, Z: min.Z}},
		{r3.Vector{X: min.X, Y: max.Y, Z: min.Z}, r3.Vector{X: max.X, Y: max.Y, Z: min.Z}},

		// Top face edges
		{r3.Vector{X: min.X, Y: min.Y, Z: max.Z}, r3.Vector{X: max.X, Y: min.Y, Z: max.Z}},
		{r3.Vector{X: min.X, Y: min.Y, Z: max.Z}, r3.Vector{X: min.X, Y: max.Y, Z: max.Z}},
		{r3.Vector{X: max.X, Y: min.Y, Z: max.Z}, r3.Vector{X: max.X, Y: max.Y, Z: max.Z}},
		{r3.Vector{X: min.X, Y: max.Y, Z: max.Z}, r3.Vector{X: max.X, Y: max.Y, Z: max.Z}},

		// Connecting edges
		{r3.Vector{X: min.X, Y: min.Y, Z: min.Z}, r3.Vector{X: min.X, Y: min.Y, Z: max.Z}},
		{r3.Vector{X: max.X, Y: min.Y, Z: min.Z}, r3.Vector{X: max.X, Y: min.Y, Z: max.Z}},
		{r3.Vector{X: min.X, Y: max.Y, Z: min.Z}, r3.Vector{X: min.X, Y: max.Y, Z: max.Z}},
		{r3.Vector{X: max.X, Y: max.Y, Z: min.Z}, r3.Vector{X: max.X, Y: max.Y, Z: max.Z}},
	}

	for _, edge := range aabbEdges {
		if triangleIntersectsLine(tri, edge.a, edge.b) {
			return true
		}
	}

	// If we reach here, no intersection was found
	return false
}

func pointInAABB(p, min, max r3.Vector) bool {
	return p.X >= min.X && p.X <= max.X &&
		p.Y >= min.Y && p.Y <= max.Y &&
		p.Z >= min.Z && p.Z <= max.Z
}

func lineIntersectsAABB(a, b, min, max r3.Vector) bool {
	// Check if either endpoint is inside the AABB
	if pointInAABB(a, min, max) || pointInAABB(b, min, max) {
		return true
	}

	// Check intersection with each face of the AABB
	dir := b.Sub(a)

	// For each axis, compute intersection with the two planes defining that face
	tMin := -math.MaxFloat64
	tMax := math.MaxFloat64

	// X-axis
	if math.Abs(dir.X) < 1e-10 {
		// Line is parallel to the X planes
		if a.X < min.X || a.X > max.X {
			return false
		}
	} else {
		invD := 1.0 / dir.X
		t1 := (min.X - a.X) * invD
		t2 := (max.X - a.X) * invD

		if t1 > t2 {
			t1, t2 = t2, t1
		}

		tMin = math.Max(tMin, t1)
		tMax = math.Min(tMax, t2)

		if tMin > tMax {
			return false
		}
	}

	// Y-axis
	if math.Abs(dir.Y) < 1e-10 {
		// Line is parallel to the Y planes
		if a.Y < min.Y || a.Y > max.Y {
			return false
		}
	} else {
		invD := 1.0 / dir.Y
		t1 := (min.Y - a.Y) * invD
		t2 := (max.Y - a.Y) * invD

		if t1 > t2 {
			t1, t2 = t2, t1
		}

		tMin = math.Max(tMin, t1)
		tMax = math.Min(tMax, t2)

		if tMin > tMax {
			return false
		}
	}

	// Z-axis
	if math.Abs(dir.Z) < 1e-10 {
		// Line is parallel to the Z planes
		if a.Z < min.Z || a.Z > max.Z {
			return false
		}
	} else {
		invD := 1.0 / dir.Z
		t1 := (min.Z - a.Z) * invD
		t2 := (max.Z - a.Z) * invD

		if t1 > t2 {
			t1, t2 = t2, t1
		}

		tMin = math.Max(tMin, t1)
		tMax = math.Min(tMax, t2)

		if tMin > tMax {
			return false
		}
	}

	// If tMin and tMax are still valid, the line intersects the AABB
	return tMin <= 1.0 && tMax >= 0.0
}

func triangleIntersectsLine(tri types.Triangle, a, b r3.Vector) bool {
	// Möller-Trumbore algorithm for ray-triangle intersection
	dir := b.Sub(a)

	edge1 := tri.V2.Sub(tri.V1)
	edge2 := tri.V3.Sub(tri.V1)

	h := dir.Cross(edge2)
	det := edge1.Dot(h)

	if math.Abs(det) < 1e-10 {
		return false // Line is parallel to the triangle
	}

	invDet := 1.0 / det
	s := a.Sub(tri.V1)
	u := s.Dot(h) * invDet

	if u < 0.0 || u > 1.0 {
		return false
	}

	q := s.Cross(edge1)
	v := dir.Dot(q) * invDet

	if v < 0.0 || u+v > 1.0 {
		return false
	}

	t := edge2.Dot(q) * invDet

	return t >= 0.0 && t <= 1.0
}
