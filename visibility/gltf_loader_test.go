package visibility

import "testing"

func TestImportGLTFMapModel(t *testing.T) {
	model, err := ImportGLTFMapModel("path/to/map_model.gltf", "de_mirage")

	if model == nil || err != nil {
		t.Errorf("Failed to import GLTF map model: %v", err)
	}

	// Test that the model has a non-empty list of meshes
	if len(model.BaseModel.visibilityPoints) == 0 {
		t.Errorf("Imported GLTF map model does not have any meshes")
	}

	// Test that each mesh in the model has vertices, faces, and materials
	/*for _, mesh := range model.navMeshVertices {
	      if len(mesh.Vertices) == 0 || len(mesh.Faces) == 0 || len(mesh.Materials) == 0 {
	          t.Errorf("Imported GLTF map model has a mesh with missing vertices, faces, or materials")
	      }
	  }

	  // Test that the model has a non-empty list of animations
	  if len(model.Animations) == 0 {
	      t.Errorf("Imported GLTF map model does not have any animations")
	  }*/
}
