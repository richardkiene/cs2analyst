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
	objDirPath string
	LosSystem  LineOfSightSystem
	logger     slog.Logger
}

func New(objDirPath string) *Visibility {
	return &Visibility{
		objDirPath: objDirPath,
		logger:     *slog.Default(),
	}
}

type Model struct {
	triangles []types.Triangle
	min, max  r3.Vector
	sectors   map[string][]types.Triangle
	gridSize  float64
	hitboxes  []Hitbox
}

type Hitbox struct {
	Name      string
	BoneName  string
	Vertices  []r3.Vector
	MinBounds r3.Vector
	MaxBounds r3.Vector
	Type      string // "Box", "Sphere", "Capsule"
}

func NewModel() *Model {
	return &Model{
		sectors:  make(map[string][]types.Triangle),
		gridSize: 16.0,
		min:      r3.Vector{X: 1e10, Y: 1e10, Z: 1e10},
		max:      r3.Vector{X: -1e10, Y: -1e10, Z: -1e10},
	}
}

// TrianglesRaw returns the model's triangle slice
func (m *Model) TrianglesRaw() []types.Triangle {
	return m.triangles
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
	mapM, err := loadMapModel("")
	if err != nil {
		return nil, fmt.Errorf("failed to create a new LOS system: %v", err)
	}
	playerM, err := loadPlayerModel("")
	if err != nil {
		return nil, fmt.Errorf("failed to create a new LOS system: %v", err)
	}
	los.mapModel = mapM
	los.playerModel = playerM
	return &los, nil
}

func loadMapModel(path string) (*Model, error) {
	m, err := LoadOBJ("C:\\Users\\richa\\code\\CS2ResourceAPI\\GameDataService\\ModelOutput\\world_output.obj")
	if err != nil {
		return nil, fmt.Errorf("failed to load map model: %w", err)
	}
	return m, nil
}

func loadPlayerModel(path string) (*Model, error) {
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
				key := fmt.Sprintf("%d:%d:%d", x, y, z)
				m.sectors[key] = append(m.sectors[key], t)
			}
		}
	}
}

func (m *Model) getSectorKey(pos r3.Vector) string {
	x := int(pos.X / m.gridSize)
	y := int(pos.Y / m.gridSize)
	z := int(pos.Z / m.gridSize)
	return fmt.Sprintf("%d:%d:%d", x, y, z)
}

// GetRelevantMapGeometry returns triangles along the line from start->end
func (m *Model) GetRelevantMapGeometry(start, end r3.Vector) []types.Triangle {
	visited := make(map[string]bool)
	var relevant []types.Triangle

	dir := end.Sub(start)
	length := dir.Norm()
	steps := int(length/m.gridSize) + 1

	const corridorRadius = 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		pos := start.Add(dir.Mul(t))
		for dx := -corridorRadius; dx <= corridorRadius; dx++ {
			for dy := -corridorRadius; dy <= corridorRadius; dy++ {
				for dz := -corridorRadius; dz <= corridorRadius; dz++ {
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
						if triList, ok := m.sectors[key]; ok {
							relevant = append(relevant, triList...)
						}
					}
				}
			}
		}
	}

	// check start+end with a bigger radius
	for _, point := range []r3.Vector{start, end} {
		const endpointRadius = 2
		for dx := -endpointRadius; dx <= endpointRadius; dx++ {
			for dy := -endpointRadius; dy <= endpointRadius; dy++ {
				for dz := -endpointRadius; dz <= endpointRadius; dz++ {
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
						if triList, ok := m.sectors[key]; ok {
							relevant = append(relevant, triList...)
						}
					}
				}
			}
		}
	}

	slog.Debug("GetRelevantMapGeometry",
		"startPos", start,
		"endPos", end,
		"sectorsVisited", len(visited),
		"trianglesFound", len(relevant),
	)
	return relevant
}

// FindLastContinuousVisibilityStart is unchanged from your code
func (v *Visibility) FindLastContinuousVisibilityStart(
	playerID, targetID uint64,
	currentTick int,
	perTickInfo map[int]map[uint64]collector.PlayerTickData,
) (int, bool) {
	invisibleTicks := 0
	lastVisibleTick := -1
	lastInvisibleTick := -1
	firstVisibleTick := -1

	for tick := currentTick; tick >= 0; tick-- {
		if currentTick-tick > 320 {
			break
		}

		playerData, ok := perTickInfo[tick]
		if !ok {
			continue
		}
		shooterTick, ok := playerData[playerID]
		if !ok || !shooterTick.IsAlive || shooterTick.IsBlinded {
			continue
		}
		targetTick, ok := playerData[targetID]
		if !ok || !targetTick.IsAlive {
			continue
		}

		isVisible := CanSeeTarget(shooterTick, targetTick, v.LosSystem.playerModel, v.LosSystem.mapModel, -1)
		if isVisible {
			if lastVisibleTick == -1 {
				firstVisibleTick = tick
			}
			lastVisibleTick = tick
			invisibleTicks = 0
		} else {
			lastInvisibleTick = tick
			invisibleTicks++
			if invisibleTicks > 8 && lastVisibleTick != -1 {
				if lastInvisibleTick != -1 {
					pd := perTickInfo[lastInvisibleTick]
					CanSeeTarget(pd[playerID], pd[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, lastInvisibleTick)
				}
				if firstVisibleTick != -1 {
					pd := perTickInfo[firstVisibleTick]
					CanSeeTarget(pd[playerID], pd[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, firstVisibleTick)
				}
				return lastVisibleTick, true
			}
		}
	}

	if lastVisibleTick != -1 {
		if lastInvisibleTick != -1 {
			pd := perTickInfo[lastInvisibleTick]
			CanSeeTarget(pd[playerID], pd[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, lastInvisibleTick)
		}
		if firstVisibleTick != -1 {
			pd := perTickInfo[firstVisibleTick]
			CanSeeTarget(pd[playerID], pd[targetID], v.LosSystem.playerModel, v.LosSystem.mapModel, firstVisibleTick)
		}
	}
	return lastVisibleTick, lastVisibleTick != -1
}

// getVisibilityPoints returns points on the player model (head, shoulders, torso)
func getVisibilityPoints(playerModel *Model) []r3.Vector {
	var points []r3.Vector

	for _, hitbox := range playerModel.hitboxes {
		center := r3.Vector{
			X: (hitbox.MinBounds.X + hitbox.MaxBounds.X) / 2,
			Y: (hitbox.MinBounds.Y + hitbox.MaxBounds.Y) / 2,
			Z: (hitbox.MinBounds.Z + hitbox.MaxBounds.Z) / 2,
		}

		// Always include center point
		points = append(points, center)

		// Add type-specific points
		switch hitbox.Type {
		case "Box":
			// Add front corners for box-type hitboxes
			points = append(points,
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MaxBounds.Z},
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MaxBounds.Z},
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MinBounds.Z},
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MinBounds.Z},
			)
		case "Sphere":
			// For spheres, add cardinal points
			radius := (hitbox.MaxBounds.Sub(hitbox.MinBounds)).Norm() / 2
			points = append(points,
				r3.Vector{X: center.X + radius, Y: center.Y, Z: center.Z},
				r3.Vector{X: center.X, Y: center.Y + radius, Z: center.Z},
				r3.Vector{X: center.X, Y: center.Y, Z: center.Z + radius},
			)
		case "Capsule":
			// For capsules, add points along the primary axis
			height := hitbox.MaxBounds.Z - hitbox.MinBounds.Z
			radius := (hitbox.MaxBounds.X - hitbox.MinBounds.X) / 2
			points = append(points,
				r3.Vector{X: center.X + radius, Y: center.Y, Z: center.Z},
				r3.Vector{X: center.X, Y: center.Y, Z: center.Z + height/4},
				r3.Vector{X: center.X, Y: center.Y, Z: center.Z - height/4},
			)
		}
	}

	return points
}

// CanSeeTarget checks if 'shooter' can see 'target' using line-of-sight from the shooter's eye
func CanSeeTarget(
	shooter, target collector.PlayerTickData,
	playerModel, mapModel *Model,
	tick int,
) bool {
	// 1) Compute the shooter’s eye position.
	// In Source2, shooter.Position is at the feet.
	// We use 85% of the model’s bounding box height as an approximate eye offset.
	eyeHeight := playerModel.max.Z * 0.85
	if shooter.IsCrouched {
		eyeHeight *= 0.75
	}
	// Use the computed eyeHeight so that the eye is above the feet.
	eyePos := r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y,
		Z: shooter.Position.Z + eyeHeight,
	}

	// 2) Use a relaxed FOV threshold.
	// With a base FOV of 120° (half-FOV = 60°), adding 10° slack gives an effective threshold of 70°.
	//effectiveHalfFOV := 70.0
	effectiveHalfFOV := 100.0

	// 3) Get the candidate visibility points on the target’s model.
	points := getVisibilityPoints(playerModel)

	// 4) Check if any visibility point is within the relaxed FOV when measured from eyePos.
	anyInFOV := false
	for _, bp := range points {
		wp := target.Position.Add(bp)
		// Compute the direction from eyePos to this candidate point.
		toTarget := wp.Sub(eyePos).Normalize()
		// Use the shooter’s forward vector (computed from view angles).
		forward := shooter.ForwardVector()
		// Compute the angle in degrees.
		angleDegrees := math.Acos(forward.Dot(toTarget)) * (180.0 / math.Pi)
		if angleDegrees <= effectiveHalfFOV {
			anyInFOV = true
			break
		}
	}
	if !anyInFOV {
		return false
	}

	// 5) For each visibility point that meets the relaxed FOV test, perform a line-of-sight (LOS) test.
	for _, bp := range points {
		wp := target.Position.Add(bp)
		// Recompute the angle from eyePos to the candidate point.
		toTarget := wp.Sub(eyePos).Normalize()
		forward := shooter.ForwardVector()
		angleDegrees := math.Acos(forward.Dot(toTarget)) * (180.0 / math.Pi)
		if angleDegrees > effectiveHalfFOV {
			continue
		}
		// Compute the ray direction from the eye to this point.
		rayDir := wp.Sub(eyePos).Normalize()

		// Define a bounding region for the LOS test (add padding).
		padding := r3.Vector{X: 200, Y: 200, Z: 200}
		start := minVector(eyePos, wp).Sub(padding)
		end := maxVector(eyePos, wp).Add(padding)
		relevantTriangles := mapModel.GetRelevantMapGeometry(start, end)

		blocked := false
		for _, tri := range relevantTriangles {
			if rayIntersectsTriangle(eyePos, rayDir, tri) {
				blocked = true
				break
			}
		}
		if !blocked {
			// Optionally, if you want to output debug geometry for a specific SteamID and tick:
			if tick >= 0 && shooter.SteamID == 76561197991944713 {
				_ = CreateShooterCentricFOVUsingTargetDistance(tick, mapModel, playerModel, shooter, target, 120.0, 300.0, true)
			}
			return true
		}
	}

	return false
}

func DebugEyePosConsole(
	shooter collector.PlayerTickData,
	playerModel *Model,
) {
	// 1) Calculate bounding-box height
	height := playerModel.max.Z - playerModel.min.Z

	// 2) Our existing "eyeHeight" logic
	eyeHeight := height * 0.85
	if shooter.IsCrouched {
		eyeHeight *= 0.75
	}

	// 3) Final eye pos
	eyePos := r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y,
		Z: shooter.Position.Z + eyeHeight,
	}

	fmt.Printf("\n=== DebugEyePosConsole ===\n")
	fmt.Printf("Player minZ=%.2f, maxZ=%.2f => boundingBoxHeight=%.2f\n",
		playerModel.min.Z, playerModel.max.Z, height)
	fmt.Printf("shooter feet= (%.2f, %.2f, %.2f)\n",
		shooter.Position.X, shooter.Position.Y, shooter.Position.Z)
	fmt.Printf("eyeHeight= %.2f => eyePos= (%.2f, %.2f, %.2f)\n\n",
		eyeHeight, eyePos.X, eyePos.Y, eyePos.Z)
}

func distanceToShooter(shooter collector.PlayerTickData, point r3.Vector) float64 {
	dx := point.X - shooter.Position.X
	dy := point.Y - shooter.Position.Y
	dz := point.Z - shooter.Position.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func isInFOV(shooter collector.PlayerTickData, point r3.Vector, fovDegrees float64) bool {
	toPoint := point.Sub(shooter.Position).Normalize()
	forward := shooter.ForwardVector()
	dot := forward.Dot(toPoint)
	angle := math.Acos(dot) * (180 / math.Pi)
	return angle <= fovDegrees/2
}

// basic ray intersection
func rayIntersectsTriangle(origin, direction r3.Vector, tri types.Triangle) bool {
	const EPSILON = 1e-5
	edge1 := tri.V2.Sub(tri.V1)
	edge2 := tri.V3.Sub(tri.V1)

	h := direction.Cross(edge2)
	a := edge1.Dot(h)
	if a > -EPSILON && a < EPSILON {
		return false
	}

	f := 1.0 / a
	s := origin.Sub(tri.V1)
	u := f * s.Dot(h)
	if u < 0.0 || u > 1.0 {
		return false
	}

	q := s.Cross(edge1)
	v := f * direction.Dot(q)
	if v < 0.0 || (u+v) > 1.0 {
		return false
	}

	dist := f * edge2.Dot(q)
	return dist > EPSILON
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
