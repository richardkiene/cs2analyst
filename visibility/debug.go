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

// Draws a ray from start to end, with optional hit point visualization
func WriteRay(w io.Writer, start, end r3.Vector, hasHit bool, material string, vertexIndex *int) {
	fmt.Fprintf(w, "\no debug_ray\n")
	fmt.Fprintf(w, "usemtl %s\n", material)

	// Write ray vertices
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", start.X, start.Y, start.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", end.X, end.Y, end.Z)

	// Create ray line
	fmt.Fprintf(w, "l %d %d\n", *vertexIndex, *vertexIndex+1)
	*vertexIndex += 2

	// If there's a hit, draw a small sphere at the intersection point
	if hasHit {
		WriteSphere(w, end, 2.0, 8, "hit_material", vertexIndex)
	}
}

func WriteFOVCone(w io.Writer, eyePos r3.Vector, forward r3.Vector, material string, vertexIndex *int) {
	slog.Info("WriteFOVCone inputs",
		"eyePos", eyePos,
		"forward", forward,
		"forward length", forward.Norm())

	const (
		HORIZONTAL_FOV = 90.0  // CS2's actual horizontal FOV
		VERTICAL_FOV   = 74.0  // CS2's actual vertical FOV
		CONE_LENGTH    = 200.0 // Length of visualization cone
	)

	fmt.Fprintf(w, "\no fov_cone\n")
	fmt.Fprintf(w, "usemtl %s\n", material)

	// Convert FOV angles to radians
	hFovRad := (HORIZONTAL_FOV / 2.0) * (math.Pi / 180.0)
	vFovRad := (VERTICAL_FOV / 2.0) * (math.Pi / 180.0)

	// Calculate base dimensions
	baseWidth := CONE_LENGTH * math.Tan(hFovRad)
	baseHeight := CONE_LENGTH * math.Tan(vFovRad)

	// In Blender space, Z is up
	var up r3.Vector
	if math.Abs(forward.Z) > 0.99 {
		// If looking straight up/down, use forward-perpendicular up
		if forward.Z > 0 {
			up = r3.Vector{X: 0, Y: 1, Z: 0}
		} else {
			up = r3.Vector{X: 0, Y: -1, Z: 0}
		}
	} else {
		up = r3.Vector{X: 0, Y: 0, Z: 1}
	}

	// Calculate right and true up vectors
	right := forward.Cross(up).Normalize()
	up = right.Cross(forward).Normalize()

	// Calculate cone end points
	endPoint := eyePos.Add(forward.Mul(CONE_LENGTH))

	// Calculate corner points of FOV frustum
	topRight := endPoint.Add(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	topLeft := endPoint.Sub(right.Mul(baseWidth)).Add(up.Mul(baseHeight))
	bottomRight := endPoint.Add(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))
	bottomLeft := endPoint.Sub(right.Mul(baseWidth)).Sub(up.Mul(baseHeight))

	slog.Info("FOV Cone points",
		"apex", eyePos,
		"topRight", topRight,
		"topLeft", topLeft,
		"bottomRight", bottomRight,
		"bottomLeft", bottomLeft)

	// Write vertices
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", eyePos.X, eyePos.Y, eyePos.Z) // Apex
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", topRight.X, topRight.Y, topRight.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", topLeft.X, topLeft.Y, topLeft.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", bottomRight.X, bottomRight.Y, bottomRight.Z)
	fmt.Fprintf(w, "v %.6f %.6f %.6f\n", bottomLeft.X, bottomLeft.Y, bottomLeft.Z)

	// Create lines for FOV visualization (unchanged)
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
	extraPadding float64,
	includeCone bool,
	hitPoints []r3.Vector,
	rayIntersections []r3.Vector, // New parameter for ray intersection points
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

	// transformPosition converts from Source2 to Blender coordinate system
	transformPosition := func(v r3.Vector) r3.Vector {
		return r3.Vector{
			X: v.Y,  // Source2 Y (north) becomes Blender X (forward)
			Y: -v.X, // Source2 X (east) becomes Blender -Y (left)
			Z: v.Z,  // Z stays the same
		}.Sub(shooterPos) // translate to shooter-centric space
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
	slog.Debug("Starting map geometry processing",
		"totalTriangles", len(mapModel.triangles),
		"maxDistance", maxDistance)

	eyePos := GetEyePosition(shooter, playerModel)

	fmt.Fprintf(writer, "\no partial_map\n")
	fmt.Fprintf(writer, "usemtl map_material\n")

	// Create a temporary PlayerTickData with rotated view angles for FOV checks
	fovChecker := shooter
	// Add 90 degrees to rotate the view direction from Source2 to Blender space
	fovChecker.ViewAngleX = shooter.ViewAngleX + 90

	for _, tri := range mapModel.triangles {
		vA := tri.V1
		vB := tri.V2
		vC := tri.V3

		distA := distanceToShooter(shooter, vA)
		distB := distanceToShooter(shooter, vB)
		distC := distanceToShooter(shooter, vC)

		// Use the rotated view angles for FOV checks
		inFOVA := fovChecker.IsInFieldOfViewFromEye(vA, eyePos)
		inFOVB := fovChecker.IsInFieldOfViewFromEye(vB, eyePos)
		inFOVC := fovChecker.IsInFieldOfViewFromEye(vC, eyePos)

		keep := false
		if (distA <= maxDistance && inFOVA) ||
			(distB <= maxDistance && inFOVB) ||
			(distC <= maxDistance && inFOVC) {
			keep = true
		}

		/*slog.Debug("Triangle FOV check",
		"distA", distA,
		"distB", distB,
		"distC", distC,
		"inFOVA", inFOVA,
		"inFOVB", inFOVB,
		"inFOVC", inFOVC,
		"maxDistance", maxDistance)*/

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

	slog.Debug("Finished map geometry processing",
		"trianglesKept", vertexIndex/3)

	// 5) optional cone from shooter local origin (0,0,0)
	// Before writing the cone, we need to transform the forward vector just like we
	// transformed the coordinates
	if includeCone {
		eyePos := transformPosition(GetEyePosition(shooter, playerModel))
		// Get a point 100 units in front of player in Source2 space
		sourceForward := shooter.ForwardVector()
		//sourceForwardPoint := GetEyePosition(shooter, playerModel).Add(sourceForward.Mul(100))

		// Transform both points and calculate new forward direction
		//transformedForwardPoint := transformPosition(sourceForwardPoint)

		// Debug prints
		slog.Info("FOV Cone vectors",
			"sourceEyePos", GetEyePosition(shooter, playerModel),
			"transformedEyePos", eyePos,
			"sourceForward", sourceForward)
		//"sourceForwardPoint", sourceForwardPoint,
		//"transformedForwardPoint", transformedForwardPoint)

		//blenderForward := transformedForwardPoint.Sub(eyePos).Normalize()
		blenderForward := r3.Vector{
			X: -sourceForward.Y, // Source2 Y (north) becomes Blender -X to match transformPosition's convention
			Y: sourceForward.X,  // Source2 X (east) becomes Blender Y
			Z: sourceForward.Z,  // Z stays the same
		}.Normalize()
		slog.Info("Calculated blender forward", "blenderForward", blenderForward)

		WriteFOVCone(writer, eyePos, blenderForward, "cone_material", &vertexIndex)
	}

	// Visualize rays and their intersections
	points := playerModel.GetVisibilityPoints()

	for _, bp := range points {
		endPos := getCandidateWorldPoint(target, bp)
		start := transformPosition(GetEyePosition(shooter, playerModel))
		end := transformPosition(endPos)

		// Draw ray from eye to candidate point
		WriteRay(writer, start, end, false, "ray_material", &vertexIndex)
	}

	// Visualize ray intersections
	for _, intersectionPoint := range rayIntersections {
		localPoint := transformPosition(intersectionPoint)
		WriteSphere(writer, localPoint, 3.0, 8, "intersection_material", &vertexIndex)
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
