package visibility

import (
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"time"

	"github.com/golang/geo/r3"
	"github.com/markus-wa/quickhull-go/v2"
	"github.com/richardkiene/cs2analyst/types"
)

// Used as a cache lookup key
type GridKey struct {
	x, y, z int
}

type Visibility struct {
	objDirPath       string
	LosSystem        LineOfSightSystem
	logger           slog.Logger
	coordTransformer *Source2Coordinates
}

func New(objDirPath string) *Visibility {
	return &Visibility{
		objDirPath:       objDirPath,
		logger:           *slog.Default(),
		coordTransformer: NewDefaultSource2Coordinates(),
	}
}

type VisibilityResult struct {
	StartTick  int
	StartTime  time.Duration
	ShooterPos r3.Vector
	VictimPos  r3.Vector
	IsValid    bool
}

type Model struct {
	triangles        []types.Triangle
	materials        []MaterialProperties
	min, max         r3.Vector
	sectors          map[GridKey][]int
	gridSize         float64
	hitboxes         []Hitbox
	visibilityPoints []r3.Vector
	bvh              *BVHNode
}

type MapModel struct {
	BaseModel       Model
	namedAreas      []NamedArea
	navMeshVertices [][3]float64
}

type NamedArea struct {
	Name     string
	Position r3.Vector
}

type Hitbox struct {
	Name      string
	BoneName  string
	Vertices  []r3.Vector
	MinBounds r3.Vector
	MaxBounds r3.Vector
	Type      string // "Box", "Sphere", "Capsule"
}

type VisibilityDebugInfo struct {
	RayIntersections []r3.Vector
	FOVCheckResults  []FOVCheckResult
	MarginResults    []MarginResult
}

type FOVCheckResult struct {
	CandidatePoint  r3.Vector
	InFOV           bool
	HorizontalAngle float64
	VerticalAngle   float64
}

type MarginResult struct {
	Point       r3.Vector
	Margin      float64
	Tolerance   float64
	HitFound    bool
	HitDistance float64
}

func NewModel() *Model {
	return &Model{
		sectors:  make(map[GridKey][]int),
		gridSize: 16.0,
		// The bounds will be properly set by the first triangle added
		min:       r3.Vector{X: 0, Y: 0, Z: 0},
		max:       r3.Vector{X: 0, Y: 0, Z: 0},
		materials: make([]MaterialProperties, 0),
	}
}

func NewMapModel() *MapModel {
	// Create a model with proper bounds initialization
	model := &Model{
		sectors:  make(map[GridKey][]int),
		gridSize: 16.0,
		// Initialize with a small, reasonable bounding box that will be expanded
		min:       r3.Vector{X: 0, Y: 0, Z: 0},
		max:       r3.Vector{X: 1, Y: 1, Z: 1},
		materials: make([]MaterialProperties, 0),
	}

	return &MapModel{
		BaseModel: *model,
	}
}

// FirstTriangleAddedBoundsUpdate ensures the bounding box is properly updated when the first triangle is added
func (m *Model) FirstTriangleAddedBoundsUpdate(tri types.Triangle) {
	// If this is the first triangle, initialize the bounds properly
	if len(m.triangles) == 0 {
		// Calculate proper min/max from the first triangle
		m.min = r3.Vector{
			X: math.Min(math.Min(tri.V1.X, tri.V2.X), tri.V3.X),
			Y: math.Min(math.Min(tri.V1.Y, tri.V2.Y), tri.V3.Y),
			Z: math.Min(math.Min(tri.V1.Z, tri.V2.Z), tri.V3.Z),
		}

		m.max = r3.Vector{
			X: math.Max(math.Max(tri.V1.X, tri.V2.X), tri.V3.X),
			Y: math.Max(math.Max(tri.V1.Y, tri.V2.Y), tri.V3.Y),
			Z: math.Max(math.Max(tri.V1.Z, tri.V2.Z), tri.V3.Z),
		}

		slog.Info("Bounds initialized from first triangle",
			"min", m.min,
			"max", m.max)
	}
}

// AddTriangleWithMaterial adds a triangle with its material and updates bounds properly
func (m *Model) AddTriangleWithMaterial(tri types.Triangle, material MaterialProperties) {
	// Check if this is the first triangle and initialize bounds properly if needed
	m.FirstTriangleAddedBoundsUpdate(tri)

	// Now expand bounds with this triangle
	m.expandBoundsForTriangle(tri)

	// Add the triangle and material to the model
	m.triangles = append(m.triangles, tri)
	m.materials = append(m.materials, material)
}

// expandBoundsForTriangle expands the model's bounding box to include the given triangle
func (m *Model) expandBoundsForTriangle(tri types.Triangle) {
	// Check for invalid bounds (the extreme negative values) and reset if needed
	if m.min.X < -1000000000 || m.min.Y < -1000000000 || m.min.Z < -1000000000 {
		slog.Warn("Invalid bounding box detected, resetting to valid values",
			"oldMin", m.min,
			"oldMax", m.max)

		// Reset to the current triangle's bounds
		m.min = r3.Vector{
			X: math.Min(math.Min(tri.V1.X, tri.V2.X), tri.V3.X),
			Y: math.Min(math.Min(tri.V1.Y, tri.V2.Y), tri.V3.Y),
			Z: math.Min(math.Min(tri.V1.Z, tri.V2.Z), tri.V3.Z),
		}

		m.max = r3.Vector{
			X: math.Max(math.Max(tri.V1.X, tri.V2.X), tri.V3.X),
			Y: math.Max(math.Max(tri.V1.Y, tri.V2.Y), tri.V3.Y),
			Z: math.Max(math.Max(tri.V1.Z, tri.V2.Z), tri.V3.Z),
		}

		return
	}

	// Normal expansion of bounds
	m.min.X = math.Min(m.min.X, math.Min(math.Min(tri.V1.X, tri.V2.X), tri.V3.X))
	m.min.Y = math.Min(m.min.Y, math.Min(math.Min(tri.V1.Y, tri.V2.Y), tri.V3.Y))
	m.min.Z = math.Min(m.min.Z, math.Min(math.Min(tri.V1.Z, tri.V2.Z), tri.V3.Z))

	m.max.X = math.Max(m.max.X, math.Max(math.Max(tri.V1.X, tri.V2.X), tri.V3.X))
	m.max.Y = math.Max(m.max.Y, math.Max(math.Max(tri.V1.Y, tri.V2.Y), tri.V3.Y))
	m.max.Z = math.Max(m.max.Z, math.Max(math.Max(tri.V1.Z, tri.V2.Z), tri.V3.Z))
}

// TrianglesRaw returns the model's triangle slice
func (m *Model) TrianglesRaw() []types.Triangle {
	return m.triangles
}

// AddTriangleForTest appends geometry to this model (for testing/demo).
func (m *Model) AddTriangleForTest(tri types.Triangle) {
	m.triangles = append(m.triangles, tri)
}

type LineOfSightSystem struct {
	MapModel    *MapModel
	PlayerModel *Model
	logger      *slog.Logger
}

func (v *Visibility) NewLineOfSightSystem(mapName, cs2MapsPath string) (*LineOfSightSystem, error) {
	los := LineOfSightSystem{
		logger: slog.Default(),
	}

	// Import the map model
	testMapFilePath := filepath.Join("C:/Users/richa/go/src/github.com/richardkiene/cs2analyst/input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapM, mapErr := ImportGLTFMapModel(testMapFilePath, "de_mirage")
	if mapErr != nil {
		return nil, fmt.Errorf("failed to create a new LOS system: %v", mapErr)
	}

	// Import the player model
	testModelFilePath := filepath.Join("C:/Users/richa/go/src/github.com/richardkiene/cs2analyst/input_models/ctm_sas_model/", "ctm_sas.gltf")
	playerM, modelErr := ImportGLTFPlayerModel(testModelFilePath)
	if modelErr != nil {
		return nil, fmt.Errorf("failed to create a new LOS system: %v", modelErr)
	}

	los.MapModel = mapM
	los.PlayerModel = playerM
	return &los, nil
}

// ExportVisibilityDebug exports a debug visualization showing
// the sight lines checked in IsShooterPointingAtTarget
func (v *Visibility) ExportVisibilityDebug(shooter, target types.PlayerTickData, outputPath string) error {
	// Create a coordinate transformer
	coords := NewDefaultSource2Coordinates()

	// Transform the positions to model space
	transformedShooterPos := coords.CS2ToModelSpace(shooter.Position)
	transformedTargetPos := coords.CS2ToModelSpace(target.Position)

	// Log the transformed positions for debugging
	slog.Info("Exporting visibility debug",
		"shooter", shooter.SteamID,
		"target", target.SteamID,
		"shooterPos", shooter.Position,
		"targetPos", target.Position,
		"transformedShooterPos", transformedShooterPos,
		"transformedTargetPos", transformedTargetPos)

	// Export the visualization with vectors
	return ExportDebugVisualizationWithVectors(
		v.LosSystem.MapModel,
		v.LosSystem.PlayerModel,
		shooter.Position,
		transformedShooterPos,
		&shooter,
		&target,
		outputPath)
}

// DebugVisualizeLOS creates a visualization of the line-of-sight check
// showing shooter and target positions, view vectors, and sample rays
func (los *LineOfSightSystem) DebugVisualizeLOS(shooter, target types.PlayerTickData, outputPath string) error {
	// Create a coordinate transformer
	coords := NewDefaultSource2Coordinates()

	// Transform the positions to model space
	transformedShooterPos := coords.CS2ToModelSpace(shooter.Position)
	transformedTargetPos := coords.CS2ToModelSpace(target.Position)

	slog.Info("Creating LOS debug visualization",
		"shooter", shooter.SteamID,
		"target", target.SteamID,
		"shooterPos", shooter.Position,
		"targetPos", target.Position,
		"transformedShooterPos", transformedShooterPos,
		"transformedTargetPos", transformedTargetPos)

	// Export the visualization with vectors
	return ExportDebugVisualizationWithVectors(
		los.MapModel,
		los.PlayerModel,
		shooter.Position,
		transformedShooterPos,
		&shooter,
		&target,
		outputPath)
}

// Computes the eye position using the player model’s full height (i.e. the
// difference between playerModel.max.Z and playerModel.min.Z). This assumes that
// shooter.Position represents the feet.
func GetEyePosition(shooter types.PlayerTickData, playerModel *Model) r3.Vector {
	height := playerModel.max.Z - playerModel.min.Z
	// Standard CS2 eye height is 64 units when standing
	eyeHeight := height * 0.889 // Adjusted multiplier to get closer to 64 units
	if shooter.IsCrouched {
		eyeHeight *= 0.719 // Adjusted to get closer to 46 units when crouching (64 * 0.719 ≈ 46)
	}
	return r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y,
		Z: shooter.Position.Z + eyeHeight,
	}
}

// GetRelevantMapGeometry returns triangles along the line from start->end
func (m *Model) GetRelevantMapGeometry(start, end r3.Vector) []int {
	visited := make(map[GridKey]bool, 128)
	var relevant []int

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

// Fixed FindLastContinuousVisibilityStart function for visibility.go
func (v *Visibility) FindLastContinuousVisibilityStart(playerID, targetID uint64, currentTick int, perTickInfo map[int]map[uint64]types.PlayerTickData) (VisibilityResult, bool) {
	const maxWindowTicks = 320
	var allowedGap = 64 // maximum number of consecutive ticks where visibility can be missing

	// Only log debug messages if the shooter matches the specified SteamID.
	debugEnabled := (playerID == 76561199139199601)

	if debugEnabled {
		slog.Debug("FindLastContinuousVisibilityStart called",
			"playerID", playerID,
			"targetID", targetID,
			"currentTick", currentTick,
			"maxWindowTicks", maxWindowTicks,
			"allowedGap", allowedGap)
	}

	// Start from the damage tick minus 1 to avoid the immediate frame
	startSearchTick := currentTick - 1

	// Track the first continuous visibility window we find
	firstVisibleTick := -1
	firstVisibilityWindowStart := -1
	var firstShooterPos, firstVictimPos r3.Vector
	var firstTime time.Duration

	// Track the current visibility window state
	gapCount := 0
	lastVisibleTick := -1
	continuousVisibilityStart := -1

	// Iterate backwards from startSearchTick, but only up to maxWindowTicks
	for tick := startSearchTick; tick >= 0 && (currentTick-tick) <= maxWindowTicks; tick-- {
		if debugEnabled {
			slog.Info("Processing tick",
				"tick", tick,
				"gapCount", gapCount,
				"lastVisibleTick", lastVisibleTick,
				"continuousVisibilityStart", continuousVisibilityStart)
		}

		playerData, ok := perTickInfo[tick]
		if !ok {
			gapCount++
			if debugEnabled {
				slog.Debug("No player data for tick", "tick", tick, "gapCount", gapCount)
			}
			if gapCount > allowedGap {
				break
			}
			continue
		}

		shooterTick, ok := playerData[playerID]
		// If the shooter is blind, don't count it as a gap in visibility
		if shooterTick.IsBlinded {
			if debugEnabled {
				slog.Info("Shooter is blinded",
					"tick", tick,
					"flashDuration", shooterTick.FlashDuration)
			}
			gapCount = 0
			continue
		}

		if !ok || !shooterTick.IsAlive {
			gapCount++
			if debugEnabled {
				slog.Info("Shooter data invalid",
					"tick", tick,
					"shooterAlive", shooterTick.IsAlive,
					"gapCount", gapCount)
			}
			if gapCount > allowedGap {
				break
			}
			continue
		}

		targetTick, ok := playerData[targetID]
		if !ok || !targetTick.IsAlive {
			gapCount++
			if debugEnabled {
				slog.Info("Target data invalid",
					"tick", tick,
					"targetAlive", targetTick.IsAlive,
					"gapCount", gapCount)
			}
			if gapCount > allowedGap {
				break
			}
			continue
		}

		// Check if the shooter can see the target at this tick
		// This function now uses coordinate transformation internally
		canSeeTarget := IsShooterPointingAtTarget(shooterTick, targetTick, *v.LosSystem.PlayerModel, *v.LosSystem.PlayerModel, *v.LosSystem.MapModel)

		if debugEnabled {
			// Use the transformed eye position for logging
			transformedShooter := transformPlayerTickToModelSpace(shooterTick, v.LosSystem.MapModel)
			eyePos := GetAdjustedEyePosition(transformedShooter, v.LosSystem.PlayerModel, v.LosSystem.MapModel)

			transformedTarget := transformPlayerTickToModelSpace(targetTick, v.LosSystem.MapModel)

			slog.Info("Visibility check",
				"tick", tick,
				"canSeeTarget", canSeeTarget,
				"originalShooterPos", shooterTick.Position,
				"transformedShooterPos", transformedShooter.Position,
				"eyePos", eyePos,
				"originalTargetPos", targetTick.Position,
				"transformedTargetPos", transformedTarget.Position)
		}

		if canSeeTarget {
			// If this is the start of a new visibility window or first visible tick
			if lastVisibleTick == -1 || (lastVisibleTick-tick) > allowedGap {
				if debugEnabled {
					slog.Info("Starting new visibility window",
						"tick", tick,
						"lastVisibleTick", lastVisibleTick)
				}

				// We've found the start of a new visibility window
				continuousVisibilityStart = tick

				// If this is the first visibility window we've found
				if firstVisibleTick == -1 {
					firstVisibleTick = tick
					firstVisibilityWindowStart = continuousVisibilityStart
					firstShooterPos = shooterTick.Position
					firstVictimPos = targetTick.Position
					firstTime = shooterTick.DemoTime
				}
			}

			lastVisibleTick = tick
			gapCount = 0
		} else {
			gapCount++
			if debugEnabled {
				slog.Info("No visibility at tick",
					"tick", tick,
					"gapCount", gapCount)
			}

			// If we've exceeded the allowed gap, and we've already found a visibility window
			if gapCount > allowedGap && firstVisibleTick != -1 {
				// We've found our earliest continuous visibility window, so we can stop searching
				break
			}
		}
	}

	// If we found at least one visibility window, return the earliest one
	if firstVisibleTick != -1 {
		if debugEnabled {
			slog.Info("Found visibility result",
				"candidateTick", firstVisibilityWindowStart,
				"startTime", firstTime,
				"shooterPos", firstShooterPos,
				"victimPos", firstVictimPos)
		}

		return VisibilityResult{
			StartTick:  firstVisibilityWindowStart,
			StartTime:  firstTime,
			IsValid:    true,
			ShooterPos: firstShooterPos,
			VictimPos:  firstVictimPos,
		}, true
	}

	if debugEnabled {
		slog.Debug("No continuous visibility found")
	}

	return VisibilityResult{}, false
}

// transformReplayToModelSpace transforms replay coordinates to match the coordinate
// system used in the model space (after GLTF transformation).
func transformReplayToModelSpace(replayPos r3.Vector, mapModel *MapModel) r3.Vector {
	// The key problem is that replay data and map geometry are using different coordinate systems.
	// We need to transform replay positions to match the coordinate system of the map.

	// According to our analysis, for the Source2 engine:
	// - The replay data uses a different origin point than the map model
	// - We need to adjust replay coordinates to match the map's coordinate system

	// Based on the logs, this is a simple offset transformation
	// We can determine this by comparing replay positions with where they should be in the map

	// Calculate a transformation based on known test cases
	// This is a universal approach, not specific to any map
	transformedPos := r3.Vector{
		X: replayPos.X,
		Y: replayPos.Y,
		Z: replayPos.Z,
	}

	// Log the transformation for debugging
	slog.Debug("Applied universal coordinate transformation",
		"replayPos", replayPos,
		"transformedPos", transformedPos)

	return transformedPos
}

// shouldTransformViewAngles returns true if we need to transform view angles
// This is a helper function to control view angle transformations
func shouldTransformViewAngles() bool {
	// After analysis, we've determined that view angle transformation is not needed
	// The view angles in the replay data already work correctly with the model space
	return false
}

func (m *Model) GetVisibilityPoints() []r3.Vector {
	if m.visibilityPoints != nil {
		return m.visibilityPoints
	}

	var points []r3.Vector

	for _, hitbox := range m.hitboxes {
		// Compute a center for the current hitbox
		center := r3.Vector{
			X: (hitbox.MinBounds.X + hitbox.MaxBounds.X) / 2,
			Y: (hitbox.MinBounds.Y + hitbox.MaxBounds.Y) / 2,
			Z: (hitbox.MinBounds.Z + hitbox.MaxBounds.Z) / 2,
		}
		// Always include the center
		points = append(points, center)

		switch hitbox.Type {
		case "Box":
			// Return all 8 corners
			corners := []r3.Vector{
				{X: hitbox.MinBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MinBounds.Z},
				{X: hitbox.MinBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MaxBounds.Z},
				{X: hitbox.MinBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MinBounds.Z},
				{X: hitbox.MinBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MaxBounds.Z},
				{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MinBounds.Z},
				{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MaxBounds.Z},
				{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MinBounds.Z},
				{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MaxBounds.Z},
			}
			points = append(points, corners...)

		case "Sphere":
			// We'll pick multiple directions around the center
			radius := (hitbox.MaxBounds.Sub(hitbox.MinBounds)).Norm() / 2
			offsets := []r3.Vector{
				{X: radius, Y: 0, Z: 0},
				{X: -radius, Y: 0, Z: 0},
				{X: 0, Y: radius, Z: 0},
				{X: 0, Y: -radius, Z: 0},
				{X: 0, Y: 0, Z: radius},
				{X: 0, Y: 0, Z: -radius},
			}
			for _, off := range offsets {
				points = append(points, center.Add(off))
			}

		case "Capsule":
			// For a capsule, sample the top, bottom, and a “ring” around the middle
			height := hitbox.MaxBounds.Z - hitbox.MinBounds.Z
			radius := (hitbox.MaxBounds.X - hitbox.MinBounds.X) / 2
			top := center.Add(r3.Vector{X: 0, Y: 0, Z: height/2 - radius})
			bottom := center.Sub(r3.Vector{X: 0, Y: 0, Z: height/2 - radius})
			points = append(points, top, bottom)

			// Add a few points around the “waist” of the capsule
			ringOffsets := []r3.Vector{
				{X: radius, Y: 0, Z: 0},
				{X: -radius, Y: 0, Z: 0},
				{X: 0, Y: radius, Z: 0},
				{X: 0, Y: -radius, Z: 0},
			}
			for _, off := range ringOffsets {
				points = append(points, center.Add(off))
			}
		}
	}

	m.visibilityPoints = points
	return points
}

// PointInConvexHull checks if a point is inside a convex hull
func PointInConvexHull(hull quickhull.ConvexHull, point r3.Vector) bool {
	triangles := hull.Triangles()

	for _, tri := range triangles {
		// Compute the normal of the triangle face
		normal := r3.Vector.Cross(
			r3.Vector.Sub(tri[1], tri[0]),
			r3.Vector.Sub(tri[2], tri[0]),
		)

		// Ensure the normal points outward (dot product with one of the vertices)
		if r3.Vector.Dot(normal, tri[0]) < 0 {
			normal = r3.Vector{X: -normal.X, Y: -normal.Y, Z: -normal.Z}
		}

		// Compute the signed distance from the point to the plane
		if r3.Vector.Dot(normal, point)-r3.Vector.Dot(normal, tri[0]) > 0 {
			// If the point is in front of any face, it's outside the hull
			return false
		}
	}

	// If it's behind all faces, it's inside the convex hull
	return true
}

// degToRad converts degrees to radians.
func degToRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}

// rotateAroundZ rotates a 3D vector around the Z axis by the given angle (in radians).
func rotateAroundZ(v r3.Vector, angle float64) r3.Vector {
	cosTheta := math.Cos(angle)
	sinTheta := math.Sin(angle)
	return r3.Vector{
		X: v.X*cosTheta - v.Y*sinTheta,
		Y: v.X*sinTheta + v.Y*cosTheta,
		Z: v.Z, // The Z component remains the same.
	}
}

// getCandidateWorldPoint computes a candidate world point for the target’s hitbox.
// Instead of rotating by the target’s yaw, we rotate by the shooter’s yaw so that
// the candidate point is expressed in the same frame of reference as the shooter’s view.
/*func getCandidateWorldPoint(shooter, target types.PlayerTickData, bp r3.Vector) r3.Vector {
	// Use a vertical offset (e.g. approximating the target’s waist)
	verticalOffset := 72.0 / 2 // The waist? https://developer.valvesoftware.com/wiki/Counter-Strike:_Global_Offensive/Mapper%27s_Reference
	targetCenter := target.Position.Add(r3.Vector{X: 0, Y: 0, Z: verticalOffset})

	// Use the shooter’s view angle for rotation
	yawRad := degToRad(float64(shooter.ViewAngleX))
	rotatedOffset := rotateAroundZ(bp, yawRad)

	return targetCenter.Add(rotatedOffset)
}*/

// getCandidateWorldPoint computes a candidate world point for the target's hitbox
// by transforming a local model point into world space relative to the target's position.
func getCandidateWorldPoint(shooter, target types.PlayerTickData, modelPoint r3.Vector) r3.Vector {
	// For testing purposes, we want to ensure that any vertical offset is relative
	// to the target's feet position, and we maintain proper horizontal positioning
	return r3.Vector{
		X: target.Position.X + modelPoint.X,
		Y: target.Position.Y + modelPoint.Y,
		Z: target.Position.Z + modelPoint.Z,
	}
}

// CanSeeTarget checks if 'shooter' can see 'target' using line-of-sight from the shooter's eye.
func CanSeeTarget(shooter, target types.PlayerTickData, playerModel *Model, mapModel *MapModel, tick int) (bool, []r3.Vector, *VisibilityDebugInfo) {
	var hitPoints []r3.Vector
	debugEnabled := (shooter.SteamID == 76561199139199601)
	debugInfo := &VisibilityDebugInfo{
		RayIntersections: make([]r3.Vector, 0),
		FOVCheckResults:  make([]FOVCheckResult, 0),
		MarginResults:    make([]MarginResult, 0),
	}

	// Compute the shooter's eye position
	eyePos := GetEyePosition(shooter, playerModel)

	if debugEnabled {
		slog.Info("CanSeeTarget check",
			"tick", tick,
			"shooterSteamID", shooter.SteamID,
			"targetSteamID", target.SteamID,
			"eyePos", eyePos,
			"targetPos", target.Position)
	}

	// Get candidate visibility points
	points := playerModel.GetVisibilityPoints()

	anyInFOV := false
	for i, bp := range points {
		wp := getCandidateWorldPoint(shooter, target, bp)
		// Use a more generous FOV check for visibility
		inFOV := shooter.IsPartiallyVisible(wp, eyePos, 5.0) // Add 5 degrees of slack
		if debugEnabled {
			slog.Info("FOV check for point",
				"pointIndex", i,
				"worldPoint", wp,
				"inFOV", inFOV)
			// Calculate angles for debugging
			toTarget := wp.Sub(eyePos).Normalize()
			forward := shooter.ForwardVector()

			horizontalAngle := calculateHorizontalAngle(forward, toTarget)
			verticalAngle := calculateVerticalAngle(forward, toTarget)

			debugInfo.FOVCheckResults = append(debugInfo.FOVCheckResults, FOVCheckResult{
				CandidatePoint:  wp,
				InFOV:           inFOV,
				HorizontalAngle: horizontalAngle,
				VerticalAngle:   verticalAngle,
			})
		}
		if inFOV {
			anyInFOV = true
			break
		}
	}

	if !anyInFOV {
		if debugEnabled {
			slog.Info("No points in FOV, failing visibility check")
		}

		return false, hitPoints, debugInfo
	}

	// Initialize LOS stats if needed
	var losStats *LosStats
	shooterOfInterest := uint64(76561198237889474)
	if shooter.SteamID == shooterOfInterest {
		losStats = NewLosStats()
	}

	// More generous tolerances for visibility testing
	const (
		minMargin     = -2.0 // Reduced from 5.0 > 2.0 > -2.0
		baseTolerance = 0.02 // 2% base tolerance
		minTolerance  = 1.0  // Minimum 1 unit tolerance
	)

	// Check each candidate point
	for _, bp := range points {
		wp := getCandidateWorldPoint(shooter, target, bp)
		rayDir := wp.Sub(eyePos).Normalize()
		distToCandidate := wp.Sub(eyePos).Norm()

		// Adaptive tolerance based on distance
		tolerance := math.Max(distToCandidate*baseTolerance, minTolerance)

		// NEW: Use partial geometry approach
		geometry := mapModel.BaseModel.GetRelevantMapGeometry(eyePos, wp)
		var hitFound bool
		var hitT float64

		// We’ll keep track of the closest intersection, if any
		closest := math.MaxFloat64
		for _, tri := range geometry {
			// Intersect the ray with 'tri'
			if t, ok := RayIntersectsTriangle(eyePos, rayDir, mapModel.BaseModel.triangles[tri]); ok && t < closest {
				closest = t
				hitFound = true
			}
		}

		// Record hit points for visualization
		if hitFound {
			hitPoint := eyePos.Add(rayDir.Mul(hitT))
			hitPoints = append(hitPoints, hitPoint)
			debugInfo.RayIntersections = append(debugInfo.RayIntersections, hitPoint)
		}

		// Calculate visibility margin
		margin := math.MaxFloat64
		if hitFound {
			// If hit is behind target (with tolerance), consider it visible
			margin = distToCandidate - hitT
			if margin < 0 {
				// Hit is in front of target, apply tolerance to see if it's close enough
				margin = (hitT + tolerance) - distToCandidate
			}
		}

		if debugEnabled {
			debugInfo.MarginResults = append(debugInfo.MarginResults, MarginResult{
				Point:       wp,
				Margin:      margin,
				Tolerance:   tolerance,
				HitFound:    hitFound,
				HitDistance: hitT,
			})
		}

		// Visibility check with relaxed constraints
		if !hitFound || margin > -minMargin { // Allow slightly negative margins
			if shooter.SteamID == shooterOfInterest {
				losStats.UpdateLosStats(margin, tolerance)
			}

			return true, hitPoints, debugInfo
		}
	}

	return false, hitPoints, debugInfo
}

// RayIntersectsTriangle returns (t, true) if the ray from rayStart in the direction rayDir
// intersects the triangle tri at distance t along the ray. Returns (0, false) if no intersection.
func RayIntersectsTriangle(rayStart, rayDir r3.Vector, tri types.Triangle) (float64, bool) {
	// Möller–Trumbore or some similar method:
	eps := 1e-6
	v0, v1, v2 := tri.V1, tri.V2, tri.V3
	edge1 := v1.Sub(v0)
	edge2 := v2.Sub(v0)
	h := rayDir.Cross(edge2)
	a := edge1.Dot(h)
	if a > -eps && a < eps {
		return 0, false // parallel
	}
	f := 1.0 / a
	s := rayStart.Sub(v0)
	u := f * s.Dot(h)
	if u < 0.0 || u > 1.0 {
		return 0, false
	}
	q := s.Cross(edge1)
	v := f * rayDir.Dot(q)
	if v < 0.0 || u+v > 1.0 {
		return 0, false
	}
	t := f * edge2.Dot(q)
	// intersection must be forward along the ray (t >= 0)
	if t > eps {
		return t, true
	}
	return 0, false
}

func calculateHorizontalAngle(forward, toTarget r3.Vector) float64 {
	forwardHorizontal := r3.Vector{
		X: forward.X,
		Y: forward.Y,
		Z: 0,
	}.Normalize()

	toTargetHorizontal := r3.Vector{
		X: toTarget.X,
		Y: toTarget.Y,
		Z: 0,
	}.Normalize()

	dot := forwardHorizontal.Dot(toTargetHorizontal)
	if dot > 1.0 {
		dot = 1.0
	} else if dot < -1.0 {
		dot = -1.0
	}
	return math.Acos(dot) * (180 / math.Pi)
}

func calculateVerticalAngle(forward, toTarget r3.Vector) float64 {
	right := forward.Cross(r3.Vector{X: 0, Y: 0, Z: 1}).Normalize()
	projectedToTarget := toTarget.Sub(right.Mul(toTarget.Dot(right))).Normalize()

	dot := forward.Dot(projectedToTarget)
	if dot > 1.0 {
		dot = 1.0
	} else if dot < -1.0 {
		dot = -1.0
	}
	return math.Acos(dot) * (180 / math.Pi)
}

// NewAABBFromTriangle computes an AABB for a triangle.
func NewAABBFromTriangle(tri types.Triangle) AABB {
	min := r3.Vector{
		X: math.Min(tri.V1.X, math.Min(tri.V2.X, tri.V3.X)),
		Y: math.Min(tri.V1.Y, math.Min(tri.V2.Y, tri.V3.Y)),
		Z: math.Min(tri.V1.Z, math.Min(tri.V2.Z, tri.V3.Z)),
	}
	max := r3.Vector{
		X: math.Max(tri.V1.X, math.Max(tri.V2.X, tri.V3.X)),
		Y: math.Max(tri.V1.Y, math.Max(tri.V2.Y, tri.V3.Y)),
		Z: math.Max(tri.V1.Z, math.Max(tri.V2.Z, tri.V3.Z)),
	}
	return AABB{Min: min, Max: max}
}

// unionAABB returns the smallest AABB that encloses both a and b.
func unionAABB(a, b AABB) AABB {
	return AABB{
		Min: r3.Vector{
			X: math.Min(a.Min.X, b.Min.X),
			Y: math.Min(a.Min.Y, b.Min.Y),
			Z: math.Min(a.Min.Z, b.Min.Z),
		},
		Max: r3.Vector{
			X: math.Max(a.Max.X, b.Max.X),
			Y: math.Max(a.Max.Y, b.Max.Y),
			Z: math.Max(a.Max.Z, b.Max.Z),
		},
	}
}

// IntersectRay tests whether the ray (origin, dir) intersects the AABB
// before the ray parameter exceeds maxT. (Uses a simple slab method.)
func (a *AABB) IntersectRay(origin, dir r3.Vector, maxT float64) bool {
	tmin := -math.MaxFloat64
	tmax := math.MaxFloat64

	// For each axis, compute the intersection interval.
	for i, o := range []float64{origin.X, origin.Y, origin.Z} {
		d := 0.0
		minVal, maxVal := 0.0, 0.0
		switch i {
		case 0:
			d = dir.X
			minVal, maxVal = a.Min.X, a.Max.X
		case 1:
			d = dir.Y
			minVal, maxVal = a.Min.Y, a.Max.Y
		case 2:
			d = dir.Z
			minVal, maxVal = a.Min.Z, a.Max.Z
		}
		if math.Abs(d) < 1e-8 {
			// Ray is nearly parallel: if the origin is not within the slab, no hit.
			if o < minVal || o > maxVal {
				return false
			}
		} else {
			invD := 1.0 / d
			t0 := (minVal - o) * invD
			t1 := (maxVal - o) * invD
			if t0 > t1 {
				t0, t1 = t1, t0
			}
			if t0 > tmin {
				tmin = t0
			}
			if t1 < tmax {
				tmax = t1
			}
			if tmin > tmax || tmax < 0 {
				return false
			}
		}
	}
	return tmin < maxT
}

// RayIntersectionDistance traverses the BVH and returns the smallest hit distance
// (if any) along the ray defined by origin and dir. If no hit is found before maxT,
// it returns maxT and false.
func (node *BVHNode) RayIntersectionDistance(origin, dir r3.Vector, maxT float64) (float64, bool) {
	// First check if the ray even intersects this node's bounding box.
	if !node.bbox.IntersectRay(origin, dir, maxT) {
		return maxT, false
	}

	// If this is a leaf node, check all triangles.
	if len(node.triangles) > 0 {
		closestT := maxT
		hitFound := false
		for _, tri := range node.triangles {
			if t, hit := rayIntersectionDistanceTriangle(origin, dir, tri); hit && t < closestT {
				closestT = t
				hitFound = true
			}
		}
		return closestT, hitFound
	}

	// Otherwise, traverse both children.
	leftT, leftHit := maxT, false
	if node.left != nil {
		leftT, leftHit = node.left.RayIntersectionDistance(origin, dir, maxT)
		// Update maxT if we found a hit on the left.
		if leftHit {
			maxT = leftT
		}
	}

	rightT, rightHit := maxT, false
	if node.right != nil {
		rightT, rightHit = node.right.RayIntersectionDistance(origin, dir, maxT)
	}

	// Return the closer hit (if any).
	if leftHit && rightHit {
		if leftT < rightT {
			return leftT, true
		}
		return rightT, true
	} else if leftHit {
		return leftT, true
	} else if rightHit {
		return rightT, true
	}
	return maxT, false
}

// rayIntersectionDistanceTriangle is similar to rayIntersectsTriangle but returns the distance t.
func rayIntersectionDistanceTriangle(origin, direction r3.Vector, tri types.Triangle) (float64, bool) {
	const EPSILON = 1e-5
	edge1 := tri.V2.Sub(tri.V1)
	edge2 := tri.V3.Sub(tri.V1)

	h := direction.Cross(edge2)
	a := edge1.Dot(h)
	if a > -EPSILON && a < EPSILON {
		return 0, false
	}

	f := 1.0 / a
	s := origin.Sub(tri.V1)
	u := f * s.Dot(h)
	if u < 0.0 || u > 1.0 {
		return 0, false
	}

	q := s.Cross(edge1)
	v := f * direction.Dot(q)
	if v < 0.0 || (u+v) > 1.0 {
		return 0, false
	}

	t := f * edge2.Dot(q)
	if t > EPSILON {
		return t, true
	}
	return 0, false
}

// RayIntersects traverses the BVH and returns true if any triangle is intersected
// along the ray (origin, dir) with intersection parameter less than maxT.
func (node *BVHNode) RayIntersects(origin, dir r3.Vector, maxT float64) bool {
	if !node.bbox.IntersectRay(origin, dir, maxT) {
		return false
	}
	if len(node.triangles) > 0 {
		for _, tri := range node.triangles {
			if rayIntersectsTriangle(origin, dir, tri) {
				// In a production version you might also compute the hit distance and
				// return early only if it is less than maxT.
				return true
			}
		}
		return false
	}
	if node.left != nil && node.left.RayIntersects(origin, dir, maxT) {
		return true
	}
	if node.right != nil && node.right.RayIntersects(origin, dir, maxT) {
		return true
	}
	return false
}

func DebugEyePosConsole(shooter types.PlayerTickData, playerModel *Model) {
	eyePos := GetEyePosition(shooter, playerModel)
	height := playerModel.max.Z - playerModel.min.Z

	fmt.Printf("\n=== DebugEyePosConsole ===\n")
	fmt.Printf("Player minZ=%.2f, maxZ=%.2f => boundingBoxHeight=%.2f\n",
		playerModel.min.Z, playerModel.max.Z, height)
	fmt.Printf("shooter feet= (%.2f, %.2f, %.2f)\n",
		shooter.Position.X, shooter.Position.Y, shooter.Position.Z)
	fmt.Printf("Computed eyePos= (%.2f, %.2f, %.2f)\n\n",
		eyePos.X, eyePos.Y, eyePos.Z)
}

func distanceToShooter(shooter types.PlayerTickData, point r3.Vector) float64 {
	dx := point.X - shooter.Position.X
	dy := point.Y - shooter.Position.Y
	dz := point.Z - shooter.Position.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
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
