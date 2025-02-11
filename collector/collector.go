package collector

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/golang/geo/r3"
	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
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
	PerTickInfo  map[int]map[uint64]types.PlayerTickData
}

func New() *Collector {
	return &Collector{
		Match:       NewMatch(),
		Logger:      *slog.Default(),
		TickRate:    -1,
		TickTime:    -1,
		PerTickInfo: make(map[int]map[uint64]types.PlayerTickData, 0),
	}
}

func NewMatch() *types.Match {
	return &types.Match{
		Events:      make([]types.Event, 0),
		PlayerStats: make(map[uint64]*types.PlayerStats),
		Date:        time.Now().Format("1999-12-31"),
	}
}

// Collect runs the entire demo parse
func (c *Collector) Collect(demoPath string) (*types.Match, error) {
	f, err := os.Open(demoPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c.parser = dem.NewParser(f)
	defer c.parser.Close()

	c.registerEventHandlers()

	// parse until we detect map name
	for !c.mapNameFound {
		moreFrames, err := c.parser.ParseNextFrame()
		if err != nil || !moreFrames {
			if err == dem.ErrUnexpectedEndOfDemo {
				return nil, fmt.Errorf("unable to determine map name")
			}
			return nil, fmt.Errorf("error during initial parsing: %v", err)
		}
	}
	c.Logger.Info("Map name detected: ", "MapName", c.Match.MapName)

	c.Logger.Debug("Resuming full parsing...")

	if err := c.parser.ParseToEnd(); err != nil {
		return nil, fmt.Errorf("error parsing to end: %v", err)
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
	disp := currentPos.Sub(lastPos)
	return disp.Norm() / timeDelta
}

func (c *Collector) calculateVelocity2D(currentPos, lastPos r3.Vector) float64 {
	timeDelta := float64(c.TickTime.Milliseconds())
	disp := currentPos.Sub(lastPos)
	return math.Sqrt(disp.X*disp.X+disp.Y*disp.Y) / timeDelta
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
	if currentTick < 0 {
		return
	}

	if c.PerTickInfo[currentTick] == nil {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	prevTick := currentTick - 1
	var prevData map[uint64]types.PlayerTickData
	if prevTick >= 0 {
		prevData = c.PerTickInfo[prevTick]
	}

	for _, player := range gs.Participants().Playing() {
		if player.SteamID64 == 0 {
			continue
		}

		var lastPTD types.PlayerTickData
		if prevData != nil {
			if ptd, ok := prevData[player.SteamID64]; ok {
				lastPTD = ptd
			}
		}

		var vel2D, vel3D float64
		if player.IsAlive() {
			vel2D = c.calculateVelocity2D(player.Position(), lastPTD.Position)
			vel3D = c.calculateVelocity3D(player.Position(), lastPTD.Position)
		}

		pTick, found := c.PerTickInfo[currentTick][player.SteamID64]
		if !found {
			pTick = types.PlayerTickData{
				SteamID:             player.SteamID64,
				DamageDealtToPlayer: make(map[uint64]types.DamageDealt),
			}
		}

		pTick.SteamID = player.SteamID64
		pTick.PlayerTeam = player.Team
		pTick.PlayerName = player.Name
		pTick.Position = player.Position()
		pTick.ViewAngleX = player.ViewDirectionX()
		pTick.ViewAngleY = player.ViewDirectionY()
		pTick.IsAlive = player.IsAlive()
		pTick.Velocity2D = vel2D
		pTick.Velocity3D = vel3D
		pTick.ActiveWeapon = player.ActiveWeapon()
		pTick.AmmoLeft = player.AmmoLeft
		pTick.EntityID = player.Entity.ID()
		pTick.FlashedAtTick = player.FlashTick
		pTick.FlashedTimeRemaining = player.FlashDurationTimeRemaining()
		pTick.Team = player.Team
		pTick.IsConnected = player.IsConnected
		pTick.IsAirborne = player.IsAirborne()
		pTick.IsBlinded = player.IsBlinded()
		pTick.IsBot = player.IsBot
		pTick.IsCrouched = player.IsDucking()
		pTick.IsDefusing = player.IsDefusing
		pTick.IsPlanting = player.IsPlanting
		pTick.IsReloading = player.IsReloading
		pTick.IsScoped = player.IsScoped()
		pTick.IsUpright = player.IsStanding()
		pTick.IsWalking = player.IsWalking()
		pTick.Assists = player.Assists()
		pTick.Deaths = player.Deaths()
		pTick.Kills = player.Kills()
		pTick.Damage = player.TotalDamage()
		pTick.Health = player.Health()
		pTick.Armor = player.Armor()
		pTick.UtilityDamage = player.UtilityDamage()
		pTick.Money = player.Money()
		pTick.CurrentRoundMoneySpent = player.MoneySpentThisRound()
		pTick.CurrentMoneySpentTotal = player.MoneySpentTotal()
		pTick.DemoTime = c.parser.CurrentTime()

		c.PerTickInfo[currentTick][player.SteamID64] = pTick
	}
}

func (c *Collector) handleWeaponFire(e events.WeaponFire) {
	if e.Shooter == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick][e.Shooter.SteamID64]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
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

	if _, ok := c.PerTickInfo[currentTick][attackerID]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}
	if _, ok := c.PerTickInfo[currentTick][victimID]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	attackerData := c.PerTickInfo[currentTick][attackerID]
	victimData := c.PerTickInfo[currentTick][victimID]

	if attackerData.DamageDealtToPlayer == nil {
		attackerData.DamageDealtToPlayer = make(map[uint64]types.DamageDealt)
	}

	dmg := types.DamageDealt{
		ArmorDamage:  e.ArmorDamageTaken,
		HealthDamage: e.HealthDamageTaken,
		HitGroup:     byte(e.HitGroup),
	}

	attackerData.DamageDealtToPlayer[victimID] = dmg

	c.PerTickInfo[currentTick][attackerID] = attackerData
	c.PerTickInfo[currentTick][victimID] = victimData
}
