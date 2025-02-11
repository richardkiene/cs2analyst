package visibility

import (
	"bufio"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

const maxTrianglesPerLeaf = 8

type AABB struct {
	Min, Max r3.Vector
}

// Used as a cache lookup key
type gridKey struct {
	x, y, z int
}

type BVHNode struct {
	bbox      AABB
	left      *BVHNode
	right     *BVHNode
	triangles []types.Triangle // non-nil for leaf nodes
}

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

type VisibilityResult struct {
	StartTick  int
	StartTime  time.Duration
	ShooterPos r3.Vector
	VictimPos  r3.Vector
	IsValid    bool
}

type Model struct {
	triangles        []types.Triangle
	min, max         r3.Vector
	sectors          map[gridKey][]types.Triangle
	gridSize         float64
	hitboxes         []Hitbox
	visibilityPoints []r3.Vector
	bvh              *BVHNode
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
		sectors:  make(map[gridKey][]types.Triangle),
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
				key := gridKey{x, y, z}
				m.sectors[key] = append(m.sectors[key], t)
			}
		}
	}
}

// GetRelevantMapGeometry returns triangles along the line from start->end
func (m *Model) GetRelevantMapGeometry(start, end r3.Vector) []types.Triangle {
	visited := make(map[gridKey]bool, 128)
	var relevant []types.Triangle

	// Compute the direction and length.
	dir := end.Sub(start)
	length := dir.Norm()
	if length == 0 {
		key := gridKey{
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
					key := gridKey{
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
					key := gridKey{cx + dx, cy + dy, cz + dz}
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

func (v *Visibility) FindLastContinuousVisibilityStart(playerID, targetID uint64, currentTick int, perTickInfo map[int]map[uint64]types.PlayerTickData) (VisibilityResult, bool) {
	const maxWindowTicks = 320
	const allowedGap = 8 // maximum number of consecutive ticks where visibility can be missing
	var candidateTick = -1
	var candidateTime time.Duration
	var candidateShooterPos, candidateVictimPos r3.Vector

	gapCount := 0
	// Start at currentTick and move backwards, but only up to maxWindowTicks
	for tick := currentTick; tick >= 0 && (currentTick-tick) <= maxWindowTicks; tick-- {
		playerData, ok := perTickInfo[tick]
		if !ok {
			gapCount++
			if gapCount > allowedGap {
				break
			}
			continue
		}
		shooterTick, ok := playerData[playerID]
		if !ok || !shooterTick.IsAlive || shooterTick.IsBlinded {
			gapCount++
			if gapCount > allowedGap {
				break
			}
			continue
		}
		targetTick, ok := playerData[targetID]
		if !ok || !targetTick.IsAlive {
			gapCount++
			if gapCount > allowedGap {
				break
			}
			continue
		}
		if CanSeeTarget(shooterTick, targetTick, v.LosSystem.playerModel, v.LosSystem.mapModel, -1) {
			// Found a visible tick—update candidate and reset gap counter
			candidateTick = tick
			candidateTime = shooterTick.DemoTime
			candidateShooterPos = shooterTick.Position
			candidateVictimPos = targetTick.Position
			gapCount = 0
		} else {
			gapCount++
			if gapCount > allowedGap {
				break
			}
		}
	}

	if candidateTick != -1 {
		return VisibilityResult{
			StartTick:  candidateTick,
			StartTime:  candidateTime,
			IsValid:    true,
			ShooterPos: candidateShooterPos,
			VictimPos:  candidateVictimPos,
		}, true
	}
	return VisibilityResult{}, false
}

func (m *Model) GetVisibilityPoints() []r3.Vector {
	if m.visibilityPoints != nil {
		return m.visibilityPoints
	}

	var points []r3.Vector
	for _, hitbox := range m.hitboxes {
		center := r3.Vector{
			X: (hitbox.MinBounds.X + hitbox.MaxBounds.X) / 2,
			Y: (hitbox.MinBounds.Y + hitbox.MaxBounds.Y) / 2,
			Z: (hitbox.MinBounds.Z + hitbox.MaxBounds.Z) / 2,
		}

		points = append(points, center)

		switch hitbox.Type {
		case "Box":
			points = append(points,
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MaxBounds.Z},
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MaxBounds.Z},
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MinBounds.Y, Z: hitbox.MinBounds.Z},
				r3.Vector{X: hitbox.MaxBounds.X, Y: hitbox.MaxBounds.Y, Z: hitbox.MinBounds.Z},
			)
		case "Sphere":
			radius := (hitbox.MaxBounds.Sub(hitbox.MinBounds)).Norm() / 2
			points = append(points,
				r3.Vector{X: center.X + radius, Y: center.Y, Z: center.Z},
				r3.Vector{X: center.X, Y: center.Y + radius, Z: center.Z},
				r3.Vector{X: center.X, Y: center.Y, Z: center.Z + radius},
			)
		case "Capsule":
			height := hitbox.MaxBounds.Z - hitbox.MinBounds.Z
			radius := (hitbox.MaxBounds.X - hitbox.MinBounds.X) / 2
			points = append(points,
				r3.Vector{X: center.X + radius, Y: center.Y, Z: center.Z},
				r3.Vector{X: center.X, Y: center.Y, Z: center.Z + height/4},
				r3.Vector{X: center.X, Y: center.Y, Z: center.Z - height/4},
			)
		}
	}

	m.visibilityPoints = points
	return points
}

// CanSeeTarget checks if 'shooter' can see 'target' using line-of-sight from the shooter's eye
func CanSeeTarget(shooter, target types.PlayerTickData, playerModel, mapModel *Model, tick int) bool {
	// 1) Compute the shooter’s eye position.
	eyeHeight := playerModel.max.Z * 0.85
	if shooter.IsCrouched {
		eyeHeight *= 0.75
	}
	eyePos := r3.Vector{
		X: shooter.Position.X,
		Y: shooter.Position.Y,
		Z: shooter.Position.Z + eyeHeight,
	}

	// 2) Use a relaxed FOV threshold.
	effectiveHalfFOV := 100.0
	cosThreshold := math.Cos(effectiveHalfFOV * math.Pi / 180.0)

	// 3) Get candidate visibility points.
	points := playerModel.GetVisibilityPoints()
	forward := shooter.ForwardVector()

	// Check if any candidate is roughly in the shooter’s FOV.
	anyInFOV := false
	for _, bp := range points {
		wp := target.Position.Add(bp)
		toTarget := wp.Sub(eyePos).Normalize()
		if forward.Dot(toTarget) >= cosThreshold {
			anyInFOV = true
			break
		}
	}
	if !anyInFOV {
		return false
	}

	// Initialize our LOS aggregator for the shooter of interest.
	// TODO: Remove this debug code
	var losStats *LosStats
	shooterOfInterest := uint64(76561197991944713)
	if shooter.SteamID == shooterOfInterest {
		losStats = NewLosStats()
	}

	// 4) For each candidate that passes the FOV test, do a detailed LOS test.
	for _, bp := range points {
		wp := target.Position.Add(bp)
		toTarget := wp.Sub(eyePos).Normalize()
		if forward.Dot(toTarget) < cosThreshold {
			continue // Skip candidates outside the FOV.
		}

		// Compute ray direction and distance.
		rayDir := wp.Sub(eyePos).Normalize()
		distToCandidate := wp.Sub(eyePos).Norm()

		// 5) Compute a tolerance (5% of the candidate distance).
		//tolerance := distToCandidate * 0.05
		tolerance := distToCandidate * 0.10

		// 6) Query the BVH for the nearest intersection distance.
		hitT, hitFound := mapModel.bvh.RayIntersectionDistance(eyePos, rayDir, math.MaxFloat64)

		// Calculate the delta.
		delta := distToCandidate - (hitT + tolerance)

		// If this is our shooter of interest, update the aggregate stats.
		if shooter.SteamID == shooterOfInterest {
			// Consider a candidate borderline if delta is between 0 and 0.05 * distToCandidate.
			// (You can adjust this fraction as needed.)
			borderlineThreshold := distToCandidate * 0.05
			losStats.UpdateLosStats(delta, borderlineThreshold)
		}

		// 7) Log detailed candidate info for debugging. (Keep it to only a single shooter because this output is huge for all shooters ~50GB)
		/*if shooter.SteamID == 76561197991944713 {
			slog.Debug("LOS candidate test",
				"shooter", shooter.SteamID,
				"shooter position X", shooter.Position.X,
				"shooter position Y", shooter.Position.Y,
				"target", target.SteamID,
				"target position X", target.Position.X,
				"target position Y", target.Position.Y,
				"eyePos", eyePos,
				"candidatePoint", wp,
				"distToCandidate", distToCandidate,
				"hitFound", hitFound,
				"hitT", hitT,
				"tolerance", tolerance)
		}*/

		// 8) Decision: if a hit is found and occurs significantly before the candidate point, this candidate is blocked.
		if hitFound && (hitT+tolerance) < distToCandidate {
			// Candidate is blocked; try the next candidate.
			continue
		} else {
			// Either no hit was found or the hit is very near or beyond the candidate.
			return true
		}
	}

	// If we're processing our shooter of interest, output the aggregated stats.
	if shooter.SteamID == shooterOfInterest && losStats != nil {
		slog.Info("LOS aggregate stats for shooter", "stats", losStats.Summary())
	}

	return false
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

func DebugEyePosConsole(
	shooter types.PlayerTickData,
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

func distanceToShooter(shooter types.PlayerTickData, point r3.Vector) float64 {
	dx := point.X - shooter.Position.X
	dy := point.Y - shooter.Position.Y
	dz := point.Z - shooter.Position.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func isInFOV(shooter types.PlayerTickData, point r3.Vector, fovDegrees float64) bool {
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
