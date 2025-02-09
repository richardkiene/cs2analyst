package collector

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/golang/geo/r3"
	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/msgs2"
	"github.com/richardkiene/cs2analyst/types"
)

type Collector struct {
	TickRate     float64
	TickTime     time.Duration
	mapNameFound bool
	Match        *types.Match
	parser       dem.Parser
	Logger       slog.Logger
	PerTickInfo  map[int]map[uint64]PlayerTickData
}

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

func New() *Collector {
	return &Collector{
		Match:       NewMatch(),
		Logger:      *slog.Default(),
		TickRate:    -1,
		TickTime:    -1,
		PerTickInfo: make(map[int]map[uint64]PlayerTickData, 0),
	}
}

func NewMatch() *types.Match {
	return &types.Match{
		Events:      make([]types.Event, 0),
		PlayerStats: make(map[uint64]*types.PlayerStats),
		Date:        time.Now().Format("1999-12-31"),
	}
}

func (p *PlayerTickData) ForwardVector() r3.Vector {
	rawYaw := float64(p.ViewAngleX)   // 0..360
	rawPitch := float64(p.ViewAngleY) // 270..90

	// Normalize pitch from 270..360 to -90..0
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

func (p *PlayerTickData) IsInFieldOfView(target r3.Vector) bool {
	const FOV_DEGREES = 120.0 // CS2's typical FOV is 90 but we're relaxing it

	toTarget := r3.Vector{
		X: target.X - p.Position.X,
		Y: target.Y - p.Position.Y,
		Z: target.Z - p.Position.Z,
	}.Normalize()

	forward := p.ForwardVector()
	dotProduct := forward.X*toTarget.X + forward.Y*toTarget.Y + forward.Z*toTarget.Z
	angleRadians := math.Acos(dotProduct)
	angleDegrees := angleRadians * (180 / math.Pi)

	return angleDegrees <= FOV_DEGREES/2
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

	return fmt.Sprintf("PlayerTickData{SteamID: %d, Name: %s, Team: %s, Pos: (%.2f, %.2f, %.2f), ViewAngle: (%.2f, %.2f), Alive: %t, Vel2D: %.2f, Vel3D: %.2f}",
		p.SteamID, p.PlayerName, team,
		p.Position.X, p.Position.Y, p.Position.Z,
		p.ViewAngleX, p.ViewAngleY, p.IsAlive,
		p.Velocity2D, p.Velocity3D,
	)
}

func (c *Collector) Collect(demoPath string) (*types.Match, error) {
	f, err := os.Open(demoPath)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	c.parser = dem.NewParser(f)
	defer c.parser.Close()

	c.registerEventHandlers()

	// Parse frames until we handle a ServerInfo event with the map name available
	// https://github.com/markus-wa/demoinfocs-golang/issues/435#issuecomment-2613140840
	for !c.mapNameFound {
		moreFrames, err := c.parser.ParseNextFrame()
		if err != nil || !moreFrames {
			if err == dem.ErrUnexpectedEndOfDemo {
				return nil, fmt.Errorf("unable to determine map name from demo file")
			}
			return nil, fmt.Errorf("error during initial parsing: %v", err)
		}
	}

	c.Logger.Info("Map name detected: ", "MapName", c.Match.MapName)

	c.Logger.Debug("Resuming full parsing...")

	if err := c.parser.ParseToEnd(); err != nil {
		return nil, fmt.Errorf("error during full parsing: %v", err)
	}

	c.Logger.Debug("Finished parsing events", "Events", len(c.Match.Events))

	for steamID, stats := range c.Match.PlayerStats {
		c.Logger.Debug("Player stats",
			"name", stats.Name,
			"steam_id", steamID,
			"kills", stats.Kills,
			"deaths", stats.Deaths,
			"assists", stats.Assists,
			"total_damage", stats.TotalDamage,
		)
	}

	return c.Match, nil
}

func (c *Collector) registerEventHandlers() {
	c.parser.RegisterNetMessageHandler(c.handleServerInfo)
	c.parser.RegisterNetMessageHandler(c.handleEntityUpdate)
	c.parser.RegisterEventHandler(c.handleWeaponFire)
	c.parser.RegisterEventHandler(c.handlePlayerHurt)
}

func (c *Collector) calculateVelocity3D(currentPos, lastPos r3.Vector) float64 {
	timeDelta := float64(c.TickTime.Milliseconds())

	displacement := currentPos.Sub(lastPos)
	return math.Sqrt(displacement.X*displacement.X+
		displacement.Y*displacement.Y+
		displacement.Z*displacement.Z) / timeDelta
}

func (c *Collector) calculateVelocity2D(currentPos, lastPos r3.Vector) float64 {
	timeDelta := float64(c.TickTime.Milliseconds())

	displacement := currentPos.Sub(lastPos)
	return math.Sqrt(displacement.X*displacement.X+
		displacement.Y*displacement.Y) / timeDelta
}

func (c *Collector) handleServerInfo(msg *msgs2.CSVCMsg_ServerInfo) {
	mapName := msg.GetMapName()
	if !c.mapNameFound && mapName != "" {
		c.Match.MapName = mapName
		c.mapNameFound = true
		c.Logger.Debug("Map name detected from server info", "MapName", c.Match.MapName)
	}
}

func (c *Collector) handleEntityUpdate(msg *msgs2.CSVCMsg_PacketEntities) {
	if c.TickRate == -1 {
		c.TickRate = c.parser.TickRate()
		c.TickTime = c.parser.TickTime()

		c.Logger.Debug("Server Tickrate", "tickRate", c.TickRate)
		c.Logger.Debug("Tick time", "tickTime", c.TickTime)
	}

	gs := c.parser.GameState()
	currentTick := gs.IngameTick()

	// At the beginning of the demo, IngameTick can be invalid
	if currentTick < 0 {
		return
	}

	if c.PerTickInfo[currentTick] == nil {
		c.PerTickInfo[currentTick] = make(map[uint64]PlayerTickData)
	}

	prevTick := currentTick - 1
	var previousTickMap map[uint64]PlayerTickData
	if prevTick >= 0 {
		previousTickMap = c.PerTickInfo[prevTick]
	}

	for _, player := range gs.Participants().Playing() {
		if player.SteamID64 == 0 {
			continue
		}

		var lastPlayerTick PlayerTickData
		if previousTickMap != nil {
			if ptd, ok := previousTickMap[player.SteamID64]; ok {
				lastPlayerTick = ptd
			}
		}

		var velocity2D, velocity3D float64

		// We only want to calculate the velocity of a player if they are alive
		if player.IsAlive() {
			velocity2D = c.calculateVelocity2D(player.Position(), lastPlayerTick.Position)
			velocity3D = c.calculateVelocity3D(player.Position(), lastPlayerTick.Position)
		}

		playerTick, found := c.PerTickInfo[currentTick][player.SteamID64]
		if !found {
			// Only create a new struct if this is the first time we see this player on this tick
			playerTick = PlayerTickData{
				SteamID:             player.SteamID64,
				DamageDealtToPlayer: make(map[uint64]DamageDealt),
			}
		}

		playerTick.SteamID = player.SteamID64
		playerTick.PlayerTeam = player.Team
		playerTick.PlayerName = player.Name
		playerTick.Position = player.Position()
		playerTick.ViewAngleX = player.ViewDirectionX()
		playerTick.ViewAngleY = player.ViewDirectionY()
		playerTick.IsAlive = player.IsAlive()
		playerTick.Velocity2D = velocity2D
		playerTick.Velocity3D = velocity3D
		playerTick.ActiveWeapon = player.ActiveWeapon()
		playerTick.AmmoLeft = player.AmmoLeft
		playerTick.EntityID = player.Entity.ID()
		playerTick.FlashedAtTick = player.FlashTick
		playerTick.FlashedTimeRemaining = player.FlashDurationTimeRemaining()
		playerTick.Team = player.Team
		playerTick.IsConnected = player.IsConnected
		playerTick.IsAirborne = player.IsAirborne()
		playerTick.IsBlinded = player.IsBlinded()
		playerTick.IsBot = player.IsBot
		playerTick.IsCrouched = player.IsDucking()
		playerTick.IsDefusing = player.IsDefusing
		playerTick.IsPlanting = player.IsPlanting
		playerTick.IsReloading = player.IsReloading
		playerTick.IsScoped = player.IsScoped()
		playerTick.IsUpright = player.IsStanding()
		playerTick.IsWalking = player.IsWalking()
		playerTick.Assists = player.Assists()
		playerTick.Deaths = player.Deaths()
		playerTick.Kills = player.Kills()
		playerTick.Damage = player.TotalDamage()
		playerTick.Health = player.Health()
		playerTick.Armor = player.Armor()
		playerTick.UtilityDamage = player.UtilityDamage()
		playerTick.Money = player.Money()
		playerTick.CurrentRoundMoneySpent = player.MoneySpentThisRound()
		playerTick.CurrentMoneySpentTotal = player.MoneySpentTotal()

		c.PerTickInfo[currentTick][player.SteamID64] = playerTick
	}
}

func (c *Collector) handleWeaponFire(e events.WeaponFire) {
	if e.Shooter == nil {
		return
	}

	currentTick := c.parser.GameState().IngameTick()

	if _, exists := c.PerTickInfo[currentTick][e.Shooter.SteamID64]; !exists {
		c.PerTickInfo[currentTick] = make(map[uint64]PlayerTickData)
	}

	shooterData := c.PerTickInfo[currentTick][e.Shooter.SteamID64]
	shooterData.FiredActiveWeapon = true
	c.PerTickInfo[currentTick][e.Shooter.SteamID64] = shooterData
}

func (c *Collector) handlePlayerHurt(e events.PlayerHurt) {
	gs := c.parser.GameState()
	currentTick := gs.IngameTick()

	if e.Attacker == nil || e.Player == nil {
		return
	}

	attackerID := e.Attacker.SteamID64
	victimID := e.Player.SteamID64

	if _, exists := c.PerTickInfo[currentTick][attackerID]; !exists {
		c.PerTickInfo[currentTick] = make(map[uint64]PlayerTickData)
	}

	if _, exists := c.PerTickInfo[currentTick][victimID]; !exists {
		c.PerTickInfo[currentTick] = make(map[uint64]PlayerTickData)
	}

	attackerData := c.PerTickInfo[currentTick][attackerID]
	victimData := c.PerTickInfo[currentTick][victimID]

	if attackerData.DamageDealtToPlayer == nil {
		attackerData.DamageDealtToPlayer = make(map[uint64]DamageDealt)
	}

	currentDamage := DamageDealt{
		ArmorDamage:  e.ArmorDamageTaken,
		HealthDamage: e.HealthDamageTaken,
		HitGroup:     byte(e.HitGroup),
	}

	attackerData.DamageDealtToPlayer[victimID] = currentDamage

	c.PerTickInfo[currentTick][attackerID] = attackerData
	c.PerTickInfo[currentTick][victimID] = victimData
}
