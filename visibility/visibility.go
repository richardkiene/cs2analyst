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

// Fixed FindLastContinuousVisibilityStart function to properly handle continuous visibility windows
func (v *Visibility) FindLastContinuousVisibilityStart(playerID, targetID uint64, currentTick int, perTickInfo map[int]map[uint64]types.PlayerTickData) (VisibilityResult, bool) {
	const maxWindowTicks = 320
	var allowedGap = 64 // maximum number of consecutive ticks where visibility can be missing

	// Only log debug messages if the shooter matches the specified SteamID.
	debugEnabled := playerID == 76561199002420143 && targetID == 76561198237889474

	if debugEnabled {
		slog.Info("FindLastContinuousVisibilityStart called",
			"playerID", playerID,
			"targetID", targetID,
			"currentTick", currentTick,
			"maxWindowTicks", maxWindowTicks,
			"allowedGap", allowedGap)
	}

	// Start from the damage tick minus 1 to avoid the immediate frame
	startSearchTick := currentTick - 1

	// Track the earliest visible tick in the current window
	earliestVisibleTick := -1
	windowStartTick := -1
	var windowStartShooterPos, windowStartVictimPos r3.Vector
	var windowStartTime time.Duration

	// Track the current visibility window state
	gapCount := 0
	lastVisibleTick := -1
	inVisibilityWindow := false

	// Iterate backwards from startSearchTick, but only up to maxWindowTicks
	for tick := startSearchTick; tick >= 0 && (currentTick-tick) <= maxWindowTicks; tick-- {
		if debugEnabled {
			slog.Info("Processing tick",
				"tick", tick,
				"gapCount", gapCount,
				"lastVisibleTick", lastVisibleTick,
				"earliestVisibleTick", earliestVisibleTick)
		}

		playerData, ok := perTickInfo[tick]
		if !ok {
			gapCount++
			if debugEnabled {
				slog.Debug("No player data for tick", "tick", tick, "gapCount", gapCount)
			}
			if gapCount > allowedGap {
				// If we've exceeded the allowed gap and we were in a visibility window,
				// we've found our window's start
				break
			}
			continue
		}

		shooterTick, ok := playerData[playerID]
		// If the shooter is blind, don't count it as a gap in visibility
		if ok && shooterTick.IsBlinded {
			if debugEnabled {
				slog.Info("Shooter is blinded",
					"tick", tick,
					"flashDuration", shooterTick.FlashDuration)
			}
			// This isn't a valid gap since the shooter is blind
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
		canSeeTarget := IsShooterPointingAtTarget(shooterTick, targetTick, *v.LosSystem.PlayerModel, *v.LosSystem.PlayerModel, *v.LosSystem.MapModel)

		if debugEnabled {
			// Use the shared utility functions for consistency
			coords := NewDefaultSource2Coordinates()

			// Use shared GetEyePosition instead of custom transformation
			eyePos := GetEyePosition(shooterTick)
			transformedEyePos := coords.CS2ToModelSpace(eyePos)

			// Transform positions consistently
			transformedShooterPos := coords.CS2ToModelSpace(shooterTick.Position)
			transformedTargetPos := coords.CS2ToModelSpace(targetTick.Position)

			slog.Info("Visibility check",
				"tick", tick,
				"canSeeTarget", canSeeTarget,
				"gapCount", gapCount,
				"lastVisibleTick", lastVisibleTick,
				"originalShooterPos", shooterTick.Position,
				"transformedShooterPos", transformedShooterPos,
				"eyePos", transformedEyePos,
				"originalTargetPos", targetTick.Position,
				"transformedTargetPos", transformedTargetPos)
		}

		if canSeeTarget {
			// If we weren't in a visibility window or if we have a gap larger than allowedGap
			if !inVisibilityWindow {
				if debugEnabled {
					slog.Info("Starting new visibility window",
						"tick", tick,
						"lastVisibleTick", lastVisibleTick)
				}

				// Mark the start of a new visibility window
				inVisibilityWindow = true
				windowStartTick = tick // This is the start of the window (earliest tick so far)
				earliestVisibleTick = tick
				windowStartShooterPos = shooterTick.Position
				windowStartVictimPos = targetTick.Position
				windowStartTime = shooterTick.DemoTime
			} else {
				// We're continuing an existing visibility window
				// Update the earliest tick in this window
				earliestVisibleTick = tick
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

			// If we've exceeded the allowed gap and we were in a visibility window
			if gapCount > allowedGap && inVisibilityWindow {
				// We've found our window's start, so we can stop searching
				break
			}
		}
	}

	// If we found a visibility window, return the results
	if inVisibilityWindow && earliestVisibleTick != -1 {
		if debugEnabled {
			slog.Info("About to return visibility result",
				"playerID", playerID,
				"targetID", targetID,
				"earliestVisibleTick", earliestVisibleTick,
				"windowStartTick", windowStartTick,
				"lastVisibleTick", lastVisibleTick,
				"currentTick", currentTick)
		}

		return VisibilityResult{
			StartTick:  earliestVisibleTick, // Return the earliest tick in the window
			StartTime:  windowStartTime,
			IsValid:    true,
			ShooterPos: windowStartShooterPos,
			VictimPos:  windowStartVictimPos,
		}, true
	}

	if debugEnabled {
		slog.Debug("No continuous visibility found")
	}

	return VisibilityResult{}, false
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
	eyePos := GetEyePosition(shooter)
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
