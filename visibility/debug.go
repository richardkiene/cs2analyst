package visibility

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

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

// WriteSphere writes a simple sphere to the OBJ file at the given center point
func WriteSphere(w io.Writer, center r3.Vector, radius float64, segments int, material string, vertexIndex *int) {
	fmt.Fprintf(w, "\no hit_point\n")
	fmt.Fprintf(w, "usemtl %s\n", material)

	// Generate vertices
	vertices := make([]r3.Vector, 0)

	// Generate vertices for a UV sphere
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

	// Add top and bottom vertices
	topVertex := r3.Vector{X: center.X, Y: center.Y, Z: center.Z + radius}
	bottomVertex := r3.Vector{X: center.X, Y: center.Y, Z: center.Z - radius}
	vertices = append(vertices, topVertex, bottomVertex)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", topVertex.X, topVertex.Y, topVertex.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", bottomVertex.X, bottomVertex.Y, bottomVertex.Z)

	// Generate faces
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

// WriteCone draws a cone from `apex` to `base` with `baseRadius`, using a given material name.
// NOTE: Only define this function once!
func WriteCone(
	w io.Writer,
	apex, base r3.Vector,
	baseRadius float64,
	material string,
	vertexIndex *int,
) {
	segments := 16

	// Start a new object
	fmt.Fprintf(w, "\no debug_cone\n")
	fmt.Fprintf(w, "usemtl %s\n", material)

	// apex
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", apex.X, apex.Y, apex.Z)
	apexIdx := *vertexIndex
	*vertexIndex++

	dir := base.Sub(apex).Normalize()
	var u r3.Vector
	up := r3.Vector{0, 0, 1}
	if math.Abs(dir.Z) > 0.99 {
		u = r3.Vector{1, 0, 0}
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

	// side faces
	for i := 0; i < segments; i++ {
		next := (i + 1) % segments
		fmt.Fprintf(w, "f %d %d %d\n", apexIdx, baseVerts[i], baseVerts[next])
	}

	// base face
	fmt.Fprintf(w, "f")
	for i := segments - 1; i >= 0; i-- {
		fmt.Fprintf(w, " %d", baseVerts[i])
	}
	fmt.Fprintf(w, "\n")
}

// CreateShooterCentricFOVUsingTargetDistance is your partial-geometry debug function
// that culls by distance & FOV, then centers the geometry so shooter is at (0,0,0).
func CreateShooterCentricFOVUsingTargetDistance(
	tick int,
	mapModel *Model,
	playerModel *Model,
	shooter types.PlayerTickData,
	target types.PlayerTickData,
	fovDegrees float64,
	extraPadding float64,
	includeCone bool,
	hitPoints []r3.Vector,
) error {
	// 1) compute distance to target, define maxDistance
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

	// transformPosition subtracts the shooterPos (so he's at origin)
	transformPosition := func(v r3.Vector) r3.Vector {
		return v.Sub(shooterPos) // local space
	}

	// 2) Write shooter at origin
	fmt.Fprintf(writer, "o shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transformPosition(tri.V1.Add(shooter.Position))
		v2 := transformPosition(tri.V2.Add(shooter.Position))
		v3 := transformPosition(tri.V3.Add(shooter.Position))

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 3) Write target (also in shooter-centric space)
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.triangles {
		v1 := transformPosition(tri.V1.Add(target.Position))
		v2 := transformPosition(tri.V2.Add(target.Position))
		v3 := transformPosition(tri.V3.Add(target.Position))

		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// 4) cull map geometry by distance & FOV
	eyePos := GetEyePosition(shooter, playerModel)

	fmt.Fprintf(writer, "\no partial_map\n")
	fmt.Fprintf(writer, "usemtl map_material\n")

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

		keep := false
		if (distA <= maxDistance && inFOVA) ||
			(distB <= maxDistance && inFOVB) ||
			(distC <= maxDistance && inFOVC) {
			keep = true
		}

		if keep {
			outA := transformPosition(vA)
			outB := transformPosition(vB)
			outC := transformPosition(vC)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outA.X, outA.Y, outA.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outB.X, outB.Y, outB.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", outC.X, outC.Y, outC.Z)
			fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
			vertexIndex += 3
		}
	}

	// 5) optional cone from shooter local origin (0,0,0)
	if includeCone {
		fmt.Fprintf(writer, "\no debug_cone\n")
		fmt.Fprintf(writer, "usemtl cone_material\n")

		// Here, instead of using a hacky constant for eye height, you might want to use the computed eyePos.
		// In shooter-centric space, the shooter's eye becomes:
		localEyePos := transformPosition(eyePos)

		// Compute the cone length (distance from shooter to target plus padding).
		coneLength := distToTarget + extraPadding

		// Convert the passed-in fovDegrees to a half-angle in radians.
		halfAngleRadians := (fovDegrees / 2.0) * (math.Pi / 180.0)
		baseRadius := coneLength * math.Tan(halfAngleRadians)

		// Cone apex will be at the shooter’s eye position (localEyePos).
		// For the cone direction, we use the shooter’s forward vector.
		forwardDir := shooter.ForwardVector()
		base := localEyePos.Add(forwardDir.Mul(coneLength))

		WriteCone(writer, localEyePos, base, baseRadius, "cone_material", &vertexIndex)
	}

	// 6) Add hit points visualization
	for _, hitPoint := range hitPoints {
		// Transform hit point to shooter-centric space
		localHitPoint := transformPosition(hitPoint)
		WriteSphere(writer, localHitPoint, 5.0, 8, "hit_material", &vertexIndex)
	}

	return nil
}

// CreateDebugOBJ writes an OBJ with shooter, target, optional map geometry, and a cone from eye->target
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

	// Shooter
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

	// Target
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

	// optional map geometry
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

	// Add hit points visualization
	for _, hitPoint := range hitPoints {
		WriteSphere(f, hitPoint, 5.0, 8, "hit_material", &vertexIndex)
	}

	return nil
}
