package types

import (
	"github.com/golang/geo/r3"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

type Match struct {
	MapName     string
	TickRate    float32
	Events      []Event
	Date        string
	PlayerStats map[uint64]*PlayerStats
}

type Event struct {
	Type string
	Data map[string]interface{}
}

// UtilityEvent represents a utility (grenade) usage event
type UtilityEvent struct {
	Type        common.EquipmentType // Type of utility (flash, smoke, etc.)
	Position    r3.Vector            // Position where utility was used
	Impact      float64              // Effectiveness score of the utility usage
	Timestamp   int                  // Tick when the utility was used
	PlayerID    uint64               // SteamID of the player who used the utility
	TeamSide    common.Team          // Team side of the player
	AreaDenied  string               // Map area denied/affected by utility
	Duration    float64              // Duration of effect in seconds
	PlayersHit  []uint64             // SteamIDs of players affected
	DamageDealt int                  // Total damage dealt by utility (for HE/Molotov)
	IsEffective bool                 // Whether the utility achieved its intended purpose
}

// CombatEvent represents a combat interaction between players
type CombatEvent struct {
	Tick               int                  // Game tick when the event occurred
	AttackerID         uint64               // SteamID of the attacking player
	VictimID           uint64               // SteamID of the player being attacked
	WeaponType         common.EquipmentType // Weapon used
	DamageDealt        int                  // Amount of damage dealt
	IsHeadshot         bool                 // Whether it was a headshot
	IsWallbang         bool                 // Whether the shot went through a wall
	IsThroughSmoke     bool                 // Whether the shot went through smoke
	Distance           float64              // Distance between attacker and victim
	TimeToDamage       float64              // Time between first sight and damage
	CrosshairPlacement float64              // Quality of crosshair placement (0-1)
	AttackerPos        r3.Vector            // Position of the attacker
	VictimPos          r3.Vector            // Position of the victim
	PenetrationCount   int                  // Number of surfaces penetrated
	HitGroup           byte                 // Body part hit
	InaccuracyFactor   float64              // Weapon inaccuracy at time of shot
	IsTradeKill        bool                 // Whether this was a trade kill
	IsEco              bool                 // Whether this occurred during an eco round
}

// EconomyState represents the economic state of a round
type EconomyState struct {
	Round           int                    // Current round number
	TeamMoney       int                    // Total team money
	PlayerMoney     int                    // Individual player money
	EnemyEconomy    string                 // Classification of enemy economy (e.g., "Full Buy", "Eco")
	BuyDecision     string                 // Actual buy decision made
	OptimalBuy      string                 // Calculated optimal buy decision
	LossBonus       int                    // Current loss bonus level (0-4)
	TeamEquipValue  int                    // Total value of team's equipment
	EnemyEquipValue int                    // Estimated value of enemy team's equipment
	SavedGuns       []common.EquipmentType // Weapons saved from previous round
	InvestmentMade  int                    // Amount spent in current round
	RoundStartMoney int                    // Money at start of round
	RoundEndMoney   int                    // Money at end of round
	BuyRequests     []BuyRequest           // Team buy requests for the round
	StrategyType    string                 // Economic strategy (e.g., "Full Save", "Force Buy")
	IsForceRound    bool                   // Whether this is a force buy round
	IsEcoRound      bool                   // Whether this is an eco round
}

// BuyRequest represents a player's buy request for the round
type BuyRequest struct {
	PlayerID       uint64
	RequestedItems []common.EquipmentType
	Priority       int    // Priority level of the request
	MoneyAvailable int    // Money available for this request
	Role           string // Player's role/position
}

type MapState struct {
	AreaName     string
	Angles       []string
	CoverPoints  []r3.Vector
	ClearPoints  []r3.Vector
	Obstructions []r3.Vector
}
