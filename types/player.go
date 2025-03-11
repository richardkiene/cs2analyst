package types

import (
	"fmt"
	"math"
	"time"

	"github.com/golang/geo/r3"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

type DamageDealt struct {
	ArmorDamage     int
	HealthDamage    int
	HitGroup        byte
	Distance        float32   // Distance between attacker and victim
	NumPenetrations int       // Number of walls penetrated
	IsNoScope       bool      // Shot was made without scoping
	IsAttackerInAir bool      // Attacker was in the air
	IsThroughSmoke  bool      // Shot went through smoke
	BulletsFired    int       // Number of bullets fired in this exchange
	BulletsHit      int       // Number of bullets that hit
	AimPunchAngle   r3.Vector // Aim punch when damage was dealt
	DamageDirection r3.Vector // Direction the damage came from
}

type GrenadeData struct {
	Type             common.EquipmentType
	ProjectileID     int
	ThrownPosition   r3.Vector
	DetonatePosition r3.Vector
	Bounces          []r3.Vector // Positions where grenade bounced
	AffectedPlayers  []uint64    // Players affected by the grenade
	Damage           int         // Damage dealt by HE/Molotov
	FlashDuration    float32     // Duration of flash effect
	ThrownTick       int         // When it was thrown
	DetonateTick     int         // When it detonated/activated
	InnerRadius      float32     // Effective inner radius
	OuterRadius      float32     // Effective outer radius
}

type PlayerTickData struct {
	SteamID                uint64
	PlayerName             string
	PlayerTeam             common.Team
	Position               r3.Vector
	ViewAngleX             float32
	ViewAngleY             float32
	IsAlive                bool
	Velocity2D             float64
	Velocity3D             float64
	ActiveWeapon           *common.Equipment
	AmmoLeft               [32]int
	EntityID               int
	FlashedAtTick          int
	FlashedTimeRemaining   time.Duration
	Team                   common.Team
	IsAirborne             bool
	IsBlinded              bool
	IsCrouched             bool
	IsConnected            bool
	IsBot                  bool
	IsDefusing             bool
	IsPlanting             bool
	IsReloading            bool
	IsScoped               bool
	IsUpright              bool
	IsWalking              bool
	HasHelmet              bool
	HasKit                 bool
	FiredActiveWeapon      bool
	ArmorRemaining         int
	Assists                int
	Deaths                 int
	Kills                  int
	Health                 int
	Armor                  int
	Damage                 int
	UtilityDamage          int
	Money                  int
	CurrentRoundMoneySpent int
	CurrentMoneySpentTotal int
	DamageDealtToPlayer    map[uint64]DamageDealt
	DemoTime               time.Duration

	// Enhanced combat data
	BulletsFired        int
	BulletsHit          int
	HeadshotCount       int
	WallbangCount       int
	NoScopeCount        int
	BlindKills          int
	ThroughSmokeKills   int
	LastDamageDealtTick int
	LastDamageTakenTick int
	LastDeathTick       int
	LastKillTick        int
	KillStreak          int
	BlindShots          int
	JumpShots           int

	// Enhanced movement data
	LastJumpTick      int
	LastLandedTick    int
	JumpCount         int
	StutterStepCount  int
	CrouchCount       int
	WalkCount         int
	RunCount          int
	StopCount         int
	DirectionChanges  int
	LastMoveDirection r3.Vector

	// Enhanced utility data
	ActiveGrenades     map[int]*GrenadeData // Key is ProjectileID
	GrenadeHistory     []*GrenadeData
	FlashDuration      float32
	LastFlashTick      int
	IsInSmoke          bool
	IsInMolotov        bool
	SmokeBlockedCount  int // Times vision was blocked by smoke
	UtilityDamageDealt int // Damage dealt with utility
	UtilityDamageTaken int // Damage taken from utility
	UnusedUtilityValue int

	// Enhanced equipment data
	PrimaryWeapon   *common.Equipment
	SecondaryWeapon *common.Equipment
	GrenadeLoadout  []*common.Equipment
	LastBuyTick     int
	LastDropTick    int
	DroppedValue    int // Value of dropped equipment
	BuyHistory      []common.EquipmentType

	// Enhanced tactical data
	SpottedBy         []uint64  // SteamIDs of players who can see this player
	CanSee            []uint64  // SteamIDs of players this player can see
	AreaName          string    // Current map area name
	ExposedToAreas    []string  // Areas this player is exposed to
	NearestCover      r3.Vector // Nearest cover position
	DistanceToBomb    float32   // Distance to bomb/bombsite
	LastContactTick   int       // Last tick with enemy contact
	IsHoldingAngle    bool      // Player is holding an angle
	AngleHeldDuration int       // Ticks spent holding current angle

	// Enhanced sound data
	SoundEvents   []string // Types of sounds made this tick
	StepCount     int      // Number of steps taken
	LastStepTick  int
	IsMakingNoise bool
	NoiseRadius   float32

	// Economic data
	RoundStartMoney int
	RoundSpendMoney int
	SavedMoney      int
	LossBonus       int
	TeamEconomyType string // e.g., "Full Buy", "Eco", "Force Buy"

	// Round contribution
	RoundDamageDealt    int
	RoundUtilityDamage  int
	RoundEnemiesFlashed int
	RoundSupportScore   int // Score for supportive actions

	// Map geometry interaction
	VisibleMapAreas []int   // Areas of map currently visible
	CoverPercentage float32 // Percentage of body in cover
	PeekType        string  // Type of peek: "Wide", "Tight", "Shoulder"
	LastPeekTick    int
	PeekCount       int
}

// degToRad converts degrees to radians.
func degToRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}

// ForwardVector returns the forward direction in Source2 space.
// In Source2:
//   - X is forward (east)
//   - Y is left (north)
//   - Z is up
//
// And view angles are defined so that:
//   - ViewAngleX = 0° means facing north (+Y)
//   - ViewAngleX = 90° means facing east (+X)
//   - ViewAngleY = -90° means looking straight up (+Z)
//   - ViewAngleY = 90° means looking straight down (-Z)
/*func (p *PlayerTickData) ForwardVector() r3.Vector {
	yawRad := degToRad(float64(p.ViewAngleX))
	pitchRad := degToRad(float64(p.ViewAngleY))
	return r3.Vector{
		// When yaw = 0, sin(0)=0 so X = 0; cos(0)=1 so Y = 1.
		X: math.Sin(yawRad) * math.Cos(pitchRad), // East component
		Y: math.Cos(yawRad) * math.Cos(pitchRad), // North component
		Z: -math.Sin(pitchRad),                   // Up/Down component
	}.Normalize()
}*/

func (p *PlayerTickData) ForwardVector() r3.Vector {
	// Convert angles to radians
	pitchRad := degToRad(float64(p.ViewAngleY)) // This is X in the code (pitch)
	yawRad := degToRad(float64(p.ViewAngleX))   // This is Y in the code (yaw)

	// Following the Source engine's AngleVectors implementation
	return r3.Vector{
		X: math.Cos(pitchRad) * math.Cos(yawRad),
		Y: math.Cos(pitchRad) * math.Sin(yawRad),
		Z: -math.Sin(pitchRad),
	}.Normalize()
}

// IsInFieldOfViewFromEye returns whether 'target' is within CS2's actual FOV from eye position
func (p *PlayerTickData) IsInFieldOfViewFromEye(target, eyePos r3.Vector) bool {
	const (
		HORIZONTAL_FOV = 90.0
		VERTICAL_FOV   = 74.0
	)

	toTarget := target.Sub(eyePos).Normalize()
	forward := p.ForwardVector()

	// For nearly vertical views (looking straight up/down), we primarily care about the vertical angle
	pitch := float64(p.ViewAngleY)
	if pitch > 180 {
		pitch -= 360
	}

	// If we're looking nearly straight up/down (within 5 degrees of vertical)
	if math.Abs(pitch) > 85 {
		// Calculate angle between forward vector and target vector
		dot := forward.Dot(toTarget)
		if dot > 1.0 {
			dot = 1.0
		} else if dot < -1.0 {
			dot = -1.0
		}
		angle := math.Acos(dot) * (180 / math.Pi)
		return angle <= (VERTICAL_FOV / 2)
	}

	// For non-vertical views, continue with normal horizontal and vertical FOV checks
	// Calculate horizontal angle using X-Y plane
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

	horizontalDot := forwardHorizontal.Dot(toTargetHorizontal)
	if horizontalDot > 1.0 {
		horizontalDot = 1.0
	} else if horizontalDot < -1.0 {
		horizontalDot = -1.0
	}
	horizontalAngle := math.Acos(horizontalDot) * (180 / math.Pi)

	// Calculate vertical angle
	right := forward.Cross(r3.Vector{X: 0, Y: 0, Z: 1}).Normalize()
	projectedToTarget := toTarget.Sub(right.Mul(toTarget.Dot(right))).Normalize()

	verticalDot := forward.Dot(projectedToTarget)
	if verticalDot > 1.0 {
		verticalDot = 1.0
	} else if verticalDot < -1.0 {
		verticalDot = -1.0
	}
	verticalAngle := math.Acos(verticalDot) * (180 / math.Pi)

	return horizontalAngle <= (HORIZONTAL_FOV/2) && verticalAngle <= (VERTICAL_FOV/2)
}

func (p *PlayerTickData) IsPartiallyVisible(target, eyePos r3.Vector, slackDegrees float64) bool {
	// Our base FOV remains the same (e.g., 120°)
	const baseFOV = 120.0
	// Effective half FOV plus extra slack
	effectiveThreshold := (baseFOV / 2.0) + slackDegrees

	// Compute the vector from the shooter’s position to the target.
	// (If you want to use eye position instead, replace p.Position with the computed eyePos.)
	toTarget := target.Sub(eyePos).Normalize()
	forward := p.ForwardVector()

	dot := forward.Dot(toTarget)
	angleDegrees := math.Acos(dot) * (180 / math.Pi)

	return angleDegrees <= effectiveThreshold
}

func (p PlayerTickData) String() string {
	var team string
	if p.PlayerTeam == 2 {
		team = "Terrorists"
	} else if p.PlayerTeam == 3 {
		team = "Counter-Terrorists"
	} else {
		team = "Unknown"
	}

	return fmt.Sprintf(
		"PlayerTickData{SteamID: %d, Name: %s, Team: %s, Pos: (%.2f, %.2f, %.2f), ViewAngle: (%.2f, %.2f), Alive: %t, Vel2D: %.2f, Vel3D: %.2f}",
		p.SteamID, p.PlayerName, team,
		p.Position.X, p.Position.Y, p.Position.Z,
		p.ViewAngleX, p.ViewAngleY, p.IsAlive,
		p.Velocity2D, p.Velocity3D,
	)
}

type PlayerStats struct {
	Name       string
	SteamID    uint64
	Kills      int
	Deaths     int
	Assists    int
	Headshots  int
	ShotsTotal int
	HitsTotal  int
	IsAlive    bool
	Team       int

	// Aim metrics
	EnemySpottedShots   int
	SprayShots          int
	SprayHits           int
	CounterStrafedShots int
	TimeToFirstDamage   []float64
	TimeToFirstShot     []float64
	FirstBulletHits     int
	FirstBulletShots    int
	CrosshairPlacement  []float64
	MedianTTD           float64
	MovingRifleShots    int // Tracks only rifle shots

	// Trade metrics
	TradeKills               int
	TradeKillOpportunities   int
	TradeKillAttempts        int
	TradedDeaths             int
	TradedDeathOpportunities int
	TradedDeathAttempts      int
	TimesTraded              int

	// Round metrics
	RoundsSurvived  int
	RoundsActive    int
	SurvivalByPhase map[string]int
	OpeningDuels    int
	OpeningDuelsWon int
	ClutchAttempts  int
	ClutchesWon     int

	// Multi-kill tracking
	TwoKills   int
	ThreeKills int
	FourKills  int
	FiveKills  int

	// Utility metrics
	UtilityStats       UtilityStats
	UnusedUtilityValue int
	TeamUtilityDamage  int
	TotalDamage        int

	// Movement tracking
	Velocity map[string]float64

	WeaponStats       map[string]*WeaponStats
	MapAreaKills      map[string]int
	MapAreaDeaths     map[string]int
	FlashAssists      int
	PeekKills         int
	PeekDeaths        int
	SprayTransfers    int
	ReactionTimeTotal float64
	ReactionTimeCount int
	EquipmentValue    int
	MoneySpent        int
	SiteHolds         int
	EntryKills        int
	EntryAttempts     int
	ForceBuyKills     int
	ForceBuyDeaths    int
}

type UtilityStats struct {
	HEGrenadesThrown   int
	HEDamage           int
	FlashesThrown      int
	MolotovsThrown     int
	SmokesThrown       int
	EnemiesFlashed     int
	TeammatesFlashed   int
	FlashAssists       int
	TotalBlindDuration float64
}

type WeaponStats struct {
	Kills     int
	Deaths    int
	Shots     int
	Hits      int
	Headshots int
	Damage    int
}
