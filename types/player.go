package types

import (
	"fmt"
	"math"
	"time"

	"github.com/golang/geo/r3"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

type DamageDealt struct {
	ArmorDamage  int
	HealthDamage int
	HitGroup     byte
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
}

// ForwardVector computes the direction the player is looking in Source2 coords:
// (X=forward, Y=left, Z=up). We also fix the pitch range 270..360 => -90..0.
func (p *PlayerTickData) ForwardVector() r3.Vector {
	rawYaw := float64(p.ViewAngleX)   // 0..360
	rawPitch := float64(p.ViewAngleY) // 270..90 => remap >180 => negative

	if rawPitch > 180 {
		rawPitch -= 360
	}

	yaw := rawYaw * (math.Pi / 180)
	pitch := rawPitch * (math.Pi / 180)

	return r3.Vector{
		X: math.Cos(pitch) * math.Sin(yaw),
		Y: math.Cos(pitch) * math.Cos(yaw),
		Z: math.Sin(pitch),
	}.Normalize()
}

// IsInFieldOfView returns whether 'target' is within ~120° of the forward vector
func (p *PlayerTickData) IsInFieldOfViewFromEye(target, eyePos r3.Vector) bool {
	// Define the field-of-view in degrees (half FOV = FOV/2)
	const FOV_DEGREES = 120.0 // same as before
	toTarget := target.Sub(eyePos).Normalize()
	forward := p.ForwardVector() // still computed from p.ViewAngleX/Y
	dotProduct := forward.Dot(toTarget)
	angleRadians := math.Acos(dotProduct)
	angleDegrees := angleRadians * (180 / math.Pi)
	return angleDegrees <= (FOV_DEGREES / 2)
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
