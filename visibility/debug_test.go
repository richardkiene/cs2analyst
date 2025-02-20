package visibility

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/stretchr/testify/assert"
)

// parseObjSections splits an OBJ file into sections keyed by object name (lines beginning with "o ").
func parseObjSections(obj string) map[string][]r3.Vector {
	sections := make(map[string][]r3.Vector)
	var currentSection string
	scanner := bufio.NewScanner(strings.NewReader(obj))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "o ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentSection = parts[1]
				sections[currentSection] = []r3.Vector{}
			}
		} else if strings.HasPrefix(line, "v ") && currentSection != "" {
			parts := strings.Fields(line)
			if len(parts) < 4 {
				continue
			}
			x, _ := strconv.ParseFloat(parts[1], 64)
			y, _ := strconv.ParseFloat(parts[2], 64)
			z, _ := strconv.ParseFloat(parts[3], 64)
			sections[currentSection] = append(sections[currentSection], r3.Vector{X: x, Y: y, Z: z})
		}
	}
	return sections
}

// computeDirectionForCone assumes that in the FOV cone section the first vertex is the apex
// and the next four vertices are the far-plane corners. It returns the normalized direction
// from the apex to the average of the far-plane vertices.
func computeDirectionForCone(verts []r3.Vector) r3.Vector {
	if len(verts) < 5 {
		return r3.Vector{}
	}
	apex := verts[0]
	var sum r3.Vector
	for i := 1; i < 5; i++ {
		sum = sum.Add(verts[i])
	}
	farCenter := sum.Mul(1.0 / 4.0)
	return farCenter.Sub(apex).Normalize()
}

// computeDirectionForRay expects exactly two vertices.
func computeDirectionForRay(verts []r3.Vector) r3.Vector {
	if len(verts) < 2 {
		return r3.Vector{}
	}
	return verts[1].Sub(verts[0]).Normalize()
}

// computeCentroidDirection returns the normalized vector from the origin to the centroid of the vertices.
func computeCentroidDirection(verts []r3.Vector) r3.Vector {
	if len(verts) == 0 {
		return r3.Vector{}
	}
	var sum r3.Vector
	for _, v := range verts {
		sum = sum.Add(v)
	}
	centroid := sum.Mul(1.0 / float64(len(verts)))
	return centroid.Normalize()
}

func TestObjFileDirectionConsistency(t *testing.T) {
	// Create a simple scenario with shooter at origin facing east
	shooter := types.PlayerTickData{
		Position:   r3.Vector{X: 0, Y: 0, Z: 0},
		ViewAngleX: 90, // Facing east
		ViewAngleY: 0,  // Level view
		IsAlive:    true,
	}

	target := types.PlayerTickData{
		Position:   r3.Vector{X: 100, Y: 0, Z: 0}, // Target 100 units east
		ViewAngleX: 0,
		ViewAngleY: 0,
		IsAlive:    true,
	}

	mapModel := CreateTestPlayerModel() // Simple model for testing
	playerModel := CreateTestPlayerModel()

	// Print the shooter's forward vector to verify direction
	forward := shooter.ForwardVector()
	t.Logf("Shooter forward vector: %+v", forward)

	// Get eye position
	eyePos := GetEyePosition(shooter, playerModel)
	t.Logf("Eye position: %+v", eyePos)

	// Print visibility points
	points := playerModel.GetVisibilityPoints()
	t.Logf("Number of visibility points: %d", len(points))
	for i, p := range points {
		t.Logf("Visibility point %d: %+v", i, p)
	}

	// Generate debug visualization
	err := CreateShooterCentricFOVUsingTargetDistance(
		84115,
		mapModel,
		playerModel,
		shooter,
		target,
		0.0,  // No extra padding
		true, // Include cone
		nil,  // No hit points
		nil,  // No ray intersections
	)
	assert.NoError(t, err, "Failed to create debug visualization")

	// Read and parse the generated OBJ file
	data, err := os.ReadFile("debug_visibility/shooter_centric_fov_tick_84115.obj")
	assert.NoError(t, err, "Failed to read OBJ file")

	sections := parseObjSections(string(data))

	// Verify each section exists
	coneVerts, ok := sections["fov_cone"]
	assert.True(t, ok, "OBJ must contain an 'fov_cone' section")
	rayVerts, ok := sections["debug_ray"]
	assert.True(t, ok, "OBJ must contain a 'debug_ray' section")
	mapVerts, ok := sections["partial_map"]
	assert.True(t, ok, "OBJ must contain a 'partial_map' section")

	// In shooter-centric space, east (world +X) becomes +X
	expected := r3.Vector{X: 1, Y: 0, Z: 0}

	if ok && len(coneVerts) >= 5 {
		coneDir := computeDirectionForCone(coneVerts)
		t.Logf("FOV Cone Direction: %+v", coneDir)
		assertVectorsEqual(t, expected, coneDir, 0.05, "FOV cone direction")
	}

	if ok && len(rayVerts) >= 2 {
		rayDir := computeDirectionForRay(rayVerts)
		t.Logf("Debug Ray Direction: %+v", rayDir)
		t.Logf("Ray start: %+v", rayVerts[0])
		t.Logf("Ray end: %+v", rayVerts[1])
		assertVectorsEqual(t, expected, rayDir, 0.05, "Debug ray direction")
	}

	if ok && len(mapVerts) > 0 {
		mapDir := computeCentroidDirection(mapVerts)
		t.Logf("Partial Map Direction: %+v", mapDir)
		assertVectorsEqual(t, expected, mapDir, 0.05, "Partial map direction")
	}
}
