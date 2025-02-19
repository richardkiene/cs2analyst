package visibility

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// writeMTLFile writes a basic material file for OBJ output.
func writeMTLFile(outputDir string) error {
	mtlContent := `# Basic materials for debug OBJ

newmtl shooter_material
Ka 0.2 0.2 0.2
Kd 0.8 0.2 0.2
Ks 1.0 1.0 1.0
d 1.0

newmtl target_material
Ka 0.2 0.2 0.2
Kd 0.2 0.8 0.2
Ks 1.0 1.0 1.0
d 1.0

newmtl map_material
Ka 0.2 0.2 0.2
Kd 0.5 0.5 0.5
Ks 0.1 0.1 0.1
d 1.0

newmtl cone_material
Ka 0.2 0.2 0.2
Kd 1.0 1.0 0.0
Ks 1.0 1.0 1.0
d 0.3
illum 2

newmtl hit_material
Ka 0.2 0.2 0.2
Kd 1.0 0.0 0.0
Ks 1.0 1.0 1.0
d 1.0
`
	return os.WriteFile(filepath.Join(outputDir, "debug_materials.mtl"), []byte(mtlContent), 0644)
}

// WriteRay draws a ray from start to end, optionally drawing a sphere at the hit point.
func WriteRay(w io.Writer, start, end r3.Vector, hasHit bool, material string, vertexIndex *int) {
	fmt.Fprintf(w, "\no debug_ray\n")
	fmt.Fprintf(w, "usemtl %s\n", material)
	// Write ray vertices (already in Blender space)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", start.X, start.Y, start.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", end.X, end.Y, end.Z)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex, *vertexIndex+1)
	*vertexIndex += 2
	if hasHit {
		WriteSphere(w, end, 2.0, 8, "hit_material", vertexIndex)
	}
}

// WriteFOVCone computes the FOV cone entirely in Source2 space and converts
// each vertex to Blender coordinates right before writing it out.
// shooterPos is used for shooter-centric conversion.
func WriteFOVCone(w io.Writer, eyePos, forward r3.Vector, material string, vertexIndex *int, shooterPos r3.Vector) {
	const (
		HORIZONTAL_FOV = 90.0  // Source2 horizontal FOV
		VERTICAL_FOV   = 74.0  // Source2 vertical FOV
		CONE_LENGTH    = 200.0 // Cone length in Source2 units
	)
	hFovRad := (HORIZONTAL_FOV / 2.0) * (math.Pi / 180.0)
	vFovRad := (VERTICAL_FOV / 2.0) * (math.Pi / 180.0)
	baseWidth := CONE_LENGTH * math.Tan(hFovRad)
	baseHeight := CONE_LENGTH * math.Tan(vFovRad)

	// Compute up and right vectors in Source2 space.
	var up r3.Vector
	if math.Abs(forward.Z) > 0.99 {
		if forward.Z > 0 {
			up = r3.Vector{X: 0, Y: 1, Z: 0}
		} else {
			up = r3.Vector{X: 0, Y: -1, Z: 0}
		}
	} else {
		up = r3.Vector{X: 0, Y: 0, Z: 1}
	}
	right := forward.Cross(up).Normalize()
	up = right.Cross(forward).Normalize()

	// Compute the end point and base corners in Source2 space.
	endPoint := eyePos.Add(forward.Mul(CONE_LENGTH))
	topRight := endPoint.Add(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	topLeft := endPoint.Sub(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	bottomRight := endPoint.Add(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))
	bottomLeft := endPoint.Sub(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))

	// Conversion helper: convert from Source2 world space to shooter-centric Blender space.
	// This subtracts shooterPos (making geometry shooter‑centric) and then converts coordinates.
	convert := func(v r3.Vector) r3.Vector {
		local := v.Sub(shooterPos)
		// In Source2: X = forward (East), Y = left (North), Z = up.
		// In Blender we want: Forward = +X, Up = +Z.
		// One common conversion is to swap the X and Y components and flip the sign of one axis.
		return r3.Vector{
			X: local.Y,  // Source2 Y becomes Blender X.
			Y: -local.X, // Source2 X becomes Blender -Y.
			Z: local.Z,  // Z remains the same.
		}
	}

	fmt.Fprintf(w, "\no fov_cone\n")
	fmt.Fprintf(w, "usemtl %s\n", material)
	// Write vertices after conversion.
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", convert(eyePos).X, convert(eyePos).Y, convert(eyePos).Z) // Apex
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", convert(topRight).X, convert(topRight).Y, convert(topRight).Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", convert(topLeft).X, convert(topLeft).Y, convert(topLeft).Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", convert(bottomRight).X, convert(bottomRight).Y, convert(bottomRight).Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", convert(bottomLeft).X, convert(bottomLeft).Y, convert(bottomLeft).Z)

	// Create lines for the cone visualization.
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex, *vertexIndex+1)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex, *vertexIndex+2)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex, *vertexIndex+3)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex, *vertexIndex+4)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex+1, *vertexIndex+2)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex+3, *vertexIndex+4)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex+1, *vertexIndex+3)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex+2, *vertexIndex+4)
	*vertexIndex += 5
}

// WriteSphere writes a simple sphere at the given center.
func WriteSphere(w io.Writer, center r3.Vector, radius float64, segments int, material string, vertexIndex *int) {
	fmt.Fprintf(w, "\no hit_point\n")
	fmt.Fprintf(w, "usemtl %s\n", material)
	vertices := make([]r3.Vector, 0)
	for phi := 0.0; phi <= math.Pi; phi += math.Pi / float64(segments) {
		for theta := 0.0; theta < 2*math.Pi; theta += 2 * math.Pi / float64(segments) {
			x := radius * math.Sin(phi) * math.Cos(theta)
			y := radius * math.Sin(phi) * math.Sin(theta)
			z := radius * math.Cos(phi)
			vertex := r3.Vector{
				X: center.X + x,
				Y: center.Y + y,
				Z: center.Z + z,
			}
			vertices = append(vertices, vertex)
			fmt.Fprintf(w, "v %.6f %.6f %.6f\n", vertex.X, vertex.Y, vertex.Z)
		}
	}
	topVertex := r3.Vector{X: center.X, Y: center.Y, Z: center.Z + radius}
	bottomVertex := r3.Vector{X: center.X, Y: center.Y, Z: center.Z - radius}
	vertices = append(vertices, topVertex, bottomVertex)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", topVertex.X, topVertex.Y, topVertex.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", bottomVertex.X, bottomVertex.Y, bottomVertex.Z)
	for i := 0; i < segments; i++ {
		for j := 0; j < segments; j++ {
			v1 := i*segments + j + *vertexIndex
			v2 := i*segments + ((j + 1) % segments) + *vertexIndex
			v3 := ((i+1)%segments)*segments + j + *vertexIndex
			v4 := ((i+1)%segments)*segments + ((j + 1) % segments) + *vertexIndex
			fmt.Fprintf(w, "f %d %d %d\n", v1, v2, v3)
			fmt.Fprintf(w, "f %d %d %d\n", v2, v4, v3)
		}
	}
	*vertexIndex += len(vertices)
}

// WriteCone draws a cone between an apex and a base.
func WriteCone(
	w io.Writer,
	apex, base r3.Vector,
	baseRadius float64,
	material string,
	vertexIndex *int,
) {
	segments := 16
	fmt.Fprintf(w, "\no debug_cone\n")
	fmt.Fprintf(w, "usemtl %s\n", material)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", apex.X, apex.Y, apex.Z)
	apexIdx := *vertexIndex
	*vertexIndex++
	dir := base.Sub(apex).Normalize()
	var u r3.Vector
	up := r3.Vector{X: 0, Y: 0, Z: 1}
	if math.Abs(dir.Z) > 0.99 {
		u = r3.Vector{X: 1, Y: 0, Z: 0}
	} else {
		u = dir.Cross(up).Normalize()
	}
	v := dir.Cross(u).Normalize()
	baseVerts := make([]int, segments)
	for i := 0; i < segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		cosA := math.Cos(angle) * baseRadius
		sinA := math.Sin(angle) * baseRadius
		pt := base.Add(u.Mul(cosA)).Add(v.Mul(sinA))
		fmt.Fprintf(w, "v %.6f %.6f %.6f\n", pt.X, pt.Y, pt.Z)
		baseVerts[i] = *vertexIndex
		*vertexIndex++
	}
	for i := 0; i < segments; i++ {
		next := (i + 1) % segments
		fmt.Fprintf(w, "f %d %d %d\n", apexIdx, baseVerts[i], baseVerts[next])
	}
	fmt.Fprintf(w, "f")
	for i := segments - 1; i >= 0; i-- {
		fmt.Fprintf(w, " %d", baseVerts[i])
	}
	fmt.Fprintf(w, "\n")
}

// CreateShooterCentricFOVUsingTargetDistance builds the OBJ using all geometry in Source2 space.
// Conversion to Blender coordinates happens only at output time.
func CreateShooterCentricFOVUsingTargetDistance(
	tick int,
	mapModel *Model,
	playerModel *Model,
	shooter types.PlayerTickData,
	target types.PlayerTickData,
	extraPadding float64,
	includeCone bool,
	hitPoints []r3.Vector,
	rayIntersections []r3.Vector,
) error {
	// Compute distance to target and define maximum distance.
	distToTarget := distanceToShooter(shooter, target.Position)
	maxDistance := distToTarget + extraPadding

	outputDir := "debug_visibility"
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return err
	}
	if err := writeMTLFile(outputDir); err != nil {
		return err
	}
	fileName := fmt.Sprintf("shooter_centric_fov_tick_%d.obj", tick)
	fpath := filepath.Join(outputDir, fileName)
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	fmt.Fprintf(writer, "mtllib debug_materials.mtl\n\n")
	fmt.Fprintf(writer, "# Shooter-centric partial FOV debug\n")

	vertexIndex := 1
	shooterPos := shooter.Position

	// Define a composite conversion function.
	// It takes a Source2 world-space vector, subtracts shooterPos to get shooter-centric coordinates,
	// then converts to Blender space.
	convert := func(v r3.Vector) r3.Vector {
		local := v.Sub(shooterPos)
		return r3.Vector{
			X: local.Y,  // Source2 Y becomes Blender X.
			Y: -local.X, // Source2 X becomes Blender -Y.
			Z: local.Z,  // Z stays the same.
		}
	}

	// Write shooter geometry using player model triangles.
	fmt.Fprintf(writer, "o shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.triangles {
		v1 := convert(tri.V1.Add(shooter.Position))
		v2 := convert(tri.V2.Add(shooter.Position))
		v3 := convert(tri.V3.Add(shooter.Position))
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Write target geometry.
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.triangles {
		v1 := convert(tri.V1.Add(target.Position))
		v2 := convert(tri.V2.Add(target.Position))
		v3 := convert(tri.V3.Add(target.Position))
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Process and write map geometry (FOV checks use Source2 data with no extra +90 offset).
	slog.Debug("Starting map geometry processing",
		"totalTriangles", len(mapModel.triangles),
		"maxDistance", maxDistance)

	eyePos := GetEyePosition(shooter, playerModel) // In Source2 space

	fmt.Fprintf(writer, "\no partial_map\n")
	fmt.Fprintf(writer, "usemtl map_material\n")

	// Use the shooter data directly for FOV checks.
	for _, tri := range mapModel.triangles {
		vA := tri.V1
		vB := tri.V2
		vC := tri.V3

		distA := distanceToShooter(shooter, vA)
		distB := distanceToShooter(shooter, vB)
		distC := distanceToShooter(shooter, vC)

		inFOVA := shooter.IsInFieldOfViewFromEye(vA, eyePos)
		inFOVB := shooter.IsInFieldOfViewFromEye(vB, eyePos)
		inFOVC := shooter.IsInFieldOfViewFromEye(vC, eyePos)

		keep := (distA <= maxDistance && inFOVA) ||
			(distB <= maxDistance && inFOVB) ||
			(distC <= maxDistance && inFOVC)

		if keep {
			outA := convert(vA)
			outB := convert(vB)
			outC := convert(vC)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outA.X, outA.Y, outA.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outB.X, outB.Y, outB.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outC.X, outC.Y, outC.Z)
			fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
			vertexIndex += 3
		}
	}

	slog.Debug("Finished map geometry processing",
		"trianglesKept", vertexIndex/3)

	// Write FOV cone using raw Source2 data.
	if includeCone {
		sourceEyePos := GetEyePosition(shooter, playerModel) // Source2 space
		sourceForward := shooter.ForwardVector().Normalize() // Source2 forward vector from raw angles
		// WriteFOVCone converts each vertex to Blender space internally.
		WriteFOVCone(writer, sourceEyePos, sourceForward, "cone_material", &vertexIndex, shooterPos)
	}

	// Visualize rays.
	points := playerModel.GetVisibilityPoints()
	for _, bp := range points {
		endPos := getCandidateWorldPoint(target, bp)
		start := convert(GetEyePosition(shooter, playerModel))
		end := convert(endPos)
		WriteRay(writer, start, end, false, "ray_material", &vertexIndex)
	}

	// Visualize ray intersections.
	for _, intersectionPoint := range rayIntersections {
		localPoint := convert(intersectionPoint)
		WriteSphere(writer, localPoint, 3.0, 8, "intersection_material", &vertexIndex)
	}

	// Visualize hit points.
	for _, hitPoint := range hitPoints {
		localHitPoint := convert(hitPoint)
		WriteSphere(writer, localHitPoint, 5.0, 8, "hit_material", &vertexIndex)
	}

	return nil
}

// CreateDebugOBJ writes an OBJ with shooter, target, optional map geometry, and a cone from eye->target.
// Here, shooter and target geometry are written in Source2 space and then converted to Blender space only at output.
func CreateDebugOBJ(
	tick int,
	mapModel *Model,
	playerModel *Model,
	shooter types.PlayerTickData,
	target types.PlayerTickData,
	eyePos, targetPos r3.Vector,
	includeMap bool,
	coneRadius float64,
	hitPoints []r3.Vector,
) error {
	outDir := "debug_visibility"
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return err
	}
	if err := writeMTLFile(outDir); err != nil {
		return err
	}
	fileName := filepath.Join(outDir, fmt.Sprintf("debug_tick_%d.obj", tick))
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	fmt.Fprintf(writer, "mtllib debug_materials.mtl\n\n")

	vertexIndex := 1

	// Write shooter geometry.
	fmt.Fprintf(writer, "o shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.TrianglesRaw() {
		v1 := tri.V1.Add(shooter.Position)
		v2 := tri.V2.Add(shooter.Position)
		v3 := tri.V3.Add(shooter.Position)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Write target geometry.
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.TrianglesRaw() {
		v1 := tri.V1.Add(target.Position)
		v2 := tri.V2.Add(target.Position)
		v3 := tri.V3.Add(target.Position)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Write map geometry.
	if includeMap && mapModel != nil {
		fmt.Fprintf(writer, "\no map_geometry\n")
		fmt.Fprintf(writer, "usemtl map_material\n")
		for _, tri := range mapModel.TrianglesRaw() {
			v1 := tri.V1
			v2 := tri.V2
			v3 := tri.V3
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
			fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
			vertexIndex += 3
		}
	}

	if coneRadius > 0.0 {
		WriteCone(f, eyePos, targetPos, coneRadius, "cone_material", &vertexIndex)
	}

	// Write hit points.
	for _, hitPoint := range hitPoints {
		WriteSphere(f, hitPoint, 5.0, 8, "hit_material", &vertexIndex)
	}

	return nil
}
