package types

import (
	"time"

	"github.com/golang/geo/r3"
	"github.com/markus-wa/demoinfocs-golang/common"
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
