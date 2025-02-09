package visibility

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/collector"
	"github.com/richardkiene/cs2analyst/types"
)

// convertToBlender flips raw Source 2 coords (X=forward, Y=left, Z=up) into a
// Blender-friendly orientation.  (X, Y, Z) => (Y, -X, Z).
func convertToBlender(v r3.Vector) r3.Vector {
	return r3.Vector{
		X: v.Y,
		Y: -v.X,
		Z: v.Z,
	}
}

// WriteCone creates a visualization of a cone in OBJ format.
func WriteCone(f *os.File, apex, base r3.Vector, baseRadius float64, material string, vertexIndex *int) {
	segments := 16 // how many segments around the cone's base
	f.WriteString(fmt.Sprintf("\no cone_%d\n", *vertexIndex))
	f.WriteString(fmt.Sprintf("usemtl %s\n", material))

	// Write apex vertex
	f.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", apex.X, apex.Y, apex.Z))
	apexIdx := *vertexIndex
	*vertexIndex++

	direction := base.Sub(apex)
	length := direction.Norm()
	direction = direction.Normalize()

	var u r3.Vector
	worldUp := r3.Vector{X: 0, Y: 0, Z: 1}
	if math.Abs(direction.Z) > 0.99 {
		u = r3.Vector{X: 1, Y: 0, Z: 0}
	} else {
		u = direction.Cross(worldUp).Normalize()
	}
	v := direction.Cross(u).Normalize()

	baseVertices := make([]int, segments)
	actualRadius := baseRadius * (length / 100.0)
	for i := 0; i < segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		cosA := math.Cos(angle) * actualRadius
		sinA := math.Sin(angle) * actualRadius
		point := r3.Vector{
			X: base.X + cosA*u.X + sinA*v.X,
			Y: base.Y + cosA*u.Y + sinA*v.Y,
			Z: base.Z + cosA*u.Z + sinA*v.Z,
		}
		f.WriteString(fmt.Sprintf("v %.6f %.6f %.6f\n", point.X, point.Y, point.Z))
		baseVertices[i] = *vertexIndex
		*vertexIndex++
	}

	for i := 0; i < segments; i++ {
		next := (i + 1) % segments
		f.WriteString(fmt.Sprintf("f %d %d %d\n", apexIdx, baseVertices[i], baseVertices[next]))
	}

	f.WriteString("f")
	for i := segments - 1; i >= 0; i-- {
		f.WriteString(fmt.Sprintf(" %d", baseVertices[i]))
	}
	f.WriteString("\n\n")
}

func writeMTLFile(outputDir string) error {
	mtlContent := `# Material for shooter model
newmtl shooter_material
Ka 0.2 0.2 0.2
Kd 0.8 0.2 0.2
Ks 1.0 1.0 1.0
d 1.0

# Material for target model
newmtl target_material
Ka 0.2 0.2 0.2
Kd 0.2 0.8 0.2
Ks 1.0 1.0 1.0
d 1.0

# Material for ray/cone
newmtl ray
Ka 0.2 0.2 0.2
Kd 1.0 1.0 0.0
Ks 1.0 1.0 1.0
d 0.5

# Material for map geometry
newmtl map_material
Ka 0.2 0.2 0.2
Kd 0.5 0.5 0.5
Ks 0.0 0.0 0.0
d 1.0`
	mtlPath := filepath.Join(outputDir, "materials.mtl")
	return os.WriteFile(mtlPath, []byte(mtlContent), 0644)
}

// CreateVisibilityDebugOBJ creates an OBJ to visualize your shooter, target, map geometry,
// and a small cone representing the shooter’s line-of-sight in Blender.
func CreateVisibilityDebugOBJ(
	eyePos, worldPoint r3.Vector,
	triangles []types.Triangle,
	tick int,
	playerModel *Model,
	shooter collector.PlayerTickData,
	target collector.PlayerTickData,
) error {
	outputDir := "debug_visibility"
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return err
	}

	if err := writeMTLFile(outputDir); err != nil {
		return fmt.Errorf("failed to create materials file: %w", err)
	}

	fileName := filepath.Join(outputDir, fmt.Sprintf("debug_tick_%d.obj", tick))
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	vertexIndex := 1

	// Reference the materials
	fmt.Fprintf(writer, "mtllib materials.mtl\n\n")

	// Local transform function: apply the Blender-friendly axis swap
	transform := func(v r3.Vector) r3.Vector {
		return convertToBlender(v)
	}

	// 1) Write the shooter model
	fmt.Fprintf(writer, "o shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.triangles {
		// Each triangle vertex = model vertex + shooter's position
		v1 := transform(tri.V1.Add(shooter.Position))
		v2 := transform(tri.V2.Add(shooter.Position))
		v3 := transform(tri.V3.Add(shooter.Position))

		// Write them
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 2) Write the target model
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transform(tri.V1.Add(target.Position))
		v2 := transform(tri.V2.Add(target.Position))
		v3 := transform(tri.V3.Add(target.Position))

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 3) Write map geometry
	fmt.Fprintf(writer, "\no map_geometry\n")
	fmt.Fprintf(writer, "usemtl map_material\n")
	for _, tri := range triangles {
		v1 := transform(tri.V1)
		v2 := transform(tri.V2)
		v3 := transform(tri.V3)

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 4) Write a small “cone” from eyePos forward
	fmt.Fprintf(writer, "\no debug_ray\n")
	transformedEye := transform(eyePos)
	forwardEnd := eyePos.Add(shooter.ForwardVector().Mul(100))
	transformedEnd := transform(forwardEnd)

	baseRadius := 2.0
	WriteCone(f, transformedEye, transformedEnd, baseRadius, "ray", &vertexIndex)

	writer.Flush()
	return nil
}

// CreateFullDebugOBJ writes a debug OBJ with:
// - entire map geometry (mapModel.triangles)
// - shooter model
// - target model
// - optionally no cone, or a smaller cone
func CreateFullDebugOBJ(
	tick int,
	mapModel *Model,
	playerModel *Model,
	shooter collector.PlayerTickData,
	target collector.PlayerTickData,
	includeCone bool,
) error {
	// 1) Make debug_visibility folder if needed
	outputDir := "debug_visibility"
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return err
	}

	// 2) Write materials
	if err := writeMTLFile(outputDir); err != nil {
		return fmt.Errorf("failed to create materials file: %w", err)
	}

	// 3) Create the .obj
	fileName := filepath.Join(outputDir, fmt.Sprintf("debug_tick_%d.obj", tick))
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	// Some housekeeping
	vertexIndex := 1
	fmt.Fprintf(writer, "mtllib materials.mtl\n\n")

	transform := func(v r3.Vector) r3.Vector {
		// If you want Blender-friendly orientation:
		return convertToBlender(v)
		// If you want no transform at all, just return v.
	}

	// Write the shooter model
	fmt.Fprintf(writer, "o shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transform(tri.V1.Add(shooter.Position))
		v2 := transform(tri.V2.Add(shooter.Position))
		v3 := transform(tri.V3.Add(shooter.Position))

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Write the target model
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transform(tri.V1.Add(target.Position))
		v2 := transform(tri.V2.Add(target.Position))
		v3 := transform(tri.V3.Add(target.Position))

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Write the *entire map* geometry
	fmt.Fprintf(writer, "\no entire_map_geometry\n")
	fmt.Fprintf(writer, "usemtl map_material\n")
	for _, tri := range mapModel.triangles {
		v1 := transform(tri.V1)
		v2 := transform(tri.V2)
		v3 := transform(tri.V3)

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// (Optional) If you still want a cone:
	if includeCone {
		fmt.Fprintf(writer, "\no debug_ray\n")
		transformedEye := transform(shooter.Position) // or some eye offset if you like
		// e.g. 100 units forward from the player's forward vector
		forwardEnd := shooter.Position.Add(shooter.ForwardVector().Mul(100))
		transformedEnd := transform(forwardEnd)

		baseRadius := 2.0
		WriteCone(f, transformedEye, transformedEnd, baseRadius, "ray", &vertexIndex)
	}

	return nil
}

// isPointInShooterFOV checks if 'point' is within `fovDegrees/2` of shooter's view direction
func isPointInShooterFOV(shooter collector.PlayerTickData, point r3.Vector, fovDegrees float64) bool {
	toTarget := point.Sub(shooter.Position).Normalize()
	forward := shooter.ForwardVector()
	dot := toTarget.Dot(forward) // = cos(angle)
	angle := math.Acos(dot) * (180 / math.Pi)
	return angle <= fovDegrees/2
}

// CreateShooterFOVOBJ writes a debug OBJ that includes:
// - The shooter model
// - The target model
// - All map triangles for which at least 1 vertex is in the shooter's FOV
func CreateShooterFOVOBJ(
	tick int,
	mapModel *Model,
	playerModel *Model,
	shooter collector.PlayerTickData,
	target collector.PlayerTickData,
	fovDegrees float64, // e.g. 90 or 120
	includeCone bool,
) error {
	outputDir := "debug_visibility"
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return err
	}

	if err := writeMTLFile(outputDir); err != nil {
		return fmt.Errorf("failed to write materials: %w", err)
	}

	fileName := filepath.Join(outputDir, fmt.Sprintf("shooter_fov_tick_%d.obj", tick))
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	vertexIndex := 1
	fmt.Fprintf(writer, "mtllib materials.mtl\n\n")

	// Axis transform function
	transform := func(v r3.Vector) r3.Vector {
		return convertToBlender(v)
	}

	// 1) Write shooter model
	fmt.Fprintf(writer, "o shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.triangles {
		// Position the model around the shooter's location
		v1 := transform(tri.V1.Add(shooter.Position))
		v2 := transform(tri.V2.Add(shooter.Position))
		v3 := transform(tri.V3.Add(shooter.Position))
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 2) Write target model (optional: only if it's in FOV or always)
	// Here we always include the target, but you can skip if the entire target is out of FOV.
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transform(tri.V1.Add(target.Position))
		v2 := transform(tri.V2.Add(target.Position))
		v3 := transform(tri.V3.Add(target.Position))
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 3) Write partial map geometry: only the triangles in the shooter's FOV
	fmt.Fprintf(writer, "\no map_geometry_in_fov\n")
	fmt.Fprintf(writer, "usemtl map_material\n")

	for _, tri := range mapModel.triangles {
		// We'll check the 3 vertices. If ANY vertex is in FOV, keep the entire triangle.
		vA := tri.V1
		vB := tri.V2
		vC := tri.V3

		inFOVA := isPointInShooterFOV(shooter, vA, fovDegrees)
		inFOVB := isPointInShooterFOV(shooter, vB, fovDegrees)
		inFOVC := isPointInShooterFOV(shooter, vC, fovDegrees)

		if inFOVA || inFOVB || inFOVC {
			// Write it out
			outA := transform(vA)
			outB := transform(vB)
			outC := transform(vC)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outA.X, outA.Y, outA.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outB.X, outB.Y, outB.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outC.X, outC.Y, outC.Z)
			fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
			vertexIndex += 3
		}
	}

	// 4) Optionally draw a small cone or ray from the shooter
	if includeCone {
		fmt.Fprintf(writer, "\no debug_ray\n")
		eyePos := shooter.Position
		forwardEnd := eyePos.Add(shooter.ForwardVector().Mul(200))
		apex := transform(eyePos)
		base := transform(forwardEnd)
		baseRadius := 5.0
		WriteCone(f, apex, base, baseRadius, "ray", &vertexIndex)
	}

	return nil
}

// distanceToShooter returns the distance from the shooter to the given point
func distanceToShooter(shooter collector.PlayerTickData, point r3.Vector) float64 {
	dx := point.X - shooter.Position.X
	dy := point.Y - shooter.Position.Y
	dz := point.Z - shooter.Position.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func CreateShooterCentricFOVUsingTargetDistance(
	tick int,
	mapModel *Model,
	playerModel *Model,
	shooter collector.PlayerTickData,
	target collector.PlayerTickData,
	fovDegrees float64, // e.g. 90
	extraPadding float64, // e.g. 500
	includeCone bool,
) error {
	outputDir := "debug_visibility"
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return err
	}
	if err := writeMTLFile(outputDir); err != nil {
		return fmt.Errorf("failed to write materials: %w", err)
	}

	// Calculate how far away the target is, then add some padding
	distToTarget := distanceToShooter(shooter, target.Position)
	maxDistance := distToTarget + extraPadding

	// We’ll call the file "fov_tick_<tick>.obj"
	fileName := filepath.Join(outputDir, fmt.Sprintf("fov_tick_%d.obj", tick))
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	// Start writing the OBJ
	vertexIndex := 1
	fmt.Fprintf(writer, "mtllib materials.mtl\n\n")

	// The "center" transform: subtract shooter’s pos, then flip axes
	shooterPos := shooter.Position
	transform := func(v r3.Vector) r3.Vector {
		offset := r3.Vector{
			X: v.X - shooterPos.X,
			Y: v.Y - shooterPos.Y,
			Z: v.Z - shooterPos.Z,
		}
		return convertToBlender(offset)
	}

	// 1) Write the shooter model at origin
	fmt.Fprintf(writer, "o shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transform(tri.V1.Add(shooter.Position))
		v2 := transform(tri.V2.Add(shooter.Position))
		v3 := transform(tri.V3.Add(shooter.Position))

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 2) Write the target model (optional if you only want it if in FOV)
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transform(tri.V1.Add(target.Position))
		v2 := transform(tri.V2.Add(target.Position))
		v3 := transform(tri.V3.Add(target.Position))
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 3) Partial map: cull by distance to shooter *and* FOV
	fmt.Fprintf(writer, "\no partial_map\n")
	fmt.Fprintf(writer, "usemtl map_material\n")

	for _, tri := range mapModel.triangles {
		vA := tri.V1
		vB := tri.V2
		vC := tri.V3

		distA := distanceToShooter(shooter, vA)
		distB := distanceToShooter(shooter, vB)
		distC := distanceToShooter(shooter, vC)

		fovA := isPointInShooterFOV(shooter, vA, fovDegrees)
		fovB := isPointInShooterFOV(shooter, vB, fovDegrees)
		fovC := isPointInShooterFOV(shooter, vC, fovDegrees)

		// Keep the triangle if ANY vertex is within maxDistance *and* in FOV
		keep := false
		if (distA <= maxDistance && fovA) ||
			(distB <= maxDistance && fovB) ||
			(distC <= maxDistance && fovC) {
			keep = true
		}

		if keep {
			outA := transform(vA)
			outB := transform(vB)
			outC := transform(vC)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outA.X, outA.Y, outA.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outB.X, outB.Y, outB.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outC.X, outC.Y, outC.Z)
			fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
			vertexIndex += 3
		}
	}

	// 4) (Optional) Debug cone from shooter's origin
	if includeCone {
		fmt.Fprintf(writer, "\no debug_ray\n")
		apex := r3.Vector{0, 0, 0} // shooter is at origin
		forwardDir := shooter.ForwardVector()
		base := r3.Vector{
			X: forwardDir.X * (distToTarget + extraPadding),
			Y: forwardDir.Y * (distToTarget + extraPadding),
			Z: forwardDir.Z * (distToTarget + extraPadding),
		}
		apexBlender := convertToBlender(apex)
		baseBlender := convertToBlender(base)

		baseRadius := 10.0
		WriteCone(f, apexBlender, baseBlender, baseRadius, "ray", &vertexIndex)
	}

	return nil
}
