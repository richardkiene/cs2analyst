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

// We do no coordinate transform. We just add the shooter’s position for local geometry.
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

func WriteRay(w io.Writer, start, end r3.Vector, hasHit bool, material string, vertexIndex *int) {
	fmt.Fprintf(w, "\no debug_ray\n")
	fmt.Fprintf(w, "usemtl %s\n", material)
	fmt.Fprintf(w, "v %.6f %.6f %.6f # ray start\n", start.X, start.Y, start.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f # ray end\n", end.X, end.Y, end.Z)
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex, *vertexIndex+1)
	fmt.Printf("Wrote ray: start=%+v end=%+v indices=%d,%d\n", start, end, *vertexIndex, *vertexIndex+1)
	*vertexIndex += 2
	if hasHit {
		WriteSphere(w, end, 2.0, 8, "hit_material", vertexIndex)
	}
}

func WriteFOVCone(
	w io.Writer,
	eyePos, forward r3.Vector,
	material string,
	vertexIndex *int,
) {
	const (
		HORIZONTAL_FOV = 90.0
		VERTICAL_FOV   = 74.0
		CONE_LENGTH    = 200.0
	)
	hFovRad := (HORIZONTAL_FOV / 2.0) * (math.Pi / 180.0)
	vFovRad := (VERTICAL_FOV / 2.0) * (math.Pi / 180.0)
	baseWidth := CONE_LENGTH * math.Tan(hFovRad)
	baseHeight := CONE_LENGTH * math.Tan(vFovRad)

	// In Source2/CS2:
	// Forward is the view direction
	// Up is world up (0,0,1) unless nearly vertical
	// Right is perpendicular to both
	globalUp := r3.Vector{X: 0, Y: 0, Z: 1}

	// Handle near-vertical views specially
	if math.Abs(forward.Z) > 0.99 {
		// Looking nearly straight up/down - use a fixed right vector
		globalUp = r3.Vector{X: 0, Y: 1, Z: 0}
	}

	right := forward.Cross(globalUp).Normalize()
	up := right.Cross(forward).Normalize()

	fmt.Printf("FOV Cone basis - forward=%+v right=%+v up=%+v\n", forward, right, up)

	endPoint := eyePos.Add(forward.Mul(CONE_LENGTH))
	topRight := endPoint.Add(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	topLeft := endPoint.Sub(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	bottomRight := endPoint.Add(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))
	bottomLeft := endPoint.Sub(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))

	// Write the vertices and lines
	fmt.Fprintf(w, "\no fov_cone\n")
	fmt.Fprintf(w, "usemtl %s\n", material)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", eyePos.X, eyePos.Y, eyePos.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", topRight.X, topRight.Y, topRight.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", topLeft.X, topLeft.Y, topLeft.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", bottomRight.X, bottomRight.Y, bottomRight.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", bottomLeft.X, bottomLeft.Y, bottomLeft.Z)

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
	up := r3.Vector{X: 0, Y: 0, Z: 1}
	var u r3.Vector
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

// Forward vec is the shooter's view direction
// Right vec is perpendicular to up and forward
// Up vec is world up (0,0,1) in Source2
func getViewBasis(shooter types.PlayerTickData) (forward, right, up r3.Vector) {
	forward = shooter.ForwardVector()
	up = r3.Vector{X: 0, Y: 0, Z: 1} // World up in Source2
	right = forward.Cross(up).Normalize()
	up = right.Cross(forward).Normalize() // Recompute up to ensure orthogonality
	return forward, right, up
}

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
	distToTarget := distanceToShooter(shooter, target.Position)
	maxDistance := distToTarget + extraPadding

	outDir := "debug_visibility"
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return err
	}
	if err := writeMTLFile(outDir); err != nil {
		return err
	}

	fileName := fmt.Sprintf("shooter_centric_fov_tick_%d.obj", tick)
	fpath := filepath.Join(outDir, fileName)
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
	eyePos := GetEyePosition(shooter, playerModel)

	// Use vector to target as our forward direction instead of ViewAngle calculation
	toTarget := target.Position.Sub(shooter.Position).Normalize()
	worldEye := eyePos
	rayEnd := worldEye.Add(toTarget.Mul(200.0))

	fmt.Printf("Writing ray: worldEye=%+v rayEnd=%+v toTarget=%+v\n", worldEye, rayEnd, toTarget)
	WriteRay(writer, worldEye, rayEnd, false, "ray_material", &vertexIndex)

	// Write shooter geometry
	fmt.Fprintf(writer, "\no shooter\n")
	fmt.Fprintf(writer, "usemtl shooter_material\n")
	for _, tri := range playerModel.triangles {
		v1 := tri.V1.Add(shooter.Position)
		v2 := tri.V2.Add(shooter.Position)
		v3 := tri.V3.Add(shooter.Position)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Write target geometry
	fmt.Fprintf(writer, "\no target\n")
	fmt.Fprintf(writer, "usemtl target_material\n")
	for _, tri := range playerModel.triangles {
		v1 := tri.V1.Add(target.Position)
		v2 := tri.V2.Add(target.Position)
		v3 := tri.V3.Add(target.Position)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v1.X, v1.Y, v1.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v2.X, v2.Y, v2.Z)
		fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", v3.X, v3.Y, v3.Z)
		fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
		vertexIndex += 3
	}

	// Write relevant map geometry using dot product to filter
	fmt.Fprintf(writer, "\no partial_map\n")
	fmt.Fprintf(writer, "usemtl map_material\n")
	for _, tri := range mapModel.triangles {
		centroid := tri.V1.Add(tri.V2).Add(tri.V3).Mul(1.0 / 3.0)
		vectorToTri := centroid.Sub(eyePos).Normalize()

		// Use dot product to check if triangle is in front of player
		dot := toTarget.Dot(vectorToTri)
		distanceToTri := centroid.Sub(eyePos).Norm()

		// Keep triangles in front of player (positive dot) and within max distance
		if dot > 0 && distanceToTri <= maxDistance {
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", tri.V1.X, tri.V1.Y, tri.V1.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", tri.V2.X, tri.V2.Y, tri.V2.Z)
			fmt.Fprintf(writer, "v %.6f %.6f %.6f\n", tri.V3.X, tri.V3.Y, tri.V3.Z)
			fmt.Fprintf(writer, "f %d %d %d\n", vertexIndex, vertexIndex+1, vertexIndex+2)
			vertexIndex += 3
		}
	}

	// Write FOV cone using vector to target as forward direction
	if includeCone {
		WriteFOVCone(writer, eyePos, toTarget, "cone_material", &vertexIndex)
	}

	return nil
}

// CreateDebugOBJ writes a simpler OBJ with shooter, target, and optionally map geometry and a cone.
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

	// Shooter geometry (unconverted, for reference).
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

	// Target geometry.
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

	// Optional map geometry.
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

	// Hit points.
	for _, hitPoint := range hitPoints {
		WriteSphere(f, hitPoint, 5.0, 8, "hit_material", &vertexIndex)
	}

	return nil
}
