import bpy

def select_objects_by_material_name(material_name):
    # Deselect all objects first
    bpy.ops.object.select_all(action='DESELECT')
    
    # Counter for selected objects
    selected_count = 0
    
    # Iterate through all objects in the scene
    for obj in bpy.context.scene.objects:
        # Check if the object has materials
        if obj.type == 'MESH' and obj.data.materials:
            # Check each material slot
            for material_slot in obj.material_slots:
                # If the material exists and matches the name
                if material_slot.material and material_slot.material.name == material_name:
                    # Select the object
                    obj.select_set(True)
                    selected_count += 1
                    break  # No need to check other materials once we found a match
    
    # Set the active object to the last selected object if any were selected
    if selected_count > 0:
        # Set the active object to one of the selected objects
        for obj in bpy.context.selected_objects:
            bpy.context.view_layer.objects.active = obj
            break
        print(f"Selected {selected_count} objects with material '{material_name}'")
    else:
        print(f"No objects found with material '{material_name}'")

# Call the function with the specific material name
select_objects_by_material_name("de_mirage_top_ver1_blend")