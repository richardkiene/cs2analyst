package collector

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/golang/geo/r3"
	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/msgs2"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/richardkiene/cs2analyst/visibility"
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
	// Network and entity handlers
	c.parser.RegisterNetMessageHandler(c.handleServerInfo)
	c.parser.RegisterNetMessageHandler(c.handleEntityUpdate)

	// Combat events
	c.parser.RegisterEventHandler(c.handleWeaponFire)
	c.parser.RegisterEventHandler(c.handlePlayerHurt)
	c.parser.RegisterEventHandler(c.handleBulletDamage)
	c.parser.RegisterEventHandler(c.handleKill)
	c.parser.RegisterEventHandler(c.handlePlayerFlashed)
	c.parser.RegisterEventHandler(c.handleWeaponReload)

	// Movement and positioning events
	c.parser.RegisterEventHandler(c.handlePlayerJump)
	c.parser.RegisterEventHandler(c.handleFootstep)
	c.parser.RegisterEventHandler(c.handlePlayerSpottersChanged)
	c.parser.RegisterEventHandler(c.handlePlayerSound)

	// Equipment events
	c.parser.RegisterEventHandler(c.handleItemEquip)
	c.parser.RegisterEventHandler(c.handleItemPickup)
	c.parser.RegisterEventHandler(c.handleItemDrop)
	c.parser.RegisterEventHandler(c.handleItemRefund)

	// Grenade events
	c.parser.RegisterEventHandler(c.handleGrenadeProjectileThrow)
	c.parser.RegisterEventHandler(c.handleGrenadeProjectileBounce)
	c.parser.RegisterEventHandler(c.handleFlashExplode)
	c.parser.RegisterEventHandler(c.handleHeExplode)
	c.parser.RegisterEventHandler(c.handleSmokeStart)
	c.parser.RegisterEventHandler(c.handleSmokeExpired)
	c.parser.RegisterEventHandler(c.handleInfernoStart)
	c.parser.RegisterEventHandler(c.handleInfernoExpired)

	// Round state events
	c.parser.RegisterEventHandler(c.handleRoundStart)
	c.parser.RegisterEventHandler(c.handleRoundEnd)
	c.parser.RegisterEventHandler(c.handleRoundEndOfficial)

	// Player state events
	c.parser.RegisterEventHandler(c.handlePlayerConnect)
	c.parser.RegisterEventHandler(c.handlePlayerDisconnected)
	c.parser.RegisterEventHandler(c.handlePlayerTeamChange)
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
				ActiveGrenades:      make(map[int]*types.GrenadeData),
				GrenadeHistory:      make([]*types.GrenadeData, 0),
				BuyHistory:          make([]common.EquipmentType, 0),
				SpottedBy:           make([]uint64, 0),
				CanSee:              make([]uint64, 0),
				SoundEvents:         make([]string, 0),
			}
		}

		// Update basic player data
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

		// Track movement patterns
		if lastPTD.Position != (r3.Vector{X: 0, Y: 0, Z: 0}) {
			if !vectorsApproxEqual(lastPTD.LastMoveDirection, pTick.Position.Sub(lastPTD.Position)) {
				pTick.DirectionChanges++
			}
			pTick.LastMoveDirection = pTick.Position.Sub(lastPTD.Position)
		}

		c.PerTickInfo[currentTick][player.SteamID64] = pTick
	}
}

// Combat event handlers
func (c *Collector) handleWeaponFire(e events.WeaponFire) {
	if e.Shooter == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	shooterData := c.PerTickInfo[currentTick][e.Shooter.SteamID64]
	shooterData.FiredActiveWeapon = true
	shooterData.BulletsFired++

	// Track shot accuracy context
	if e.Shooter.IsBlinded() {
		shooterData.BlindShots++
	}
	if e.Shooter.IsAirborne() {
		shooterData.JumpShots++
	}

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

func (c *Collector) handleBulletDamage(e events.BulletDamage) {
	if e.Attacker == nil || e.Victim == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Update attacker data
	attackerData := c.PerTickInfo[currentTick][e.Attacker.SteamID64]
	victimData := c.PerTickInfo[currentTick][e.Victim.SteamID64]

	damageData := types.DamageDealt{
		Distance:        e.Distance,
		NumPenetrations: e.NumPenetrations,
		IsNoScope:       e.IsNoScope,
		IsAttackerInAir: e.IsAttackerInAir,
		DamageDirection: r3.Vector{X: float64(e.DamageDirX), Y: float64(e.DamageDirY), Z: float64(e.DamageDirZ)},
	}

	if attackerData.DamageDealtToPlayer == nil {
		attackerData.DamageDealtToPlayer = make(map[uint64]types.DamageDealt)
	}

	// Update attacker stats
	attackerData.BulletsFired++
	attackerData.BulletsHit++
	attackerData.LastDamageDealtTick = currentTick
	attackerData.RoundDamageDealt += damageData.HealthDamage
	if e.NumPenetrations > 0 {
		attackerData.WallbangCount++
	}
	if e.IsNoScope {
		attackerData.NoScopeCount++
	}
	if attackerData.IsBlinded {
		attackerData.BlindKills++
	}

	// Update victim stats
	victimData.LastDamageTakenTick = currentTick

	attackerData.DamageDealtToPlayer[e.Victim.SteamID64] = damageData

	c.PerTickInfo[currentTick][e.Attacker.SteamID64] = attackerData
	c.PerTickInfo[currentTick][e.Victim.SteamID64] = victimData
}

func (c *Collector) handleKill(e events.Kill) {
	if e.Killer == nil || e.Victim == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Update killer stats
	killerData := c.PerTickInfo[currentTick][e.Killer.SteamID64]
	killerData.Kills++
	killerData.LastKillTick = currentTick
	killerData.KillStreak++
	if e.IsHeadshot {
		killerData.HeadshotCount++
	}
	if e.AttackerBlind {
		killerData.BlindKills++
	}
	if e.ThroughSmoke {
		killerData.ThroughSmokeKills++
	}
	if e.PenetratedObjects > 0 {
		killerData.WallbangCount++
	}
	if e.NoScope {
		killerData.NoScopeCount++
	}
	c.PerTickInfo[currentTick][e.Killer.SteamID64] = killerData

	// Update victim stats
	victimData := c.PerTickInfo[currentTick][e.Victim.SteamID64]
	victimData.Deaths++
	victimData.KillStreak = 0
	victimData.LastDeathTick = currentTick
	c.PerTickInfo[currentTick][e.Victim.SteamID64] = victimData

	// Update assister stats
	if e.Assister != nil {
		assisterData := c.PerTickInfo[currentTick][e.Assister.SteamID64]
		assisterData.Assists++
		assisterData.RoundSupportScore += 2
		c.PerTickInfo[currentTick][e.Assister.SteamID64] = assisterData
	}
}

func (c *Collector) handlePlayerFlashed(e events.PlayerFlashed) {
	if e.Player == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Player.SteamID64]
	playerData.IsBlinded = true
	playerData.LastFlashTick = currentTick
	playerData.FlashDuration = float32(e.FlashDuration().Seconds())

	// Update flash thrower's stats if available
	if e.Attacker != nil {
		attackerData := c.PerTickInfo[currentTick][e.Attacker.SteamID64]
		if grenadeData, ok := attackerData.ActiveGrenades[e.Projectile.Entity.ID()]; ok {
			grenadeData.AffectedPlayers = append(grenadeData.AffectedPlayers, e.Player.SteamID64)
			grenadeData.FlashDuration = playerData.FlashDuration
			attackerData.RoundEnemiesFlashed++
			attackerData.RoundSupportScore++
		}
		c.PerTickInfo[currentTick][e.Attacker.SteamID64] = attackerData
	}

	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

// Movement and positioning handlers
func (c *Collector) handlePlayerJump(e events.PlayerJump) {
	if e.Player == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Player.SteamID64]
	playerData.IsAirborne = true
	playerData.LastJumpTick = currentTick
	playerData.JumpCount++

	// Track if player is making noise
	playerData.IsMakingNoise = true
	playerData.NoiseRadius = 350.0 // Typical jump sound radius

	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

func (c *Collector) handlePlayerSound(e events.PlayerSound) {
	if e.Player == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Player.SteamID64]
	playerData.IsMakingNoise = true
	playerData.NoiseRadius = float32(e.Radius)
	playerData.LastStepTick = currentTick
	playerData.StepCount++

	if e.Duration > 0 {
		playerData.SoundEvents = append(playerData.SoundEvents, "step")
	}

	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

// TODO: This function is probably worthless... see: https://github.com/markus-wa/demoinfocs-golang/issues/262#issuecomment-778646980
func (c *Collector) handlePlayerSpottersChanged(e events.PlayerSpottersChanged) {
	if e.Spotted == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	gs := c.parser.GameState()
	playerData := c.PerTickInfo[currentTick][e.Spotted.SteamID64]

	// Clear and update who can see this player
	playerData.SpottedBy = make([]uint64, 0)

	// Check all players to see who can spot the spotted player
	for _, player := range gs.Participants().Playing() {
		if player.IsAlive() && player.SteamID64 != e.Spotted.SteamID64 {
			// Check if this player can see the spotted player
			// Note: You may want to add team checks here depending on your needs
			spotterData := c.PerTickInfo[currentTick][player.SteamID64]

			// Use visibility check from the game state
			// TODO: this is flawed, I don't think it is what we want, this isn't true had LOS at Tick X
			// CONFIRMED WRONG: https://github.com/markus-wa/demoinfocs-golang/issues/262#issuecomment-778646980
			if player.HasSpotted(e.Spotted) {
				playerData.SpottedBy = append(playerData.SpottedBy, player.SteamID64)

				// Update the spotter's data as well
				if spotterData.CanSee == nil {
					spotterData.CanSee = make([]uint64, 0)
				}
				spotterData.CanSee = append(spotterData.CanSee, e.Spotted.SteamID64)
				c.PerTickInfo[currentTick][player.SteamID64] = spotterData
			}
		}
	}

	if len(playerData.SpottedBy) > 0 {
		playerData.LastContactTick = currentTick
	}

	c.PerTickInfo[currentTick][e.Spotted.SteamID64] = playerData
}

// Utility handlers
func (c *Collector) handleGrenadeProjectileThrow(e events.GrenadeProjectileThrow) {
	if e.Projectile.Thrower == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Projectile.Thrower.SteamID64]

	if playerData.ActiveGrenades == nil {
		playerData.ActiveGrenades = make(map[int]*types.GrenadeData)
	}

	grenadeData := &types.GrenadeData{
		Type:            e.Projectile.WeaponInstance.Type,
		ProjectileID:    e.Projectile.Entity.ID(),
		ThrownPosition:  e.Projectile.Thrower.Position(),
		ThrownTick:      currentTick,
		Bounces:         make([]r3.Vector, 0),
		AffectedPlayers: make([]uint64, 0),
	}

	playerData.ActiveGrenades[e.Projectile.Entity.ID()] = grenadeData

	c.PerTickInfo[currentTick][e.Projectile.Thrower.SteamID64] = playerData
}

func (c *Collector) handleGrenadeProjectileBounce(e events.GrenadeProjectileBounce) {
	currentTick := c.parser.GameState().IngameTick()
	if e.Projectile.Thrower == nil {
		return
	}

	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Projectile.Thrower.SteamID64]

	if grenadeData, ok := playerData.ActiveGrenades[e.Projectile.Entity.ID()]; ok {
		grenadeData.Bounces = append(grenadeData.Bounces, e.Projectile.Position())
		c.PerTickInfo[currentTick][e.Projectile.Thrower.SteamID64] = playerData
	}
}

func (c *Collector) handleHeExplode(e events.HeExplode) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	if e.Thrower != nil {
		playerData := c.PerTickInfo[currentTick][e.Thrower.SteamID64]
		if grenadeData, ok := playerData.ActiveGrenades[e.GrenadeEntityID]; ok {
			grenadeData.DetonatePosition = e.Position
			grenadeData.DetonateTick = currentTick

			// Calculate damage radius
			// TODO: Calculate this from the demo or a known source instead of hard coding
			grenadeData.InnerRadius = 250.0 // High damage radius
			grenadeData.OuterRadius = 500.0 // Minimum damage radius

			// Add to history and remove from active
			playerData.GrenadeHistory = append(playerData.GrenadeHistory, grenadeData)
			delete(playerData.ActiveGrenades, e.GrenadeEntityID)
		}
		c.PerTickInfo[currentTick][e.Thrower.SteamID64] = playerData
	}
}

func (c *Collector) handleSmokeStart(e events.SmokeStart) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	if e.Thrower != nil {
		playerData := c.PerTickInfo[currentTick][e.Thrower.SteamID64]
		if grenadeData, ok := playerData.ActiveGrenades[e.GrenadeEntityID]; ok {
			grenadeData.DetonatePosition = e.Position
			grenadeData.DetonateTick = currentTick

			// TODO: These should be calculated or pulled from a known source
			grenadeData.InnerRadius = 150.0 // Full smoke effect
			grenadeData.OuterRadius = 250.0 // Partial smoke effect

			// Add to history and remove from active
			playerData.GrenadeHistory = append(playerData.GrenadeHistory, grenadeData)
			delete(playerData.ActiveGrenades, e.GrenadeEntityID)

			// TODO: Update support score for good smoke placement
			// This could be enhanced with map-specific smoke spot detection
			// For now this is worthless since it increments on any throw
			playerData.RoundSupportScore++
		}
		c.PerTickInfo[currentTick][e.Thrower.SteamID64] = playerData
	}

	// Update all players' visibility relative to smoke
	for steamID, data := range c.PerTickInfo[currentTick] {
		distToSmoke := data.Position.Sub(e.Position).Norm()
		if distToSmoke < 150.0 {
			data.IsInSmoke = true
			data.SmokeBlockedCount++
			c.PerTickInfo[currentTick][steamID] = data
		}
	}
}

func (c *Collector) handleSmokeExpired(e events.SmokeExpired) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Update smoke state for all players
	for steamID, data := range c.PerTickInfo[currentTick] {
		distToSmoke := data.Position.Sub(e.Position).Norm()
		if distToSmoke < 150.0 {
			data.IsInSmoke = false
			c.PerTickInfo[currentTick][steamID] = data
		}
	}
}

func (c *Collector) handleFlashExplode(e events.FlashExplode) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	if e.Thrower != nil {
		playerData := c.PerTickInfo[currentTick][e.Thrower.SteamID64]
		if grenadeData, ok := playerData.ActiveGrenades[e.GrenadeEntityID]; ok {
			grenadeData.DetonatePosition = e.Position
			grenadeData.DetonateTick = currentTick

			// Flash effective radius
			grenadeData.InnerRadius = 500.0  // TODO: Calculate or look up full flash effect possible
			grenadeData.OuterRadius = 1000.0 // TODO: Calculate or look up minimal flash effect possible

			// Add to history and remove from active
			playerData.GrenadeHistory = append(playerData.GrenadeHistory, grenadeData)
			delete(playerData.ActiveGrenades, e.GrenadeEntityID)
		}
		c.PerTickInfo[currentTick][e.Thrower.SteamID64] = playerData
	}
}

func (c *Collector) handleInfernoStart(e events.InfernoStart) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Get the 3D convex hull of the inferno
	hull := e.Inferno.Fires().ConvexHull3D()

	// Update all players' positions relative to the inferno
	for steamID, data := range c.PerTickInfo[currentTick] {
		// Check if player position is inside the convex hull
		if visibility.PointInConvexHull(hull, data.Position) {
			data.IsInMolotov = true
			c.PerTickInfo[currentTick][steamID] = data
		}
	}
}

func (c *Collector) handleInfernoExpired(e events.InfernoExpired) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Get the 3D convex hull of the inferno
	hull := e.Inferno.Fires().ConvexHull3D()

	// Update molotov state for affected players
	for steamID, data := range c.PerTickInfo[currentTick] {
		if visibility.PointInConvexHull(hull, data.Position) {
			data.IsInMolotov = false
			c.PerTickInfo[currentTick][steamID] = data
		}
	}
}

func (c *Collector) handleDecoyStart(e events.DecoyStart) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	if e.Thrower != nil {
		playerData := c.PerTickInfo[currentTick][e.Thrower.SteamID64]
		if grenadeData, ok := playerData.ActiveGrenades[e.GrenadeEntityID]; ok {
			grenadeData.DetonatePosition = e.Position
			grenadeData.DetonateTick = currentTick
			grenadeData.InnerRadius = 300.0 // TODO: Calculate or lookup actual Decoy sound radius

			// Add to history and remove from active
			playerData.GrenadeHistory = append(playerData.GrenadeHistory, grenadeData)
			delete(playerData.ActiveGrenades, e.GrenadeEntityID)
		}
		c.PerTickInfo[currentTick][e.Thrower.SteamID64] = playerData
	}
}

// Equipment handlers
func (c *Collector) handleItemEquip(e events.ItemEquip) {
	if e.Player == nil || e.Weapon == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Player.SteamID64]

	// Update equipped weapons
	switch e.Weapon.Class() {
	case common.EqClassPistols, common.EqClassSMG, common.EqClassRifle, common.EqClassSniper:
		if e.Weapon.Class() == common.EqClassPistols {
			playerData.SecondaryWeapon = e.Weapon
		} else {
			playerData.PrimaryWeapon = e.Weapon
		}
	case common.EqClassGrenade:
		if playerData.GrenadeLoadout == nil {
			playerData.GrenadeLoadout = make([]*common.Equipment, 0)
		}
		playerData.GrenadeLoadout = append(playerData.GrenadeLoadout, e.Weapon)
	}

	playerData.ActiveWeapon = e.Weapon
	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

func (c *Collector) handleItemPickup(e events.ItemPickup) {
	if e.Player == nil || e.Weapon == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Player.SteamID64]

	// Track buy history if this is a new purchase
	if currentTick-playerData.LastBuyTick < 10 { // Within buy window
		playerData.BuyHistory = append(playerData.BuyHistory, e.Weapon.Type)
		playerData.RoundSpendMoney += e.Weapon.Price()

		// Update team economy type
		if e.Weapon.Class() == common.EqClassRifle || e.Weapon.Class() == common.EqClassSniper {
			playerData.TeamEconomyType = "Full Buy"
		} else if e.Weapon.Class() == common.EqClassSMG {
			playerData.TeamEconomyType = "Force Buy"
		}
	}

	playerData.LastBuyTick = currentTick
	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

func (c *Collector) handleItemDrop(e events.ItemDrop) {
	if e.Player == nil || e.Weapon == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Player.SteamID64]
	playerData.LastDropTick = currentTick
	playerData.DroppedValue += e.Weapon.Price()

	// Remove from loadout
	if e.Weapon == playerData.PrimaryWeapon {
		playerData.PrimaryWeapon = nil
	} else if e.Weapon == playerData.SecondaryWeapon {
		playerData.SecondaryWeapon = nil
	} else {
		// Remove from grenade loadout if present
		for i, g := range playerData.GrenadeLoadout {
			if g == e.Weapon {
				playerData.GrenadeLoadout = append(playerData.GrenadeLoadout[:i], playerData.GrenadeLoadout[i+1:]...)
				break
			}
		}
	}

	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

// Round state handlers
func (c *Collector) handleRoundStart(e events.RoundStart) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Reset round-specific stats for all players
	for steamID, playerData := range c.PerTickInfo[currentTick] {
		playerData.RoundDamageDealt = 0
		playerData.RoundUtilityDamage = 0
		playerData.RoundEnemiesFlashed = 0
		playerData.RoundSupportScore = 0
		playerData.RoundStartMoney = playerData.Money
		playerData.RoundSpendMoney = 0
		playerData.ActiveGrenades = make(map[int]*types.GrenadeData)
		playerData.GrenadeHistory = make([]*types.GrenadeData, 0)

		// Reset round-specific flags
		playerData.IsInSmoke = false
		playerData.IsInMolotov = false
		playerData.IsHoldingAngle = false
		playerData.AngleHeldDuration = 0

		// Track economy status
		if playerData.Money < 2000 {
			playerData.TeamEconomyType = "Eco"
		} else if playerData.Money < 4000 {
			playerData.TeamEconomyType = "Force Buy"
		} else {
			playerData.TeamEconomyType = "Full Buy"
		}

		c.PerTickInfo[currentTick][steamID] = playerData
	}
}

func (c *Collector) handleRoundEnd(e events.RoundEnd) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Update economy types and loss bonus for next round
	winnerState := e.WinnerState
	loserState := e.LoserState

	if winnerState != nil {
		for _, player := range winnerState.Members() {
			if playerData, ok := c.PerTickInfo[currentTick][player.SteamID64]; ok {
				playerData.LossBonus = 0                 // Reset loss bonus for winners
				playerData.SavedMoney = playerData.Money // Track money saved
				c.PerTickInfo[currentTick][player.SteamID64] = playerData
			}
		}
	}

	if loserState != nil {
		for _, player := range loserState.Members() {
			if playerData, ok := c.PerTickInfo[currentTick][player.SteamID64]; ok {
				playerData.LossBonus = min(playerData.LossBonus+1, 4) // Increment loss bonus for losers
				playerData.SavedMoney = playerData.Money
				c.PerTickInfo[currentTick][player.SteamID64] = playerData
			}
		}
	}
}

func (c *Collector) handleRoundEndOfficial(e events.RoundEndOfficial) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Calculate final round statistics for all players
	for steamID, playerData := range c.PerTickInfo[currentTick] {
		// Calculate utility efficiency
		totalUtilityValue := 0
		for _, grenade := range playerData.GrenadeHistory {
			if len(grenade.AffectedPlayers) > 0 {
				switch grenade.Type {
				case common.EqFlash:
					totalUtilityValue += 200 // Value for effective flash
				case common.EqHE:
					totalUtilityValue += grenade.Damage
				case common.EqSmoke:
					totalUtilityValue += 300 // Value for smoke that blocked vision
				case common.EqIncendiary, common.EqMolotov:
					totalUtilityValue += 150 // Base value for area denial
				}
			}
		}

		playerData.UnusedUtilityValue = calculateUnusedUtilityValue(playerData.GrenadeLoadout)
		c.PerTickInfo[currentTick][steamID] = playerData
	}
}

func (c *Collector) handleTeamSideSwitch(e events.TeamSideSwitch) {
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Reset side-specific stats and update team affiliations
	for steamID, playerData := range c.PerTickInfo[currentTick] {
		// Swap team sides
		if playerData.PlayerTeam == common.TeamCounterTerrorists {
			playerData.PlayerTeam = common.TeamTerrorists
		} else if playerData.PlayerTeam == common.TeamTerrorists {
			playerData.PlayerTeam = common.TeamCounterTerrorists
		}

		// Reset side-specific statistics
		playerData.RoundStartMoney = playerData.Money
		playerData.TeamEconomyType = "" // Will be recalculated on next round start

		c.PerTickInfo[currentTick][steamID] = playerData
	}
}

// Player state handlers
func (c *Collector) handlePlayerConnect(e events.PlayerConnect) {
	if e.Player == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	// Initialize player data
	playerData := types.PlayerTickData{
		SteamID:             e.Player.SteamID64,
		PlayerName:          e.Player.Name,
		DamageDealtToPlayer: make(map[uint64]types.DamageDealt),
		ActiveGrenades:      make(map[int]*types.GrenadeData),
		GrenadeHistory:      make([]*types.GrenadeData, 0),
		BuyHistory:          make([]common.EquipmentType, 0),
		SpottedBy:           make([]uint64, 0),
		CanSee:              make([]uint64, 0),
		SoundEvents:         make([]string, 0),
		IsConnected:         true,
	}

	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

func (c *Collector) handlePlayerDisconnected(e events.PlayerDisconnected) {
	if e.Player == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	if playerData, ok := c.PerTickInfo[currentTick][e.Player.SteamID64]; ok {
		playerData.IsConnected = false
		c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
	}
}

func (c *Collector) handlePlayerTeamChange(e events.PlayerTeamChange) {
	if e.Player == nil {
		return
	}
	currentTick := c.parser.GameState().IngameTick()
	if _, ok := c.PerTickInfo[currentTick]; !ok {
		c.PerTickInfo[currentTick] = make(map[uint64]types.PlayerTickData)
	}

	playerData := c.PerTickInfo[currentTick][e.Player.SteamID64]
	playerData.PlayerTeam = e.NewTeam

	// Reset team-specific stats
	playerData.RoundStartMoney = playerData.Money
	playerData.TeamEconomyType = ""
	playerData.LossBonus = 0

	c.PerTickInfo[currentTick][e.Player.SteamID64] = playerData
}

// Helper functions
func calculateUnusedUtilityValue(loadout []*common.Equipment) int {
	value := 0
	for _, equipment := range loadout {
		if equipment != nil {
			switch equipment.Type {
			case common.EqFlash:
				value += 200
			case common.EqHE:
				value += 300
			case common.EqSmoke:
				value += 300
			case common.EqIncendiary, common.EqMolotov:
				value += 400
			}
		}
	}
	return value
}

func vectorsApproxEqual(v1, v2 r3.Vector) bool {
	epsilon := 0.001
	return math.Abs(v1.X-v2.X) < epsilon &&
		math.Abs(v1.Y-v2.Y) < epsilon &&
		math.Abs(v1.Z-v2.Z) < epsilon
}

func (c *Collector) calculateVelocity2D(currentPos, lastPos r3.Vector) float64 {
	timeDelta := float64(c.TickTime.Milliseconds())
	disp := currentPos.Sub(lastPos)
	return math.Sqrt(disp.X*disp.X+disp.Y*disp.Y) / timeDelta
}

func (c *Collector) calculateVelocity3D(currentPos, lastPos r3.Vector) float64 {
	timeDelta := float64(c.TickTime.Milliseconds())
	disp := currentPos.Sub(lastPos)
	return disp.Norm() / timeDelta
}
