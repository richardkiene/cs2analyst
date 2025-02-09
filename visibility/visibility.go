package visibility

import (
	"bufio"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/collector"
	"github.com/richardkiene/cs2analyst/types"
)

type Visibility struct {
	objDirPath string // Directory where the 3d .obj files for geometry can be found
	LosSystem  LineOfSightSystem
	logger     slog.Logger
}

func New(objDirPath string) *Visibility {
	return &Visibility{
		objDirPath: objDirPath,
		logger:     *slog.Default(),
	}
}

// Model represents a 3D model loaded from an OBJ file
type Model struct {
	triangles []types.Triangle
	// Bounding information
	min, max r3.Vector
	// Spatial partitioning for map geometry
	sectors  map[string][]types.Triangle
	gridSize float64
}

// NewModel creates a new model with spatial partitioning initialized
func NewModel() *Model {
	return &Model{
		sectors:  make(map[string][]types.Triangle),
		gridSize: 16.0,
		min:      r3.Vector{X: 1e10, Y: 1e10, Z: 1e10},
		max:      r3.Vector{X: -1e10, Y: -1e10, Z: -1e10},
	}
}

type LineOfSightSystem struct {
	mapModel    *Model
	playerModel *Model
	logger      *slog.Logger
}

func (v *Visibility) NewLineOfSightSystem(mapName, cs2MapsPath string) (*LineOfSightSystem, error) {
	los := LineOfSightSystem{
		logger: slog.Default(),
	}

	mapModel, err := loadMapModel("")
	if err != nil {
		return nil, fmt.Errorf("failed to create a new LineOfSightSystem: %v", err)
	}

	playerModel, err := loadPlayerModel("")
	if err != nil {
		return nil, fmt.Errorf("failed to create a new LineOfSightSystem: %v", err)
	}

	los.mapModel = mapModel
	los.playerModel = playerModel

	return &los, nil

}

func loadMapModel(mapModelObjPath string) (*Model, error) {
	// TODO: mapModelObjPath should be used instead of a hardcoded string
	mapModel, err := LoadOBJ("C:\\Users\\richa\\code\\CS2ResourceAPI\\GameDataService\\ModelOutput\\world_output.obj")
	if err != nil {
		return nil, fmt.Errorf("failed to load map model: %v", err)
	}
	return mapModel, nil
}

func loadPlayerModel(playerModelObjPath string) (*Model, error) {
	// TODO: playerModelObjPath should be used instead of a hardcoded string
	playerModel, err := LoadOBJ("C:\\Users\\richa\\code\\CS2ResourceAPI\\GameDataService\\ModelOutput\\ctm_sas_output.obj")
	if err != nil {
		return nil, fmt.Errorf("failed to load player model: %v", err)
	}
	return playerModel, nil
}

// LoadOBJ loads a 3D model from an OBJ file and calculates its bounds
func LoadOBJ(filename string) (*Model, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	model := NewModel()
	var vertices []r3.Vector

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "v":
			if len(fields) < 4 {
				continue
			}
			x, _ := strconv.ParseFloat(fields[1], 64)
			y, _ := strconv.ParseFloat(fields[2], 64)
			z, _ := strconv.ParseFloat(fields[3], 64)
			vertex := r3.Vector{X: x, Y: y, Z: z}
			vertices = append(vertices, vertex)

			// Update model bounds
			model.min.X = math.Min(model.min.X, x)
			model.min.Y = math.Min(model.min.Y, y)
			model.min.Z = math.Min(model.min.Z, z)
			model.max.X = math.Max(model.max.X, x)
			model.max.Y = math.Max(model.max.Y, y)
			model.max.Z = math.Max(model.max.Z, z)

		case "f":
			if len(fields) < 4 {
				continue
			}
			v1Idx, _ := strconv.Atoi(strings.Split(fields[1], "/")[0])
			v2Idx, _ := strconv.Atoi(strings.Split(fields[2], "/")[0])
			v3Idx, _ := strconv.Atoi(strings.Split(fields[3], "/")[0])

			triangle := types.Triangle{
				V1: vertices[v1Idx-1],
				V2: vertices[v2Idx-1],
				V3: vertices[v3Idx-1],
			}
			model.triangles = append(model.triangles, triangle)

			// If this is a map model, add to spatial partitioning
			if model.gridSize > 0 {
				model.addTriangleToSectors(triangle)
			}
		}
	}

	return model, nil
}

func (v *Visibility) FindLastContinuousVisibilityStart(playerID, targetID uint64, currentTick int, perTickInfo map[int]map[uint64]collector.PlayerTickData) (int, bool) {
	invisibleTicks := 0
	lastVisibleTick := -1
	lastInvisibleTick := -1
	firstVisibleTick := -1 // Track when visibility was first gained

	for tick := currentTick; tick >= 0; tick-- {
		if currentTick-tick > 320 {
			break
		}

		playerData, exists := perTickInfo[tick]
		if !exists {
			continue
		}

		playerTick, exists := playerData[playerID]
		if !exists || !playerTick.IsAlive || playerTick.IsBlinded {
			continue
		}

		targetTick, exists := playerData[targetID]
		if !exists || !targetTick.IsAlive {
			continue
		}

		// Pass -1 for tick to prevent OBJ creation during scanning
		isVisible := CanSeeTarget(playerData[playerID], playerData[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, -1)

		if isVisible {
			if lastVisibleTick == -1 { // First moment of visibility
				firstVisibleTick = tick
			}
			lastVisibleTick = tick
			invisibleTicks = 0
		} else {
			lastInvisibleTick = tick
			invisibleTicks++
			if invisibleTicks > 8 && lastVisibleTick != -1 {
				// We've found the visibility boundary - create the OBJ files
				if lastInvisibleTick != -1 {
					// Create OBJ for the last invisible moment
					playerData := perTickInfo[lastInvisibleTick]
					CanSeeTarget(playerData[playerID], playerData[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, lastInvisibleTick)
				}
				if firstVisibleTick != -1 {
					// Create OBJ for the first visible moment
					playerData := perTickInfo[firstVisibleTick]
					CanSeeTarget(playerData[playerID], playerData[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, firstVisibleTick)
				}
				return lastVisibleTick, true
			}
		}
	}

	// If we hit the tick limit, still create the OBJ files if we found visibility
	if lastVisibleTick != -1 {
		if lastInvisibleTick != -1 {
			playerData := perTickInfo[lastInvisibleTick]
			CanSeeTarget(playerData[playerID], playerData[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, lastInvisibleTick)
		}
		if firstVisibleTick != -1 {
			playerData := perTickInfo[firstVisibleTick]
			CanSeeTarget(playerData[playerID], playerData[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, firstVisibleTick)
		}
	}

	return lastVisibleTick, lastVisibleTick != -1
}

// getSectorKey returns a string key for spatial partitioning
func (m *Model) getSectorKey(pos r3.Vector) string {
	// Maintain spatial consistency with Source 2 coordinates
	x := int(pos.X / m.gridSize) // Forward/East
	y := int(pos.Y / m.gridSize) // Left/North
	z := int(pos.Z / m.gridSize) // Up
	return fmt.Sprintf("%d:%d:%d", x, y, z)
}

// addTriangleToSectors adds a triangle to relevant spatial sectors
func (m *Model) addTriangleToSectors(t types.Triangle) {
	// Get affected sectors based on triangle bounds
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
				key := fmt.Sprintf("%d:%d:%d", x, y, z)
				m.sectors[key] = append(m.sectors[key], t)
			}
		}
	}
}

// GetRelevantMapGeometry returns map triangles that could intersect with the line of sight
func (m *Model) GetRelevantMapGeometry(start, end r3.Vector) []types.Triangle {
	visited := make(map[string]bool)
	var relevantTriangles []types.Triangle

	// Calculate direction and length for the line check
	dir := r3.Vector{
		X: end.X - start.X,
		Y: end.Y - start.Y,
		Z: end.Z - start.Z,
	}
	length := dir.Norm()
	steps := int(length/m.gridSize) + 1

	// Use a smaller corridor radius since we're combining approaches
	const corridorRadius = 1 // Reduced from 3 to 1

	// First, check sectors along the line of sight
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		pos := r3.Vector{
			X: start.X + dir.X*t,
			Y: start.Y + dir.Y*t,
			Z: start.Z + dir.Z*t,
		}

		// Check immediate neighboring sectors
		for dx := -corridorRadius; dx <= corridorRadius; dx++ {
			for dy := -corridorRadius; dy <= corridorRadius; dy++ {
				for dz := -corridorRadius; dz <= corridorRadius; dz++ {
					// Skip sectors too far from line of sight
					if dx*dx+dy*dy+dz*dz > corridorRadius*corridorRadius {
						continue
					}

					checkPos := r3.Vector{
						X: pos.X + float64(dx)*m.gridSize,
						Y: pos.Y + float64(dy)*m.gridSize,
						Z: pos.Z + float64(dz)*m.gridSize,
					}

					key := m.getSectorKey(checkPos)
					if !visited[key] {
						visited[key] = true
						if triangles, exists := m.sectors[key]; exists {
							relevantTriangles = append(relevantTriangles, triangles...)
						}
					}
				}
			}
		}
	}

	// Also check the sectors at start and end points with slightly larger radius
	for _, point := range []r3.Vector{start, end} {
		const endpointRadius = 2 // Slightly larger radius at endpoints
		for dx := -endpointRadius; dx <= endpointRadius; dx++ {
			for dy := -endpointRadius; dy <= endpointRadius; dy++ {
				for dz := -endpointRadius; dz <= endpointRadius; dz++ {
					// Skip sectors too far from point
					if dx*dx+dy*dy+dz*dz > endpointRadius*endpointRadius {
						continue
					}

					checkPos := r3.Vector{
						X: point.X + float64(dx)*m.gridSize,
						Y: point.Y + float64(dy)*m.gridSize,
						Z: point.Z + float64(dz)*m.gridSize,
					}

					key := m.getSectorKey(checkPos)
					if !visited[key] {
						visited[key] = true
						if triangles, exists := m.sectors[key]; exists {
							relevantTriangles = append(relevantTriangles, triangles...)
						}
					}
				}
			}
		}
	}

	slog.Debug("GetRelevantMapGeometry",
		"startPos", start,
		"endPos", end,
		"sectorsChecked", len(visited),
		"trianglesFound", len(relevantTriangles),
		"gridSize", m.gridSize,
		"steps", steps)

	return relevantTriangles
}

/*
 * getVisibilityPoints returns key points to check for visibility based on the player model
 *
 * TODO: Currently this applies modifiers to the model width. This is due to our obj file
 * used to generate the model coordinates being in T stance with arms extended. We should
 * find a way to manipulate either the model before obj output or the direct coordinates
 * here so that the player model is in the correc stance and does not need this.
 */
func getVisibilityPoints(playerModel *Model) []r3.Vector {
	// Get model dimensions
	width := playerModel.max.Y - playerModel.min.Y  // Width is now along Y axis (left/right)
	depth := playerModel.max.X - playerModel.min.X  // Depth is now along X axis (forward/back)
	height := playerModel.max.Z - playerModel.min.Z // Height remains on Z axis

	// Points for Source 2 coordinate system
	points := []r3.Vector{
		// Head area (multiple points with depth)
		{X: depth * 0.5, Y: 0, Z: height * 0.9},  // Front of head
		{X: -depth * 0.5, Y: 0, Z: height * 0.9}, // Back of head
		{X: 0, Y: 0, Z: height * 0.8},            // Head center

		// Upper body with reduced width
		{X: depth * 0.3, Y: width * 0.15, Z: height * 0.7},   // Right shoulder front
		{X: -depth * 0.3, Y: width * 0.15, Z: height * 0.7},  // Right shoulder back
		{X: depth * 0.3, Y: -width * 0.15, Z: height * 0.7},  // Left shoulder front
		{X: -depth * 0.3, Y: -width * 0.15, Z: height * 0.7}, // Left shoulder back

		// Center mass with depth variations
		{X: depth * 0.5, Y: 0, Z: height * 0.5},  // Front center
		{X: -depth * 0.5, Y: 0, Z: height * 0.5}, // Back center

		// Lower body
		{X: depth * 0.3, Y: 0, Z: height * 0.3},  // Lower torso front
		{X: -depth * 0.3, Y: 0, Z: height * 0.3}, // Lower torso back
	}

	return points
}

// CanSeeTarget determines if a player can see a target using view angles and models
func CanSeeTarget(shooter, target collector.PlayerTickData, playerModel, mapModel *Model, tick int) bool {
	eyeHeight := playerModel.max.Z * 0.85
	if shooter.IsCrouched {
		eyeHeight *= 0.75
	}

	eyePos := r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y - 2.5,
		Z: shooter.Position.Z + eyeHeight,
	}

	points := getVisibilityPoints(playerModel)

	// First check if any point is in FOV
	anyPointInFOV := false
	for _, basePoint := range points {
		worldPoint := r3.Vector{
			X: target.Position.X + basePoint.X,
			Y: target.Position.Y + basePoint.Y,
			Z: target.Position.Z + basePoint.Z,
		}

		if shooter.IsInFieldOfView(worldPoint) {
			anyPointInFOV = true
			break
		}
	}

	if !anyPointInFOV {
		return false
	}

	// Now check line-of-sight for all points
	for _, basePoint := range points {
		worldPoint := r3.Vector{
			X: target.Position.X + basePoint.X,
			Y: target.Position.Y + basePoint.Y,
			Z: target.Position.Z + basePoint.Z,
		}

		if !shooter.IsInFieldOfView(worldPoint) {
			continue
		}

		// Ray from eyePos to the specific point on target
		rayDir := worldPoint.Sub(eyePos).Normalize()

		start := minVector(eyePos, worldPoint).Sub(r3.Vector{X: 200, Y: 200, Z: 200})
		end := maxVector(eyePos, worldPoint).Add(r3.Vector{X: 200, Y: 200, Z: 200})

		// For collision, we still only need relevant triangles
		relevantTriangles := mapModel.GetRelevantMapGeometry(start, end)

		blocked := false
		for _, tri := range relevantTriangles {
			if rayIntersectsTriangle(eyePos, rayDir, tri) {
				blocked = true
				break
			}
		}

		if !blocked {
			// We found a point that’s visible => can see target
			// Create a debug OBJ for SteamID 76561197991944713
			if tick >= 0 && shooter.SteamID == 76561197991944713 {
				/*_ = CreateFullDebugOBJ(
					tick,
					mapModel,    // entire map
					playerModel, // the player model
					shooter,
					target,
					false, // set to true if you *do* want a cone
				)*/

				/*err := CreateShooterFOVOBJ(
					tick,
					mapModel,
					playerModel,
					shooter,
					target,
					120.0, // e.g. 90 deg FOV
					false, // don't include a big cone
				)
				if err != nil {
					fmt.Println("Failed to write FOV debug:", err)
				}*/

				err := CreateShooterCentricFOVUsingTargetDistance(
					tick,
					mapModel,
					playerModel,
					shooter,
					target,
					120.0, // e.g. 90 deg FOV
					300.0, // Pad the context by 300
					true,  // include a big cone
				)
				if err != nil {
					fmt.Println("Failed to write FOV debug:", err)
				}
			}
			return true
		}
	}

	return false // none of the points were unblocked
}

func minVector(a, b r3.Vector) r3.Vector {
	return r3.Vector{
		X: math.Min(a.X, b.X),
		Y: math.Min(a.Y, b.Y),
		Z: math.Min(a.Z, b.Z),
	}
}

func maxVector(a, b r3.Vector) r3.Vector {
	return r3.Vector{
		X: math.Max(a.X, b.X),
		Y: math.Max(a.Y, b.Y),
		Z: math.Max(a.Z, b.Z),
	}
}

// rayIntersectsTriangle determines if a ray intersects with a triangle
func rayIntersectsTriangle(origin, direction r3.Vector, triangle types.Triangle) bool {
	const EPSILON = 0.0000001

	edge1 := r3.Vector{
		X: triangle.V2.X - triangle.V1.X,
		Y: triangle.V2.Y - triangle.V1.Y,
		Z: triangle.V2.Z - triangle.V1.Z,
	}
	edge2 := r3.Vector{
		X: triangle.V3.X - triangle.V1.X,
		Y: triangle.V3.Y - triangle.V1.Y,
		Z: triangle.V3.Z - triangle.V1.Z,
	}

	h := r3.Vector{
		X: direction.Y*edge2.Z - direction.Z*edge2.Y,
		Y: direction.Z*edge2.X - direction.X*edge2.Z,
		Z: direction.X*edge2.Y - direction.Y*edge2.X,
	}

	a := edge1.X*h.X + edge1.Y*h.Y + edge1.Z*h.Z
	if a > -EPSILON && a < EPSILON {
		return false
	}

	f := 1.0 / a
	s := r3.Vector{
		X: origin.X - triangle.V1.X,
		Y: origin.Y - triangle.V1.Y,
		Z: origin.Z - triangle.V1.Z,
	}

	u := f * (s.X*h.X + s.Y*h.Y + s.Z*h.Z)
	if u < 0.0 || u > 1.0 {
		return false
	}

	q := r3.Vector{
		X: s.Y*edge1.Z - s.Z*edge1.Y,
		Y: s.Z*edge1.X - s.X*edge1.Z,
		Z: s.X*edge1.Y - s.Y*edge1.X,
	}

	v := f * (direction.X*q.X + direction.Y*q.Y + direction.Z*q.Z)
	if v < 0.0 || u+v > 1.0 {
		return false
	}

	t := f * (edge2.X*q.X + edge2.Y*q.Y + edge2.Z*q.Z)
	return t > EPSILON
}
