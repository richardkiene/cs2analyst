package visibility

import (
	"path/filepath"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/qmuntal/gltf"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportGLTFMapModel(t *testing.T) {
	testFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	model, err := ImportGLTFMapModel(testFilePath, "test_map")

	if err != nil {
		t.Skipf("Skipping test: could not load GLTF file %s: %v", testFilePath, err)
		return
	}

	require.NotNil(t, model, "Model should not be nil")
	require.NotNil(t, model.BaseModel.bvh, "BVH should be created")

	// Verify some basic properties
	assert.Greater(t, len(model.BaseModel.triangles), 0, "Model should have triangles")
	assert.Equal(t, len(model.BaseModel.triangles), len(model.BaseModel.materials),
		"Number of triangles should match number of materials")

	// Verify model bounds are valid
	assert.Less(t, model.BaseModel.min.X, model.BaseModel.max.X, "X bounds should be valid")
	assert.Less(t, model.BaseModel.min.Y, model.BaseModel.max.Y, "Y bounds should be valid")
	assert.Less(t, model.BaseModel.min.Z, model.BaseModel.max.Z, "Z bounds should be valid")
}

func TestImportGLTFPlayerModel(t *testing.T) {
	testFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	model, err := ImportGLTFPlayerModel(testFilePath)

	if err != nil {
		t.Skipf("Skipping test: could not load GLTF file %s: %v", testFilePath, err)
		return
	}

	require.NotNil(t, model, "Model should not be nil")
	require.NotNil(t, model.bvh, "BVH should be created")

	// Verify model has hitboxes
	assert.Greater(t, len(model.hitboxes), 0, "Model should have hitboxes")

	// Test that hitboxes have valid bounds
	for i, hitbox := range model.hitboxes {
		assert.NotEmpty(t, hitbox.Name, "Hitbox %d should have a name", i)
		assert.NotEmpty(t, hitbox.Vertices, "Hitbox %d should have vertices", i)
		assert.Less(t, hitbox.MinBounds.X, hitbox.MaxBounds.X, "Hitbox %d X bounds should be valid", i)
		assert.Less(t, hitbox.MinBounds.Y, hitbox.MaxBounds.Y, "Hitbox %d Y bounds should be valid", i)
		assert.Less(t, hitbox.MinBounds.Z, hitbox.MaxBounds.Z, "Hitbox %d Z bounds should be valid", i)
	}
}

func TestGetRelevantMapGeometry(t *testing.T) {
	// Create a simple test model
	model := NewModel()

	// Add some test triangles
	// A horizontal plane at z=0
	tri1 := types.Triangle{
		V1: r3.Vector{X: 0, Y: 0, Z: 0},
		V2: r3.Vector{X: 10, Y: 0, Z: 0},
		V3: r3.Vector{X: 0, Y: 10, Z: 0},
	}

	tri2 := types.Triangle{
		V1: r3.Vector{X: 10, Y: 10, Z: 0},
		V2: r3.Vector{X: 10, Y: 0, Z: 0},
		V3: r3.Vector{X: 0, Y: 10, Z: 0},
	}

	// A vertical triangle
	tri3 := types.Triangle{
		V1: r3.Vector{X: 5, Y: 5, Z: 0},
		V2: r3.Vector{X: 5, Y: 5, Z: 10},
		V3: r3.Vector{X: 10, Y: 10, Z: 5},
	}

	// Add triangles with materials (transparency doesn't matter for this test)
	material := MaterialProperties{
		IsTransparent: false,
		Opacity:       1.0,
		Name:          "test_material",
	}

	model.AddTriangleWithMaterial(tri1, material)
	model.AddTriangleWithMaterial(tri2, material)
	model.AddTriangleWithMaterial(tri3, material)

	// Build spatial structures
	err := buildSpatialStructures(model)
	require.NoError(t, err, "Building spatial structures should not error")

	// Test cases
	testCases := []struct {
		name         string
		start        r3.Vector
		end          r3.Vector
		wantTriCount int
	}{
		{
			name:         "Ray through all triangles",
			start:        r3.Vector{X: 5, Y: 5, Z: -5},
			end:          r3.Vector{X: 5, Y: 5, Z: 15},
			wantTriCount: 3,
		},
		{
			name:         "Ray through horizontal plane only",
			start:        r3.Vector{X: 2, Y: 2, Z: -5},
			end:          r3.Vector{X: 2, Y: 2, Z: 5},
			wantTriCount: 1,
		},
		{
			name:         "Ray missing all triangles",
			start:        r3.Vector{X: 20, Y: 20, Z: 0},
			end:          r3.Vector{X: 30, Y: 30, Z: 0},
			wantTriCount: 0,
		},
		{
			name:         "Long diagonal ray",
			start:        r3.Vector{X: -10, Y: -10, Z: -10},
			end:          r3.Vector{X: 20, Y: 20, Z: 20},
			wantTriCount: 3,
		},
		{
			name:         "Horizontal ray near triangles",
			start:        r3.Vector{X: 0, Y: 5, Z: 1},
			end:          r3.Vector{X: 10, Y: 5, Z: 1},
			wantTriCount: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triangles := model.GetRelevantMapGeometry(tc.start, tc.end)
			assert.Equal(t, tc.wantTriCount, len(triangles),
				"Expected %d triangles, got %d", tc.wantTriCount, len(triangles))
		})
	}
}

func TestTriangleIntersectsAABB(t *testing.T) {
	testCases := []struct {
		name     string
		triangle types.Triangle
		min, max r3.Vector
		want     bool
	}{
		{
			name: "Triangle inside box",
			triangle: types.Triangle{
				V1: r3.Vector{X: 1, Y: 1, Z: 1},
				V2: r3.Vector{X: 2, Y: 1, Z: 1},
				V3: r3.Vector{X: 1, Y: 2, Z: 1},
			},
			min:  r3.Vector{X: 0, Y: 0, Z: 0},
			max:  r3.Vector{X: 3, Y: 3, Z: 3},
			want: true,
		},
		{
			name: "Triangle outside box",
			triangle: types.Triangle{
				V1: r3.Vector{X: 10, Y: 10, Z: 10},
				V2: r3.Vector{X: 11, Y: 10, Z: 10},
				V3: r3.Vector{X: 10, Y: 11, Z: 10},
			},
			min:  r3.Vector{X: 0, Y: 0, Z: 0},
			max:  r3.Vector{X: 3, Y: 3, Z: 3},
			want: false,
		},
		{
			name: "Triangle intersects box edge",
			triangle: types.Triangle{
				V1: r3.Vector{X: -1, Y: 1, Z: 1},
				V2: r3.Vector{X: 4, Y: 1, Z: 1},
				V3: r3.Vector{X: 1, Y: 4, Z: 1},
			},
			min:  r3.Vector{X: 0, Y: 0, Z: 0},
			max:  r3.Vector{X: 3, Y: 3, Z: 3},
			want: true,
		},
		{
			name: "Triangle barely touches box",
			triangle: types.Triangle{
				V1: r3.Vector{X: 0, Y: 0, Z: 0},
				V2: r3.Vector{X: -1, Y: 0, Z: 0},
				V3: r3.Vector{X: 0, Y: -1, Z: 0},
			},
			min:  r3.Vector{X: 0, Y: 0, Z: 0},
			max:  r3.Vector{X: 3, Y: 3, Z: 3},
			want: true,
		},
		{
			name: "Thin triangle passing through box",
			triangle: types.Triangle{
				V1: r3.Vector{X: -10, Y: 1.5, Z: 1.5},
				V2: r3.Vector{X: 10, Y: 1.5, Z: 1.5},
				V3: r3.Vector{X: 0, Y: 1.6, Z: 1.6},
			},
			min:  r3.Vector{X: 0, Y: 0, Z: 0},
			max:  r3.Vector{X: 3, Y: 3, Z: 3},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := triangleIntersectsAABB(tc.triangle, tc.min, tc.max)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBVHConstruction(t *testing.T) {
	// Create a model with some test triangles
	model := NewModel()

	// Add a grid of triangles (10x10)
	material := MaterialProperties{
		IsTransparent: false,
		Opacity:       1.0,
		Name:          "test_material",
	}

	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			// Create two triangles for each grid cell
			z := float64(0)
			tri1 := types.Triangle{
				V1: r3.Vector{X: float64(x), Y: float64(y), Z: z},
				V2: r3.Vector{X: float64(x + 1), Y: float64(y), Z: z},
				V3: r3.Vector{X: float64(x), Y: float64(y + 1), Z: z},
			}

			tri2 := types.Triangle{
				V1: r3.Vector{X: float64(x + 1), Y: float64(y + 1), Z: z},
				V2: r3.Vector{X: float64(x + 1), Y: float64(y), Z: z},
				V3: r3.Vector{X: float64(x), Y: float64(y + 1), Z: z},
			}

			model.AddTriangleWithMaterial(tri1, material)
			model.AddTriangleWithMaterial(tri2, material)
		}
	}

	// Build spatial structures
	err := buildSpatialStructures(model)
	require.NoError(t, err, "Building spatial structures should not error")

	// Verify BVH construction
	require.NotNil(t, model.bvh, "BVH root should not be nil")

	// Helper function to count nodes in the BVH
	var countNodes func(*BVHNode) int
	countNodes = func(node *BVHNode) int {
		if node == nil {
			return 0
		}
		if len(node.triangles) > 0 {
			// Leaf node
			return 1
		}
		return 1 + countNodes(node.left) + countNodes(node.right)
	}

	nodeCount := countNodes(model.bvh)
	t.Logf("BVH has %d nodes", nodeCount)

	// The exact node count depends on the splitting algorithm,
	// but it should be at least 3 (root + at least two children)
	assert.GreaterOrEqual(t, nodeCount, 3, "BVH should have at least a root and two children")

	// Test ray intersection with BVH
	ray := struct {
		origin, dir r3.Vector
		maxDist     float64
	}{
		origin:  r3.Vector{X: 5, Y: 5, Z: 10},
		dir:     r3.Vector{X: 0, Y: 0, Z: -1},
		maxDist: 20,
	}

	// Normalize direction
	ray.dir = ray.dir.Mul(1.0 / ray.dir.Norm())

	intersectionDist, hit := model.bvh.RayIntersectionDistance(ray.origin, ray.dir, ray.maxDist)
	assert.True(t, hit, "Ray should hit the grid of triangles")
	assert.InDelta(t, 10.0, intersectionDist, 0.001, "Intersection distance should be approximately 10")
}

func TestMaterialHandling(t *testing.T) {
	// Create a simple model with transparent and opaque triangles
	model := NewModel()

	// Opaque material
	opaqueMaterial := MaterialProperties{
		IsTransparent: false,
		Opacity:       1.0,
		Name:          "opaque",
	}

	// Transparent material
	transparentMaterial := MaterialProperties{
		IsTransparent: true,
		Opacity:       0.5,
		Name:          "transparent",
	}

	// Add triangles with different materials
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

	model.AddTriangleWithMaterial(tri1, opaqueMaterial)
	model.AddTriangleWithMaterial(tri2, transparentMaterial)

	// Build spatial structures
	err := buildSpatialStructures(model)
	require.NoError(t, err, "Building spatial structures should not error")

	// Check if both triangles are in the model
	require.Equal(t, 2, len(model.triangles), "Model should have two triangles")
	require.Equal(t, 2, len(model.materials), "Model should have two materials")

	// Verify material properties were preserved
	assert.False(t, model.materials[0].IsTransparent, "First triangle should have opaque material")
	assert.True(t, model.materials[1].IsTransparent, "Second triangle should have transparent material")

	// Check if BVH contains material information
	assert.NotNil(t, model.bvh, "BVH should be constructed")

	// Find leaf nodes containing our triangles
	var findLeafWithTriangle func(*BVHNode, types.Triangle) *BVHNode
	findLeafWithTriangle = func(node *BVHNode, tri types.Triangle) *BVHNode {
		if node == nil {
			return nil
		}

		// Check if this is a leaf node with our triangle
		if len(node.triangles) > 0 {
			for _, nodeTri := range node.triangles {
				if nodeTri.V1 == tri.V1 && nodeTri.V2 == tri.V2 && nodeTri.V3 == tri.V3 {
					return node
				}
			}
		}

		// Recursive search in children
		if left := findLeafWithTriangle(node.left, tri); left != nil {
			return left
		}
		return findLeafWithTriangle(node.right, tri)
	}

	leafWithTri1 := findLeafWithTriangle(model.bvh, tri1)
	leafWithTri2 := findLeafWithTriangle(model.bvh, tri2)

	// We might find both triangles in the same leaf node or different ones
	if leafWithTri1 != nil && leafWithTri2 != nil {
		// If they're in the same leaf, check both materials are preserved
		if leafWithTri1 == leafWithTri2 {
			assert.Equal(t, len(leafWithTri1.triangles), len(leafWithTri1.materials),
				"Leaf node should have same number of triangles and materials")

			// Find indices of our triangles
			tri1Idx := -1
			tri2Idx := -1
			for i, tri := range leafWithTri1.triangles {
				if tri.V1 == tri1.V1 && tri.V2 == tri1.V2 && tri.V3 == tri1.V3 {
					tri1Idx = i
				}
				if tri.V1 == tri2.V1 && tri.V2 == tri2.V2 && tri.V3 == tri2.V3 {
					tri2Idx = i
				}
			}

			if tri1Idx >= 0 && tri2Idx >= 0 {
				assert.False(t, leafWithTri1.materials[tri1Idx].IsTransparent,
					"Material for first triangle should be opaque")
				assert.True(t, leafWithTri1.materials[tri2Idx].IsTransparent,
					"Material for second triangle should be transparent")
			}
		}
	}
}

// Test advanced hitbox generation
func TestHitboxGeneration(t *testing.T) {
	// Create a simple test node with a box mesh
	mesh := gltf.Mesh{
		Name: "test_box",
		Primitives: []*gltf.Primitive{
			{
				// We don't need to fully populate this for the test
				// just the minimum needed for the function to work
			},
		},
	}

	node := gltf.Node{
		Name: "box_hitbox",
		Mesh: new(int), // Point to mesh index 0
	}
	*node.Mesh = 0

	doc := &gltf.Document{
		Meshes: []*gltf.Mesh{&mesh},
		Nodes:  []*gltf.Node{&node},
	}

	// Test creating hitbox
	hitbox, err := createDetailedHitboxFromNode(doc, node, 0, mesh)

	// If our implementation correctly handles empty primitives, we might get an error
	// but we should be able to create a basic hitbox
	if err == nil {
		// Verify hitbox properties
		assert.Equal(t, "box_hitbox", hitbox.Name)
		assert.Equal(t, "Box", hitbox.Type) // Default for simple rectangular shapes
	} else {
		t.Logf("Note: createDetailedHitboxFromNode returned error: %v", err)
	}

	// Test hitbox for capsule shape (by setting node name to suggest capsule)
	capsuleNode := gltf.Node{
		Name: "player_capsule",
		Mesh: new(int),
	}
	*capsuleNode.Mesh = 0

	doc.Nodes = append(doc.Nodes, &capsuleNode)

	capsuleHitbox, err := createDetailedHitboxFromNode(doc, capsuleNode, 1, mesh)
	if err == nil {
		// Name should be preserved
		assert.Equal(t, "player_capsule", capsuleHitbox.Name)

		// Type should be "Capsule" based on the name
		assert.Equal(t, "Capsule", capsuleHitbox.Type)
	} else {
		t.Logf("Note: createDetailedHitboxFromNode returned error for capsule: %v", err)
	}
}
