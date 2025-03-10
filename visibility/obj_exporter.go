package visibility

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// ExportMapModelToOBJ exports a MapModel to an OBJ file that can be loaded in Blender
func ExportMapModelToOBJ(mapModel *MapModel, outputPath string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Write OBJ header with information about the source
	header := fmt.Sprintf("# CS2 Map Model exported to OBJ\n"+
		"# Vertices: %d\n"+
		"# Triangles: %d\n"+
		"# Bounds: Min(%.2f, %.2f, %.2f) Max(%.2f, %.2f, %.2f)\n\n",
		len(mapModel.BaseModel.triangles)*3, // 3 vertices per triangle
		len(mapModel.BaseModel.triangles),
		mapModel.BaseModel.min.X, mapModel.BaseModel.min.Y, mapModel.BaseModel.min.Z,
		mapModel.BaseModel.max.X, mapModel.BaseModel.max.Y, mapModel.BaseModel.max.Z)

	file.WriteString(header)

	// Create a material library file (MTL)
	mtlPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".mtl"
	mtlFile, err := os.Create(mtlPath)
	if err != nil {
		return fmt.Errorf("failed to create MTL file: %v", err)
	}
	defer mtlFile.Close()

	// Reference the MTL file in the OBJ
	mtlFilename := filepath.Base(mtlPath)
	file.WriteString(fmt.Sprintf("mtllib %s\n\n", mtlFilename))

	// Map to store unique materials and their indices
	materialMap := make(map[string]int)

	// Write unique materials to MTL file
	for i, mat := range mapModel.BaseModel.materials {
		matName := sanitizeMaterialName(mat.Name)
		if matName == "" {
			matName = fmt.Sprintf("material_%d", i)
		}

		// Only add material if we haven't seen it before
		if _, exists := materialMap[matName]; !exists {
			materialMap[matName] = len(materialMap)

			// Write material to MTL file
			mtlFile.WriteString(fmt.Sprintf("newmtl %s\n", matName))

			// Define material properties based on transparency
			if mat.IsTransparent {
				// Semi-transparent material
				mtlFile.WriteString("Kd 0.7 0.7 0.9\n")                 // Bluish color for transparent materials
				mtlFile.WriteString(fmt.Sprintf("d %f\n", mat.Opacity)) // Transparency
			} else {
				// Opaque material
				mtlFile.WriteString("Kd 0.8 0.8 0.8\n") // Light gray for opaque materials
				mtlFile.WriteString("d 1.0\n")          // Fully opaque
			}

			mtlFile.WriteString("Ka 0.1 0.1 0.1\n") // Ambient color
			mtlFile.WriteString("Ks 0.5 0.5 0.5\n") // Specular color
			mtlFile.WriteString("Ns 10.0\n")        // Specular exponent
			mtlFile.WriteString("\n")
		}
	}

	// Keep track of vertex indices (OBJ indices start at 1)
	vertexIndex := 1

	// Group triangles by material for better organization
	materialGroups := make(map[string][]types.Triangle)
	materialIndices := make(map[string][]int)

	// Organize triangles by material
	for i, tri := range mapModel.BaseModel.triangles {
		matName := "default"
		if i < len(mapModel.BaseModel.materials) {
			matName = sanitizeMaterialName(mapModel.BaseModel.materials[i].Name)
			if matName == "" {
				matName = fmt.Sprintf("material_%d", i)
			}
		}

		materialGroups[matName] = append(materialGroups[matName], tri)
		materialIndices[matName] = append(materialIndices[matName], vertexIndex)
		vertexIndex += 3 // Each triangle adds 3 vertices
	}

	// Write all vertices first (OBJ format requirement)
	for _, triangles := range materialGroups {
		for _, tri := range triangles {
			// Write the three vertices of this triangle
			// Note: In OBJ format, vertices are v x y z
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V1.X, tri.V1.Y, tri.V1.Z))
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V2.X, tri.V2.Y, tri.V2.Z))
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V3.X, tri.V3.Y, tri.V3.Z))
		}
	}

	// Add a blank line between vertices and faces
	file.WriteString("\n")

	// Write faces grouped by material
	for matName, triangles := range materialGroups {
		// Start a new object/group for this material
		file.WriteString(fmt.Sprintf("g %s\n", matName))
		file.WriteString(fmt.Sprintf("usemtl %s\n", matName))

		// Get the starting indices for this material
		indices := materialIndices[matName]

		// Write faces for this material
		for i := range triangles {
			baseIdx := indices[i]
			// In OBJ format, faces are f v1 v2 v3 (indices, not coordinates)
			file.WriteString(fmt.Sprintf("f %d %d %d\n",
				baseIdx, baseIdx+1, baseIdx+2))
		}

		file.WriteString("\n")
	}

	return nil
}

// ExportModelToOBJ exports a player Model to an OBJ file that can be loaded in Blender
func ExportModelToOBJ(model *Model, outputPath string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Write OBJ header with information about the source
	header := fmt.Sprintf("# CS2 Player Model exported to OBJ\n"+
		"# Vertices: %d\n"+
		"# Triangles: %d\n"+
		"# Hitboxes: %d\n"+
		"# Bounds: Min(%.2f, %.2f, %.2f) Max(%.2f, %.2f, %.2f)\n\n",
		len(model.triangles)*3, // 3 vertices per triangle
		len(model.triangles),
		len(model.hitboxes),
		model.min.X, model.min.Y, model.min.Z,
		model.max.X, model.max.Y, model.max.Z)

	file.WriteString(header)

	// Create a material library file (MTL)
	mtlPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".mtl"
	mtlFile, err := os.Create(mtlPath)
	if err != nil {
		return fmt.Errorf("failed to create MTL file: %v", err)
	}
	defer mtlFile.Close()

	// Reference the MTL file in the OBJ
	mtlFilename := filepath.Base(mtlPath)
	file.WriteString(fmt.Sprintf("mtllib %s\n\n", mtlFilename))

	// Write basic materials to MTL file
	mtlFile.WriteString("# Material definitions\n")
	mtlFile.WriteString("newmtl model_material\n")
	mtlFile.WriteString("Kd 0.8 0.8 0.8\n") // Diffuse color
	mtlFile.WriteString("Ka 0.2 0.2 0.2\n") // Ambient color
	mtlFile.WriteString("Ks 0.5 0.5 0.5\n") // Specular color
	mtlFile.WriteString("Ns 10.0\n")        // Specular exponent
	mtlFile.WriteString("d 1.0\n\n")        // Opacity

	// Special material for hitboxes
	mtlFile.WriteString("newmtl hitbox_material\n")
	mtlFile.WriteString("Kd 1.0 0.0 0.0\n") // Red for hitboxes
	mtlFile.WriteString("Ka 0.2 0.0 0.0\n") // Ambient color
	mtlFile.WriteString("Ks 0.5 0.0 0.0\n") // Specular color
	mtlFile.WriteString("Ns 10.0\n")        // Specular exponent
	mtlFile.WriteString("d 0.3\n\n")        // Mostly transparent

	// Start with the model triangles
	file.WriteString("g model_mesh\n")
	file.WriteString("usemtl model_material\n\n")

	// Write all model vertices
	vertexIndex := 1
	for _, tri := range model.triangles {
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V1.X, tri.V1.Y, tri.V1.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V2.X, tri.V2.Y, tri.V2.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V3.X, tri.V3.Y, tri.V3.Z))
	}

	file.WriteString("\n")

	// Write model faces
	for i := range model.triangles {
		baseIdx := vertexIndex + (i * 3)
		file.WriteString(fmt.Sprintf("f %d %d %d\n",
			baseIdx, baseIdx+1, baseIdx+2))
	}

	vertexIndex += len(model.triangles) * 3

	// If there are hitboxes, add them as wireframes
	if len(model.hitboxes) > 0 {
		file.WriteString("\n# Hitboxes\n")
		file.WriteString("g hitboxes\n")
		file.WriteString("usemtl hitbox_material\n\n")

		// Store all hitbox vertices and their corresponding indices
		hitboxVertexMap := make(map[r3.Vector]int)

		// Helper function to get or add a vertex
		getVertexIndex := func(v r3.Vector) int {
			if idx, exists := hitboxVertexMap[v]; exists {
				return idx
			}
			idx := vertexIndex
			hitboxVertexMap[v] = idx
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v.X, v.Y, v.Z))
			vertexIndex++
			return idx
		}

		// Process each hitbox
		for _, hitbox := range model.hitboxes {
			file.WriteString(fmt.Sprintf("# Hitbox: %s (Type: %s)\n",
				hitbox.Name, hitbox.Type))

			if hitbox.Type == "Box" {
				// For box hitboxes, create the 8 corners
				corners := []r3.Vector{
					{X: hitbox.MinBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MinBounds.Z},
					{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MinBounds.Z},
					{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MinBounds.Z},
					{X: hitbox.MinBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MinBounds.Z},
					{X: hitbox.MinBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MaxBounds.Z},
					{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MaxBounds.Z},
					{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MaxBounds.Z},
					{X: hitbox.MinBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MaxBounds.Z},
				}

				// Get indices for all corners
				cornerIndices := make([]int, 8)
				for i, corner := range corners {
					cornerIndices[i] = getVertexIndex(corner)
				}

				// Create the 12 edges of the box as line segments
				// Bottom face
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[0], cornerIndices[1]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[1], cornerIndices[2]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[2], cornerIndices[3]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[3], cornerIndices[0]))

				// Top face
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[4], cornerIndices[5]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[5], cornerIndices[6]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[6], cornerIndices[7]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[7], cornerIndices[4]))

				// Connecting edges
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[0], cornerIndices[4]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[1], cornerIndices[5]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[2], cornerIndices[6]))
				file.WriteString(fmt.Sprintf("l %d %d\n", cornerIndices[3], cornerIndices[7]))

			} else if hitbox.Type == "Sphere" || hitbox.Type == "Capsule" {
				// For sphere and capsule, add the vertices as points and connect them
				if len(hitbox.Vertices) > 0 {
					indices := make([]int, len(hitbox.Vertices))
					for i, vertex := range hitbox.Vertices {
						indices[i] = getVertexIndex(vertex)
					}

					// For visualization, connect each vertex to the next one
					for i := 0; i < len(indices)-1; i++ {
						file.WriteString(fmt.Sprintf("l %d %d\n", indices[i], indices[i+1]))
					}

					// For spheres, connect to the center point
					if hitbox.Type == "Sphere" && len(indices) > 1 {
						center := getVertexIndex(r3.Vector{
							X: (hitbox.MinBounds.X + hitbox.MaxBounds.X) / 2,
							Y: (hitbox.MinBounds.Y + hitbox.MaxBounds.Y) / 2,
							Z: (hitbox.MinBounds.Z + hitbox.MaxBounds.Z) / 2,
						})

						// Connect center to each vertex
						for _, idx := range indices {
							file.WriteString(fmt.Sprintf("l %d %d\n", center, idx))
						}
					}
				}
			} else if hitbox.Type == "Convex" {
				// For convex hull hitboxes, connect vertices to form a wireframe
				if len(hitbox.Vertices) > 0 {
					indices := make([]int, len(hitbox.Vertices))
					for i, vertex := range hitbox.Vertices {
						indices[i] = getVertexIndex(vertex)
					}

					// Connect each vertex to each other vertex (simplified visualization)
					// This is a basic representation; a proper convex hull would need more work
					for i := 0; i < len(indices); i++ {
						for j := i + 1; j < len(indices); j++ {
							file.WriteString(fmt.Sprintf("l %d %d\n", indices[i], indices[j]))
						}
					}
				}
			}
		}
	}

	return nil
}

// ExportCombinedModelToOBJ2 exports a MapModel and positioned Models to a single OBJ file
// with the player models properly aligned in model space
func ExportCombinedModelToOBJ2(mapModel *MapModel, models []*Model, positions []r3.Vector, outputPath string) error {
	// Validate input
	if len(models) != len(positions) {
		return fmt.Errorf("number of models (%d) must match number of positions (%d)", len(models), len(positions))
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Create a material library file (MTL)
	mtlPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".mtl"
	mtlFile, err := os.Create(mtlPath)
	if err != nil {
		return fmt.Errorf("failed to create MTL file: %v", err)
	}
	defer mtlFile.Close()

	// Write OBJ header with information
	header := fmt.Sprintf("# Combined CS2 Map and Player Models exported to OBJ\n"+
		"# Map Triangles: %d\n"+
		"# Player Models: %d\n"+
		"# Map Bounds: Min(%.2f, %.2f, %.2f) Max(%.2f, %.2f, %.2f)\n\n",
		len(mapModel.BaseModel.triangles),
		len(models),
		mapModel.BaseModel.min.X, mapModel.BaseModel.min.Y, mapModel.BaseModel.min.Z,
		mapModel.BaseModel.max.X, mapModel.BaseModel.max.Y, mapModel.BaseModel.max.Z)

	file.WriteString(header)

	// Reference the MTL file in the OBJ
	mtlFilename := filepath.Base(mtlPath)
	file.WriteString(fmt.Sprintf("mtllib %s\n\n", mtlFilename))

	// Start writing to MTL file - define map materials
	mtlFile.WriteString("# Map material definitions\n")

	// Create a map of material names to avoid duplicates
	materialMap := make(map[string]bool)

	// Process and write map materials
	for i, mat := range mapModel.BaseModel.materials {
		matName := sanitizeMaterialName(mat.Name)
		if matName == "" {
			matName = fmt.Sprintf("map_material_%d", i)
		}

		// Only add material if we haven't seen it before
		if !materialMap[matName] {
			materialMap[matName] = true

			// Write material to MTL file
			mtlFile.WriteString(fmt.Sprintf("newmtl %s\n", matName))

			// Define material properties based on transparency
			if mat.IsTransparent {
				// Semi-transparent material
				mtlFile.WriteString("Kd 0.7 0.7 0.9\n")                 // Bluish color for transparent materials
				mtlFile.WriteString(fmt.Sprintf("d %f\n", mat.Opacity)) // Transparency
			} else {
				// Opaque material
				mtlFile.WriteString("Kd 0.8 0.8 0.8\n") // Light gray for opaque materials
				mtlFile.WriteString("d 1.0\n")          // Fully opaque
			}

			mtlFile.WriteString("Ka 0.1 0.1 0.1\n") // Ambient color
			mtlFile.WriteString("Ks 0.5 0.5 0.5\n") // Specular color
			mtlFile.WriteString("Ns 10.0\n\n")      // Specular exponent
		}
	}

	// Add player model materials
	mtlFile.WriteString("# Player model materials\n")

	// Create a unique material for each player model
	for i := range models {
		playerMatName := fmt.Sprintf("player_%d_material", i+1)
		mtlFile.WriteString(fmt.Sprintf("newmtl %s\n", playerMatName))

		// Generate a unique color for each player
		// This creates a gradient from red to blue across multiple players
		r := 0.8 - (float64(i) * 0.8 / float64(len(models)))
		g := 0.2
		b := 0.2 + (float64(i) * 0.6 / float64(len(models)))

		mtlFile.WriteString(fmt.Sprintf("Kd %.2f %.2f %.2f\n", r, g, b))
		mtlFile.WriteString("Ka 0.2 0.2 0.2\n") // Ambient color
		mtlFile.WriteString("Ks 0.5 0.5 0.5\n") // Specular color
		mtlFile.WriteString("Ns 20.0\n")        // Specular exponent
		mtlFile.WriteString("d 1.0\n\n")        // Fully opaque
	}

	// Add hitbox material
	mtlFile.WriteString("newmtl hitbox_material\n")
	mtlFile.WriteString("Kd 1.0 0.0 0.0\n") // Red for hitboxes
	mtlFile.WriteString("Ka 0.2 0.0 0.0\n") // Ambient color
	mtlFile.WriteString("Ks 0.5 0.0 0.0\n") // Specular color
	mtlFile.WriteString("Ns 10.0\n")        // Specular exponent
	mtlFile.WriteString("d 0.3\n\n")        // Mostly transparent

	// Start with the map mesh
	file.WriteString("g map_mesh\n\n")

	// Track the current vertex index (OBJ indices start at 1)
	currentVertexIndex := 1

	// Group triangles by material for better organization
	mapMaterialGroups := make(map[string][]types.Triangle)
	mapMaterialIndices := make(map[string][]int)

	// Organize map triangles by material
	for i, tri := range mapModel.BaseModel.triangles {
		matName := "default"
		if i < len(mapModel.BaseModel.materials) {
			matName = sanitizeMaterialName(mapModel.BaseModel.materials[i].Name)
			if matName == "" {
				matName = fmt.Sprintf("map_material_%d", i)
			}
		}

		mapMaterialGroups[matName] = append(mapMaterialGroups[matName], tri)
		mapMaterialIndices[matName] = append(mapMaterialIndices[matName], currentVertexIndex)
		currentVertexIndex += 3 // Each triangle adds 3 vertices
	}

	// Write the map vertices first
	for _, triangles := range mapMaterialGroups {
		for _, tri := range triangles {
			// Write the three vertices of this triangle
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V1.X, tri.V1.Y, tri.V1.Z))
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V2.X, tri.V2.Y, tri.V2.Z))
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V3.X, tri.V3.Y, tri.V3.Z))
		}
	}

	// Track player model vertices
	playerVertexStart := currentVertexIndex

	// Write each player model's vertices
	for modelIndex, model := range models {
		position := positions[modelIndex]

		// Log transformation for debugging
		fmt.Printf("Adding player %d at position: %.2f, %.2f, %.2f\n",
			modelIndex+1, position.X, position.Y, position.Z)

		// Write the player model vertices (transformed to match map space)
		for i, tri := range model.triangles {
			if i < 3 { // Just debug the first 3 triangles
				fmt.Printf("DEBUG: Triangle %d original vertices: (%.2f,%.2f,%.2f), (%.2f,%.2f,%.2f), (%.2f,%.2f,%.2f)\n",
					i, tri.V1.X, tri.V1.Y, tri.V1.Z, tri.V2.X, tri.V2.Y, tri.V2.Z, tri.V3.X, tri.V3.Y, tri.V3.Z)
			}

			// Apply position offset to each vertex (no further transformation needed)
			v1 := r3.Vector{
				X: tri.V1.X + position.X,
				Y: tri.V1.Y + position.Y,
				Z: tri.V1.Z + position.Z,
			}

			// Log the actual vertices being written
			if i < 3 {
				fmt.Printf("DEBUG: Triangle %d transformed vertex 1: (%.2f,%.2f,%.2f)\n", i, v1.X, v1.Y, v1.Z)
			}

			v2 := r3.Vector{
				X: tri.V2.X + position.X,
				Y: tri.V2.Y + position.Y,
				Z: tri.V2.Z + position.Z,
			}
			v3 := r3.Vector{
				X: tri.V3.X + position.X,
				Y: tri.V3.Y + position.Y,
				Z: tri.V3.Z + position.Z,
			}

			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z))
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z))
			file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z))

			currentVertexIndex += 3
		}
	}

	file.WriteString("\n")

	// Write map faces grouped by material
	for matName, triangles := range mapMaterialGroups {
		file.WriteString(fmt.Sprintf("g map_%s\n", matName))
		file.WriteString(fmt.Sprintf("usemtl %s\n", matName))

		// Get the starting indices for this material
		indices := mapMaterialIndices[matName]

		// Write faces for this material
		for i := range triangles {
			baseIdx := indices[i]
			file.WriteString(fmt.Sprintf("f %d %d %d\n",
				baseIdx, baseIdx+1, baseIdx+2))
		}

		file.WriteString("\n")
	}

	// Write player model faces
	playerVertexIndex := playerVertexStart

	for modelIndex, model := range models {
		playerMatName := fmt.Sprintf("player_%d_material", modelIndex+1)
		file.WriteString(fmt.Sprintf("g player_%d\n", modelIndex+1))
		file.WriteString(fmt.Sprintf("usemtl %s\n", playerMatName))

		// Write player model faces
		for i := 0; i < len(model.triangles); i++ {
			file.WriteString(fmt.Sprintf("f %d %d %d\n",
				playerVertexIndex, playerVertexIndex+1, playerVertexIndex+2))
			playerVertexIndex += 3
		}

		file.WriteString("\n")
	}

	return nil
}

// ExportMapWithPlayerAtCS2Position correctly positions player models at CS2 coordinates
func ExportMapWithPlayerAtCS2Position(mapModel *MapModel, playerModel *Model, cs2Position r3.Vector, outputPath string) error {
	// Create a Source2Coordinates transformer
	coords := NewDefaultSource2Coordinates()

	// Transform the CS2 position to model space
	transformedPosition := coords.CS2ToModelSpace(cs2Position)

	// Log the transformation for debugging
	slog.Info("Exporting player at position",
		"cs2Position", cs2Position,
		"transformedPosition", transformedPosition)

	// Now export using the transformed position
	return ExportCombinedModelToOBJ2(mapModel, []*Model{playerModel}, []r3.Vector{transformedPosition}, outputPath)
}

// Helper function to convert world coordinates to proper positions for rendering
func ExportMapWithPlayerAtPosition(mapModel *MapModel, playerModel *Model, position r3.Vector, outputPath string) error {
	coords := NewDefaultSource2Coordinates()
	transformedPosition := coords.CS2ToModelSpace(position)
	return ExportCombinedModelToOBJ2(mapModel, []*Model{playerModel}, []r3.Vector{transformedPosition}, outputPath)
}

// VisualizePositionWithDebugPoints creates a debug visualization showing the
// player model at a specific CS2 position, along with coordinate axes
func VisualizePositionWithDebugPoints(mapModel *MapModel, playerModel *Model,
	cs2Position r3.Vector, outputPath string) error {

	coords := NewDefaultSource2Coordinates()
	modelPosition := coords.CS2ToModelSpace(cs2Position)

	// Log transformation for debugging
	slog.Info("Visualizing position with debug points",
		"cs2Position", cs2Position,
		"modelPosition", modelPosition)

	// Create a combined model with the player and coordinate axes
	return ExportDebugVisualization(mapModel, playerModel, cs2Position,
		modelPosition, outputPath)
}

// ExportDebugVisualization exports a visual debug aid with coordinate axes
// This updated version keeps all existing functionality while adding an overload that supports
// shooter and target data visualization
func ExportDebugVisualization(mapModel *MapModel, playerModel *Model,
	cs2Position, modelPosition r3.Vector, outputPath string, extraParams ...interface{}) error {

	// Check if shooter and target data are provided in extraParams
	if len(extraParams) >= 2 {
		// Try to cast to PlayerTickData pointers
		shooterData, shooterOk := extraParams[0].(*types.PlayerTickData)
		targetData, targetOk := extraParams[1].(*types.PlayerTickData)

		if shooterOk && targetOk {
			// Use the enhanced visualization with vectors
			return ExportDebugVisualizationWithVectors(
				mapModel, playerModel, cs2Position, modelPosition,
				shooterData, targetData, outputPath)
		}
	}

	// Generate a modified output path for the visualization
	axesOutputPath := outputPath
	if len(outputPath) > 4 && outputPath[len(outputPath)-4:] == ".obj" {
		axesOutputPath = outputPath[:len(outputPath)-4] + "_with_axes.obj"
	} else {
		axesOutputPath = outputPath + "_with_axes.obj"
	}

	// Original functionality - export model with axes
	err := ExportCombinedModelWithAxes(mapModel, playerModel, cs2Position, modelPosition, axesOutputPath)
	if err != nil {
		return err
	}

	return nil
}

// ExportDebugVisualizationWithVectors exports a visual debug aid with coordinate axes, forward vectors, and ray traces
func ExportDebugVisualizationWithVectors(
	mapModel *MapModel, playerModel *Model,
	cs2Position, modelPosition r3.Vector,
	shooterData, targetData *types.PlayerTickData,
	outputPath string) error {

	// Generate a modified output path for the visualization
	axesOutputPath := outputPath
	if len(outputPath) > 4 && outputPath[len(outputPath)-4:] == ".obj" {
		axesOutputPath = outputPath[:len(outputPath)-4] + "_with_vectors.obj"
	} else {
		axesOutputPath = outputPath + "_with_vectors.obj"
	}

	// Export the model with vectors
	err := ExportCombinedModelWithVectors(mapModel, playerModel, cs2Position, modelPosition,
		shooterData, targetData, axesOutputPath)
	if err != nil {
		return err
	}

	return nil
}

// ExportCombinedModelWithVectors exports the map and player model with coordinate axes,
// forward vectors, and ray cast visualizations
func ExportCombinedModelWithVectors(
	mapModel *MapModel, playerModel *Model,
	cs2Position, modelPosition r3.Vector,
	shooterData, targetData *types.PlayerTickData,
	outputPath string) error {

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Create a material library file (MTL)
	mtlPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".mtl"
	mtlFile, err := os.Create(mtlPath)
	if err != nil {
		return fmt.Errorf("failed to create MTL file: %v", err)
	}
	defer mtlFile.Close()

	// Write header information
	file.WriteString("# Debug visualization with coordinate axes and forward vectors\n")
	file.WriteString(fmt.Sprintf("# CS2 Position: (%.2f, %.2f, %.2f)\n",
		cs2Position.X, cs2Position.Y, cs2Position.Z))
	file.WriteString(fmt.Sprintf("# Model Position: (%.2f, %.2f, %.2f)\n\n",
		modelPosition.X, modelPosition.Y, modelPosition.Z))

	// Reference the MTL file
	mtlFilename := filepath.Base(mtlPath)
	file.WriteString(fmt.Sprintf("mtllib %s\n\n", mtlFilename))

	// Write material definitions to MTL file
	mtlFile.WriteString("# Material definitions\n")

	// Material for map
	mtlFile.WriteString("newmtl map_material\n")
	mtlFile.WriteString("Kd 0.8 0.8 0.8\n")
	mtlFile.WriteString("Ka 0.2 0.2 0.2\n")
	mtlFile.WriteString("Ks 0.1 0.1 0.1\n")
	mtlFile.WriteString("Ns 10.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Material for player
	mtlFile.WriteString("newmtl player_material\n")
	mtlFile.WriteString("Kd 0.2 0.5 0.8\n")
	mtlFile.WriteString("Ka 0.1 0.1 0.2\n")
	mtlFile.WriteString("Ks 0.2 0.2 0.5\n")
	mtlFile.WriteString("Ns 20.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Materials for coordinate axes
	mtlFile.WriteString("newmtl x_axis\n")
	mtlFile.WriteString("Kd 1.0 0.0 0.0\n") // Red for X axis
	mtlFile.WriteString("Ka 0.3 0.0 0.0\n")
	mtlFile.WriteString("Ks 1.0 0.0 0.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	mtlFile.WriteString("newmtl y_axis\n")
	mtlFile.WriteString("Kd 0.0 1.0 0.0\n") // Green for Y axis
	mtlFile.WriteString("Ka 0.0 0.3 0.0\n")
	mtlFile.WriteString("Ks 0.0 1.0 0.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	mtlFile.WriteString("newmtl z_axis\n")
	mtlFile.WriteString("Kd 0.0 0.0 1.0\n") // Blue for Z axis
	mtlFile.WriteString("Ka 0.0 0.0 0.3\n")
	mtlFile.WriteString("Ks 0.0 0.0 1.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Material for shooter forward vector
	mtlFile.WriteString("newmtl shooter_forward\n")
	mtlFile.WriteString("Kd 1.0 0.5 0.0\n") // Orange for shooter forward vector
	mtlFile.WriteString("Ka 0.3 0.2 0.0\n")
	mtlFile.WriteString("Ks 1.0 0.5 0.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Material for target forward vector
	mtlFile.WriteString("newmtl target_forward\n")
	mtlFile.WriteString("Kd 0.7 0.0 1.0\n") // Purple for target forward vector
	mtlFile.WriteString("Ka 0.2 0.0 0.3\n")
	mtlFile.WriteString("Ks 0.7 0.0 1.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Material for ray traces
	mtlFile.WriteString("newmtl ray_trace\n")
	mtlFile.WriteString("Kd 1.0 1.0 0.0\n") // Yellow for ray traces
	mtlFile.WriteString("Ka 0.3 0.3 0.0\n")
	mtlFile.WriteString("Ks 1.0 1.0 0.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 0.7\n\n") // Slightly transparent

	// Material for eye position
	mtlFile.WriteString("newmtl eye_position\n")
	mtlFile.WriteString("Kd 1.0 0.0 0.0\n") // Red for eye position
	mtlFile.WriteString("Ka 0.5 0.0 0.0\n")
	mtlFile.WriteString("Ks 1.0 0.0 0.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Material for target sample points
	mtlFile.WriteString("newmtl sample_point\n")
	mtlFile.WriteString("Kd 0.0 1.0 1.0\n") // Cyan for sample points
	mtlFile.WriteString("Ka 0.0 0.3 0.3\n")
	mtlFile.WriteString("Ks 0.0 1.0 1.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Start writing vertices

	// First write the map model triangles
	file.WriteString("g map_model\n")
	file.WriteString("usemtl map_material\n\n")

	for _, tri := range mapModel.BaseModel.triangles {
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V1.X, tri.V1.Y, tri.V1.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V2.X, tri.V2.Y, tri.V2.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V3.X, tri.V3.Y, tri.V3.Z))
	}

	// Write map faces
	for i := 0; i < len(mapModel.BaseModel.triangles); i++ {
		baseIdx := 1 + (i * 3)
		file.WriteString(fmt.Sprintf("f %d %d %d\n", baseIdx, baseIdx+1, baseIdx+2))
	}

	// Track player vertex index start
	playerVertexStart := 1 + (len(mapModel.BaseModel.triangles) * 3)

	// Write player model triangles
	file.WriteString("\ng player_model\n")
	file.WriteString("usemtl player_material\n\n")

	for _, tri := range playerModel.triangles {
		// Translate to player position
		v1 := r3.Vector{X: tri.V1.X + modelPosition.X, Y: tri.V1.Y + modelPosition.Y, Z: tri.V1.Z + modelPosition.Z}
		v2 := r3.Vector{X: tri.V2.X + modelPosition.X, Y: tri.V2.Y + modelPosition.Y, Z: tri.V2.Z + modelPosition.Z}
		v3 := r3.Vector{X: tri.V3.X + modelPosition.X, Y: tri.V3.Y + modelPosition.Y, Z: tri.V3.Z + modelPosition.Z}

		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z))
	}

	// Write player faces
	for i := 0; i < len(playerModel.triangles); i++ {
		baseIdx := playerVertexStart + (i * 3)
		file.WriteString(fmt.Sprintf("f %d %d %d\n", baseIdx, baseIdx+1, baseIdx+2))
	}

	// Calculate the next vertex index
	nextVertexIndex := playerVertexStart + (len(playerModel.triangles) * 3)

	// Add coordinate axes - these are just lines, not triangles
	// Define axis length - longer for better visibility
	axisLength := 50.0 // Adjust this as needed

	file.WriteString("\n# Coordinate axes\n")

	// X axis (Red)
	file.WriteString("g x_axis\n")
	file.WriteString("usemtl x_axis\n")

	// Origin vertex
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z))
	// X-axis end point
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X+axisLength, modelPosition.Y, modelPosition.Z))
	// Connect with a line
	file.WriteString(fmt.Sprintf("l %d %d\n\n", nextVertexIndex, nextVertexIndex+1))

	nextVertexIndex += 2

	// Y axis (Green)
	file.WriteString("g y_axis\n")
	file.WriteString("usemtl y_axis\n")

	// Origin vertex
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z))
	// Y-axis end point
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y+axisLength, modelPosition.Z))
	// Connect with a line
	file.WriteString(fmt.Sprintf("l %d %d\n\n", nextVertexIndex, nextVertexIndex+1))

	nextVertexIndex += 2

	// Z axis (Blue)
	file.WriteString("g z_axis\n")
	file.WriteString("usemtl z_axis\n")

	// Origin vertex
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z))
	// Z-axis end point
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z+axisLength))
	// Connect with a line
	file.WriteString(fmt.Sprintf("l %d %d\n\n", nextVertexIndex, nextVertexIndex+1))

	nextVertexIndex += 2

	// Now add forward vectors for shooter and target if they are provided
	if shooterData != nil {
		// First determine the eye position in model space
		coords := NewDefaultSource2Coordinates()

		// Calculate eye position in CS2 coordinates
		eyeHeight := 64.0
		if shooterData.IsCrouched {
			eyeHeight = 46.0
		}

		// Calculate eye position in CS2 coordinates
		shooterEyePos := r3.Vector{
			X: shooterData.Position.X,
			Y: shooterData.Position.Y,
			Z: shooterData.Position.Z + eyeHeight,
		}

		// Transform to model space
		modelEyePos := coords.CS2ToModelSpace(shooterEyePos)

		// Add a marker for the eye position
		file.WriteString("g eye_position\n")
		file.WriteString("usemtl eye_position\n")

		// Eye position vertex
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelEyePos.X, modelEyePos.Y, modelEyePos.Z))
		nextVertexIndex += 1

		// Create a small sphere to represent the eye position (using 1 vertex for now)
		// In a more complex implementation, we could add a small sphere mesh here

		// Add shooter forward vector
		file.WriteString("g shooter_forward\n")
		file.WriteString("usemtl shooter_forward\n")

		// Get forward vector in CS2 coordinates
		shooterForward := shooterData.ForwardVector()

		// Transform forward vector to model space (note: we need to transform direction differently than position)
		// We can either use the CS2 forward vector and transform it, or directly apply the rotation in model space
		modelForward := coords.CS2ToModelSpace(shooterForward)

		// Alternative approach using rotation directly in model space
		// modelForward := coords.ApplyCS2Rotation(shooterData.ViewAngleX, shooterData.ViewAngleY, r3.Vector{X: 1, Y: 0, Z: 0})

		// Scale forward vector to be visible (use a longer length for better visibility)
		forwardLength := 70.0
		scaledForward := modelForward.Mul(forwardLength)

		// Add vertices for forward vector
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelEyePos.X, modelEyePos.Y, modelEyePos.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n",
			modelEyePos.X+scaledForward.X,
			modelEyePos.Y+scaledForward.Y,
			modelEyePos.Z+scaledForward.Z))

		// Connect with a line
		file.WriteString(fmt.Sprintf("l %d %d\n\n", nextVertexIndex, nextVertexIndex+1))

		nextVertexIndex += 2

		// If we have target data, visualize the sample rays that would be cast in IsShooterPointingAtTarget
		if targetData != nil {
			// Transform the target position to model space
			targetModelPos := coords.CS2ToModelSpace(targetData.Position)

			// Add target forward vector
			if true {
				file.WriteString("g target_forward\n")
				file.WriteString("usemtl target_forward\n")

				// Get target forward vector and transform to model space
				targetForward := targetData.ForwardVector()
				// Transform target forward vector to model space
				modelTargetForward := coords.CS2ToModelSpace(targetForward)

				// Alternative approach using rotation directly
				// modelTargetForward := coords.ApplyCS2Rotation(targetData.ViewAngleX, targetData.ViewAngleY, r3.Vector{X: 1, Y: 0, Z: 0})

				// Scale forward vector
				scaledTargetForward := modelTargetForward.Mul(forwardLength)

				// Add vertices for target forward vector
				file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", targetModelPos.X, targetModelPos.Y, targetModelPos.Z))
				file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n",
					targetModelPos.X+scaledTargetForward.X,
					targetModelPos.Y+scaledTargetForward.Y,
					targetModelPos.Z+scaledTargetForward.Z))

				// Connect with a line
				file.WriteString(fmt.Sprintf("l %d %d\n\n", nextVertexIndex, nextVertexIndex+1))

				nextVertexIndex += 2
			}

			// Generate and visualize the sample points that would be used in IsShooterPointingAtTarget
			file.WriteString("g sample_points\n")
			file.WriteString("usemtl sample_point\n")

			// Standard player dimensions in CS2 units (matching the values in generateTargetSamplePoints)
			const (
				playerHeight = 72.0
				playerWidth  = 32.0
			)

			// Generate the same sample points as in generateTargetSamplePoints
			samplePoints := []r3.Vector{
				// Center position
				targetModelPos,

				// Head level (top)
				{X: targetModelPos.X, Y: targetModelPos.Y, Z: targetModelPos.Z + playerHeight*0.85},

				// Chest level (upper body)
				{X: targetModelPos.X, Y: targetModelPos.Y, Z: targetModelPos.Z + playerHeight*0.65},

				// Waist level (mid body)
				{X: targetModelPos.X, Y: targetModelPos.Y, Z: targetModelPos.Z + playerHeight*0.45},

				// Legs (lower body)
				{X: targetModelPos.X, Y: targetModelPos.Y, Z: targetModelPos.Z + playerHeight*0.25},

				// Right side
				{X: targetModelPos.X + playerWidth*0.35, Y: targetModelPos.Y, Z: targetModelPos.Z + playerHeight*0.6},
				// Left side
				{X: targetModelPos.X - playerWidth*0.35, Y: targetModelPos.Y, Z: targetModelPos.Z + playerHeight*0.6},
				// Front
				{X: targetModelPos.X, Y: targetModelPos.Y + playerWidth*0.35, Z: targetModelPos.Z + playerHeight*0.6},
				// Back
				{X: targetModelPos.X, Y: targetModelPos.Y - playerWidth*0.35, Z: targetModelPos.Z + playerHeight*0.6},

				// Corners (diagonal offsets) at chest height
				{X: targetModelPos.X + playerWidth*0.3, Y: targetModelPos.Y + playerWidth*0.3, Z: targetModelPos.Z + playerHeight*0.6},
				{X: targetModelPos.X + playerWidth*0.3, Y: targetModelPos.Y - playerWidth*0.3, Z: targetModelPos.Z + playerHeight*0.6},
				{X: targetModelPos.X - playerWidth*0.3, Y: targetModelPos.Y + playerWidth*0.3, Z: targetModelPos.Z + playerHeight*0.6},
				{X: targetModelPos.X - playerWidth*0.3, Y: targetModelPos.Y - playerWidth*0.3, Z: targetModelPos.Z + playerHeight*0.6},
			}

			// Add vertices for each sample point
			sampleVertices := make([]int, len(samplePoints))
			for i, point := range samplePoints {
				file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", point.X, point.Y, point.Z))
				sampleVertices[i] = nextVertexIndex
				nextVertexIndex++
			}

			// Now add ray traces from eye position to each sample point
			file.WriteString("g ray_traces\n")
			file.WriteString("usemtl ray_trace\n")

			for _, sampleVertex := range sampleVertices {
				// Create a line from eye position to sample point
				file.WriteString(fmt.Sprintf("l %d %d\n", nextVertexIndex-len(samplePoints)-2, sampleVertex))
			}
		}
	}

	return nil
}

// ExportCombinedModelWithAxes exports the map and player model with coordinate axes at the player position
func ExportCombinedModelWithAxes(mapModel *MapModel, playerModel *Model,
	cs2Position, modelPosition r3.Vector, outputPath string) error {

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Create a material library file (MTL)
	mtlPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".mtl"
	mtlFile, err := os.Create(mtlPath)
	if err != nil {
		return fmt.Errorf("failed to create MTL file: %v", err)
	}
	defer mtlFile.Close()

	// Write header information
	file.WriteString("# Debug visualization with coordinate axes\n")
	file.WriteString(fmt.Sprintf("# CS2 Position: (%.2f, %.2f, %.2f)\n",
		cs2Position.X, cs2Position.Y, cs2Position.Z))
	file.WriteString(fmt.Sprintf("# Model Position: (%.2f, %.2f, %.2f)\n\n",
		modelPosition.X, modelPosition.Y, modelPosition.Z))

	// Reference the MTL file
	mtlFilename := filepath.Base(mtlPath)
	file.WriteString(fmt.Sprintf("mtllib %s\n\n", mtlFilename))

	// Write material definitions to MTL file
	mtlFile.WriteString("# Material definitions\n")

	// Material for map
	mtlFile.WriteString("newmtl map_material\n")
	mtlFile.WriteString("Kd 0.8 0.8 0.8\n")
	mtlFile.WriteString("Ka 0.2 0.2 0.2\n")
	mtlFile.WriteString("Ks 0.1 0.1 0.1\n")
	mtlFile.WriteString("Ns 10.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Material for player
	mtlFile.WriteString("newmtl player_material\n")
	mtlFile.WriteString("Kd 0.2 0.5 0.8\n")
	mtlFile.WriteString("Ka 0.1 0.1 0.2\n")
	mtlFile.WriteString("Ks 0.2 0.2 0.5\n")
	mtlFile.WriteString("Ns 20.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Materials for coordinate axes
	mtlFile.WriteString("newmtl x_axis\n")
	mtlFile.WriteString("Kd 1.0 0.0 0.0\n") // Red for X axis
	mtlFile.WriteString("Ka 0.3 0.0 0.0\n")
	mtlFile.WriteString("Ks 1.0 0.0 0.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	mtlFile.WriteString("newmtl y_axis\n")
	mtlFile.WriteString("Kd 0.0 1.0 0.0\n") // Green for Y axis
	mtlFile.WriteString("Ka 0.0 0.3 0.0\n")
	mtlFile.WriteString("Ks 0.0 1.0 0.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	mtlFile.WriteString("newmtl z_axis\n")
	mtlFile.WriteString("Kd 0.0 0.0 1.0\n") // Blue for Z axis
	mtlFile.WriteString("Ka 0.0 0.0 0.3\n")
	mtlFile.WriteString("Ks 0.0 0.0 1.0\n")
	mtlFile.WriteString("Ns 100.0\n")
	mtlFile.WriteString("d 1.0\n\n")

	// Start writing vertices

	// First write the map model triangles
	file.WriteString("g map_model\n")
	file.WriteString("usemtl map_material\n\n")

	for _, tri := range mapModel.BaseModel.triangles {
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V1.X, tri.V1.Y, tri.V1.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V2.X, tri.V2.Y, tri.V2.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", tri.V3.X, tri.V3.Y, tri.V3.Z))
	}

	// Write map faces
	for i := 0; i < len(mapModel.BaseModel.triangles); i++ {
		baseIdx := 1 + (i * 3)
		file.WriteString(fmt.Sprintf("f %d %d %d\n", baseIdx, baseIdx+1, baseIdx+2))
	}

	// Track player vertex index start
	playerVertexStart := 1 + (len(mapModel.BaseModel.triangles) * 3)

	// Write player model triangles
	file.WriteString("\ng player_model\n")
	file.WriteString("usemtl player_material\n\n")

	for _, tri := range playerModel.triangles {
		// Translate to player position
		v1 := r3.Vector{X: tri.V1.X + modelPosition.X, Y: tri.V1.Y + modelPosition.Y, Z: tri.V1.Z + modelPosition.Z}
		v2 := r3.Vector{X: tri.V2.X + modelPosition.X, Y: tri.V2.Y + modelPosition.Y, Z: tri.V2.Z + modelPosition.Z}
		v3 := r3.Vector{X: tri.V3.X + modelPosition.X, Y: tri.V3.Y + modelPosition.Y, Z: tri.V3.Z + modelPosition.Z}

		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z))
		file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z))
	}

	// Write player faces
	for i := 0; i < len(playerModel.triangles); i++ {
		baseIdx := playerVertexStart + (i * 3)
		file.WriteString(fmt.Sprintf("f %d %d %d\n", baseIdx, baseIdx+1, baseIdx+2))
	}

	// Calculate the next vertex index
	nextVertexIndex := playerVertexStart + (len(playerModel.triangles) * 3)

	// Add coordinate axes - these are just lines, not triangles
	// Define axis length - longer for better visibility
	axisLength := 50.0 // Adjust this as needed

	file.WriteString("\n# Coordinate axes\n")

	// X axis (Red)
	file.WriteString("g x_axis\n")
	file.WriteString("usemtl x_axis\n")

	// Origin vertex (already defined in the model, but define again for clarity)
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z))
	// X-axis end point
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X+axisLength, modelPosition.Y, modelPosition.Z))
	// Connect with a line
	file.WriteString(fmt.Sprintf("l %d %d\n\n", nextVertexIndex, nextVertexIndex+1))

	nextVertexIndex += 2

	// Y axis (Green)
	file.WriteString("g y_axis\n")
	file.WriteString("usemtl y_axis\n")

	// Origin vertex
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z))
	// Y-axis end point
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y+axisLength, modelPosition.Z))
	// Connect with a line
	file.WriteString(fmt.Sprintf("l %d %d\n\n", nextVertexIndex, nextVertexIndex+1))

	nextVertexIndex += 2

	// Z axis (Blue)
	file.WriteString("g z_axis\n")
	file.WriteString("usemtl z_axis\n")

	// Origin vertex
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z))
	// Z-axis end point
	file.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", modelPosition.X, modelPosition.Y, modelPosition.Z+axisLength))
	// Connect with a line
	file.WriteString(fmt.Sprintf("l %d %d\n", nextVertexIndex, nextVertexIndex+1))

	return nil
}

// Helper function to sanitize material names for OBJ files
func sanitizeMaterialName(name string) string {
	// Replace spaces and special characters with underscores
	result := strings.ReplaceAll(name, " ", "_")
	result = strings.ReplaceAll(result, "/", "_")
	result = strings.ReplaceAll(result, "\\", "_")
	result = strings.ReplaceAll(result, ":", "_")
	result = strings.ReplaceAll(result, "*", "_")
	result = strings.ReplaceAll(result, "?", "_")
	result = strings.ReplaceAll(result, "\"", "_")
	result = strings.ReplaceAll(result, "<", "_")
	result = strings.ReplaceAll(result, ">", "_")
	result = strings.ReplaceAll(result, "|", "_")

	// Ensure the name is not empty
	if result == "" {
		return "material"
	}

	return result
}
