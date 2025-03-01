package visibility

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

func LoadMapModel(path string) (*Model, error) {
	m, err := LoadOBJ("C:\\Users\\richa\\code\\CS2ResourceAPI\\GameDataService\\ModelOutput\\world_output.obj")
	if err != nil {
		return nil, fmt.Errorf("failed to load map model: %w", err)
	}
	return m, nil
}

func LoadPlayerModel(path string) (*Model, error) {
	p, err := LoadOBJ("C:\\Users\\richa\\code\\CS2ResourceAPI\\GameDataService\\ModelOutput\\ctm_sas_output.obj")
	if err != nil {
		return nil, fmt.Errorf("failed to load player model: %w", err)
	}
	return p, nil
}

// LoadOBJ reads an .obj file into a Model
func LoadOBJ(filename string) (*Model, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	model := NewModel()
	var vertices []r3.Vector

	// Hitbox parsing state
	var currentHitbox *Hitbox
	inHitboxSet := false
	hitboxVertices := make([]r3.Vector, 0)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "g":
			if len(fields) > 1 && strings.HasPrefix(fields[1], "hitboxset_") {
				inHitboxSet = true
			} else {
				inHitboxSet = false
			}

		case "#":
			if inHitboxSet && len(fields) > 2 && fields[1] == "Hitbox:" {
				// Process previous hitbox if exists
				if currentHitbox != nil && len(hitboxVertices) > 0 {
					currentHitbox.Vertices = hitboxVertices
					model.hitboxes = append(model.hitboxes, *currentHitbox)
				}

				// Parse hitbox info from comment
				hitboxInfo := strings.Join(fields[2:], " ")
				parts := strings.Split(hitboxInfo, "(")
				if len(parts) == 2 {
					name := strings.TrimSpace(parts[0])
					boneName := strings.Trim(parts[1], ")")
					currentHitbox = &Hitbox{
						Name:     name,
						BoneName: boneName,
						Type:     determineHitboxType(hitboxInfo),
					}
					hitboxVertices = make([]r3.Vector, 0)
				}
			}

		case "v":
			if len(fields) < 4 {
				continue
			}
			x, _ := strconv.ParseFloat(fields[1], 64)
			y, _ := strconv.ParseFloat(fields[2], 64)
			z, _ := strconv.ParseFloat(fields[3], 64)
			v := r3.Vector{X: x, Y: y, Z: z}

			if inHitboxSet && currentHitbox != nil {
				hitboxVertices = append(hitboxVertices, v)
				// Update hitbox bounds
				if len(hitboxVertices) == 1 {
					currentHitbox.MinBounds = v
					currentHitbox.MaxBounds = v
				} else {
					currentHitbox.MinBounds = minVector(currentHitbox.MinBounds, v)
					currentHitbox.MaxBounds = maxVector(currentHitbox.MaxBounds, v)
				}
			} else {
				vertices = append(vertices, v)
				model.min = minVector(model.min, v)
				model.max = maxVector(model.max, v)
			}

		case "f":
			if !inHitboxSet && len(fields) >= 4 {
				v1Idx, _ := strconv.Atoi(strings.Split(fields[1], "/")[0])
				v2Idx, _ := strconv.Atoi(strings.Split(fields[2], "/")[0])
				v3Idx, _ := strconv.Atoi(strings.Split(fields[3], "/")[0])

				tri := types.Triangle{
					V1: vertices[v1Idx-1],
					V2: vertices[v2Idx-1],
					V3: vertices[v3Idx-1],
				}
				model.triangles = append(model.triangles, tri)
				if model.gridSize > 0 {
					model.addTriangleToSectors(tri)
				}
			}
		}
	}

	// Add final hitbox if exists
	if currentHitbox != nil && len(hitboxVertices) > 0 {
		currentHitbox.Vertices = hitboxVertices
		model.hitboxes = append(model.hitboxes, *currentHitbox)
	}

	model.bvh = BuildBVH(model.triangles)

	return model, nil
}

func determineHitboxType(info string) string {
	lower := strings.ToLower(info)
	switch {
	case strings.Contains(lower, "sphere"):
		return "Sphere"
	case strings.Contains(lower, "capsule"):
		return "Capsule"
	default:
		return "Box"
	}
}

// addTriangleToSectors populates spatial partitioning for the model
func (m *Model) addTriangleToSectors(t types.Triangle) {
	minX := math.Min(t.V1.X, math.Min(t.V2.X, t.V3.X))
	maxX := math.Max(t.V1.X, math.Max(t.V2.X, t.V3.X))
	minY := math.Min(t.V1.Y, math.Min(t.V2.Y, t.V3.Y))
	maxY := math.Max(t.V1.Y, math.Max(t.V2.Y, t.V3.Y))
	minZ := math.Min(t.V1.Z, math.Min(t.V2.Z, t.V3.Z))
	maxZ := math.Max(t.V1.Z, math.Max(t.V2.Z, t.V3.Z))

	startX := int(minX / m.gridSize)
	endX := int(maxX / m.gridSize)
	startY := int(minY / m.gridSize)
	endY := int(maxY / m.gridSize)
	startZ := int(minZ / m.gridSize)
	endZ := int(maxZ / m.gridSize)

	for x := startX; x <= endX; x++ {
		for y := startY; y <= endY; y++ {
			for z := startZ; z <= endZ; z++ {
				key := GridKey{x, y, z}
				m.sectors[key] = append(m.sectors[key], t)
			}
		}
	}
}

// BuildBVH constructs a BVH from a slice of triangles.
func BuildBVH(triangles []types.Triangle) *BVHNode {
	if len(triangles) == 0 {
		return nil
	}
	return buildBVHRecursive(triangles)
}

func buildBVHRecursive(triangles []types.Triangle) *BVHNode {
	node := &BVHNode{}
	// Compute bounding box for all triangles.
	bbox := NewAABBFromTriangle(triangles[0])
	for _, tri := range triangles[1:] {
		triBBox := NewAABBFromTriangle(tri)
		bbox = unionAABB(bbox, triBBox)
	}
	node.bbox = bbox

	// If few triangles remain, make a leaf.
	if len(triangles) <= maxTrianglesPerLeaf {
		node.triangles = triangles
		return node
	}

	// Choose the axis with the largest extent.
	extents := bbox.Max.Sub(bbox.Min)
	axis := 0
	if extents.Y > extents.X {
		axis = 1
	}
	if extents.Z > extents.X && extents.Z > extents.Y {
		axis = 2
	}

	// Create a slice of (triangle, centroid) pairs.
	type triCentroid struct {
		tri      types.Triangle
		centroid float64
	}
	arr := make([]triCentroid, len(triangles))
	for i, tri := range triangles {
		centroid := (tri.V1.Add(tri.V2).Add(tri.V3)).Mul(1.0 / 3.0)
		var c float64
		switch axis {
		case 0:
			c = centroid.X
		case 1:
			c = centroid.Y
		case 2:
			c = centroid.Z
		}
		arr[i] = triCentroid{tri: tri, centroid: c}
	}
	sort.Slice(arr, func(i, j int) bool {
		return arr[i].centroid < arr[j].centroid
	})
	mid := len(arr) / 2
	leftTris := make([]types.Triangle, mid)
	rightTris := make([]types.Triangle, len(arr)-mid)
	for i, v := range arr {
		if i < mid {
			leftTris[i] = v.tri
		} else {
			rightTris[i-mid] = v.tri
		}
	}
	node.left = buildBVHRecursive(leftTris)
	node.right = buildBVHRecursive(rightTris)
	return node
}

// GetRelevantMapGeometry returns triangles along the line from start->end
func GetRelevantMapGeometry(start, end r3.Vector, m Model) []types.Triangle {
	visited := make(map[GridKey]bool, 128)
	var relevant []types.Triangle

	// Compute the direction and length.
	dir := end.Sub(start)
	length := dir.Norm()
	if length == 0 {
		key := GridKey{
			x: int(math.Floor(start.X / m.gridSize)),
			y: int(math.Floor(start.Y / m.gridSize)),
			z: int(math.Floor(start.Z / m.gridSize)),
		}
		if triList, ok := m.sectors[key]; ok {
			relevant = append(relevant, triList...)
		}
		return relevant
	}
	direction := dir.Mul(1 / length)

	// Initialize starting cell using floor division.
	cellX := int(math.Floor(start.X / m.gridSize))
	cellY := int(math.Floor(start.Y / m.gridSize))
	cellZ := int(math.Floor(start.Z / m.gridSize))

	// Setup DDA: determine stepping and initial tMax/tDelta values.
	var stepX, stepY, stepZ int
	var tMaxX, tMaxY, tMaxZ float64
	var tDeltaX, tDeltaY, tDeltaZ float64

	if direction.X > 0 {
		stepX = 1
		tMaxX = (((float64(cellX)+1)*m.gridSize - start.X) / direction.X)
		tDeltaX = m.gridSize / direction.X
	} else if direction.X < 0 {
		stepX = -1
		tMaxX = (start.X - float64(cellX)*m.gridSize) / -direction.X
		tDeltaX = m.gridSize / -direction.X
	} else {
		tMaxX = math.MaxFloat64
		tDeltaX = math.MaxFloat64
	}

	if direction.Y > 0 {
		stepY = 1
		tMaxY = (((float64(cellY)+1)*m.gridSize - start.Y) / direction.Y)
		tDeltaY = m.gridSize / direction.Y
	} else if direction.Y < 0 {
		stepY = -1
		tMaxY = (start.Y - float64(cellY)*m.gridSize) / -direction.Y
		tDeltaY = m.gridSize / -direction.Y
	} else {
		tMaxY = math.MaxFloat64
		tDeltaY = math.MaxFloat64
	}

	if direction.Z > 0 {
		stepZ = 1
		tMaxZ = (((float64(cellZ)+1)*m.gridSize - start.Z) / direction.Z)
		tDeltaZ = m.gridSize / direction.Z
	} else if direction.Z < 0 {
		stepZ = -1
		tMaxZ = (start.Z - float64(cellZ)*m.gridSize) / -direction.Z
		tDeltaZ = m.gridSize / -direction.Z
	} else {
		tMaxZ = math.MaxFloat64
		tDeltaZ = math.MaxFloat64
	}

	// Choose a corridor radius that isn’t too big.
	const corridor = 1

	t := 0.0
	for t <= length {
		// Instead of calling fmt.Sprintf, use our gridKey struct.
		for dx := -corridor; dx <= corridor; dx++ {
			for dy := -corridor; dy <= corridor; dy++ {
				for dz := -corridor; dz <= corridor; dz++ {
					key := GridKey{
						x: cellX + dx,
						y: cellY + dy,
						z: cellZ + dz,
					}
					if !visited[key] {
						visited[key] = true
						if triList, ok := m.sectors[key]; ok {
							relevant = append(relevant, triList...)
						}
					}
				}
			}
		}

		// Step to the next grid cell using DDA.
		if tMaxX < tMaxY {
			if tMaxX < tMaxZ {
				t = tMaxX
				cellX += stepX
				tMaxX += tDeltaX
			} else {
				t = tMaxZ
				cellZ += stepZ
				tMaxZ += tDeltaZ
			}
		} else {
			if tMaxY < tMaxZ {
				t = tMaxY
				cellY += stepY
				tMaxY += tDeltaY
			} else {
				t = tMaxZ
				cellZ += stepZ
				tMaxZ += tDeltaZ
			}
		}
	}

	// Also check the start and end cells with a slightly larger corridor.
	for _, point := range []r3.Vector{start, end} {
		cx := int(math.Floor(point.X / m.gridSize))
		cy := int(math.Floor(point.Y / m.gridSize))
		cz := int(math.Floor(point.Z / m.gridSize))
		const endpointCorridor = 2
		for dx := -endpointCorridor; dx <= endpointCorridor; dx++ {
			for dy := -endpointCorridor; dy <= endpointCorridor; dy++ {
				for dz := -endpointCorridor; dz <= endpointCorridor; dz++ {
					key := GridKey{cx + dx, cy + dy, cz + dz}
					if !visited[key] {
						visited[key] = true
						if triList, ok := m.sectors[key]; ok {
							relevant = append(relevant, triList...)
						}
					}
				}
			}
		}
	}

	return relevant
}
