package analyzer

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/richardkiene/cs2analyst/types"
	"github.com/richardkiene/cs2analyst/visibility"
)

const minNewEngagementTicks = 100 // Minimum ticks between damage events to consider it a new engagement

type Analyzer struct {
	visibility visibility.Visibility
	logger     slog.Logger
}

func New() (*Analyzer, error) {
	a := &Analyzer{
		visibility: *visibility.New(""),
		logger:     *slog.Default(),
	}

	los, err := a.visibility.NewLineOfSightSystem("", "")
	if err != nil {
		a.logger.Error("Failed to initialize LineOfSightSystem", "error", err)
		return nil, err
	}

	a.visibility.LosSystem = *los

	return a, nil
}

func (a *Analyzer) Analyze(tickData map[int]map[uint64]types.PlayerTickData, smokes []types.ActiveSmoke, tickRate float64, tickTime time.Duration) (map[uint64]float64, error) {
	playerTimeToDamage := make(map[uint64][]types.TimeToDamageResult)
	medianTimeToDamage := make(map[uint64]float64)
	msPerTick := 1000.0 / tickRate

	// Track stats for debugging
	stats := make(map[uint64]struct {
		damageEvents     int
		visibilityChecks int
		ttdSamples       int
	})

	// Group damage events by player pairs
	type playerPair struct {
		shooterSteamID uint64
		shooterName    string
		targetSteamID  uint64
		targetName     string
	}

	type damageEvent struct {
		tick           int
		damage         int
		isBulletDamage bool
	}

	damagesByPair := make(map[playerPair][]damageEvent)
	var allPairs []playerPair
	pairsSeen := make(map[playerPair]bool)

	// Debug counters
	skippedEngagementWindow := 0
	skippedNoVisibility := 0
	skippedLongTTD := 0
	nonBulletDamage := 0
	acceptedTTD := 0
	totalDamageEvents := 0

	a.logger.Info("Starting analysis",
		"tickRate", tickRate,
		"msPerTick", msPerTick,
		"tickTime", tickTime)

	// Collect damage events - now filtering for bullet damage only
	for tick, playerMap := range tickData {
		for steamID, player := range playerMap {
			for targetID, damage := range player.DamageDealtToPlayer {
				// Only consider damage events with health damage > 0
				if damage.HealthDamage > 0 {
					totalDamageEvents++

					// Skip damage that's not from bullets
					if !damage.IsBulletDamage {
						nonBulletDamage++
						a.logger.Debug("non bullet damage event",
							"tick", tick,
							"shooter", steamID,
							"target", targetID,
							"damage", damage.HealthDamage,
							"active weapon", getActiveWeaponName(player),
						)
					}

					a.logger.Info("Recorded damage event",
						"tick", tick,
						"shooterSteamID", fmt.Sprintf("%d", steamID),
						"shooterPosition", player.Position,
						"shooterViewAngleX", player.ViewAngleX,
						"shooterViewAngleY", player.ViewAngleY,
						"targetSteamID", fmt.Sprintf("%d", targetID),
						"targetPosition", playerMap[targetID].Position,
						"targetViewAngleX", playerMap[targetID].ViewAngleX,
						"targetViewAngleY", playerMap[targetID].ViewAngleY,
						"shooterActiveWeapon", getActiveWeaponName(player),
						"isBulletDamage", damage.IsBulletDamage,
						"healthDamage", damage.HealthDamage)

					pair := playerPair{shooterSteamID: steamID, shooterName: player.PlayerName, targetSteamID: targetID, targetName: playerMap[targetID].PlayerName}
					damagesByPair[pair] = append(damagesByPair[pair], damageEvent{
						tick:           tick,
						damage:         damage.HealthDamage,
						isBulletDamage: damage.IsBulletDamage,
					})
					if !pairsSeen[pair] {
						allPairs = append(allPairs, pair)
						pairsSeen[pair] = true
					}
					stat := stats[steamID]
					stat.damageEvents++
					stats[steamID] = stat
				}
			}
		}
	}

	a.logger.Info("Bullet damage events collected",
		"totalDamageEvents", totalDamageEvents,
		"bulletDamageEvents", totalDamageEvents-nonBulletDamage,
		"nonBulletDamageEvents", nonBulletDamage)

	// Sort pairs for deterministic processing
	sort.Slice(allPairs, func(i, j int) bool {
		if allPairs[i].shooterSteamID != allPairs[j].shooterSteamID {
			return allPairs[i].shooterSteamID < allPairs[j].shooterSteamID
		}
		return allPairs[i].targetSteamID < allPairs[j].targetSteamID
	})

	// Process each player pair
	for _, pair := range allPairs {
		damages := damagesByPair[pair]
		sort.Slice(damages, func(i, j int) bool {
			return damages[i].tick < damages[j].tick
		})

		lastEngagementTick := -minNewEngagementTicks // Initialize to allow first engagement

		// Process each damage event
		for _, dmg := range damages {
			// Skip if this damage is too close to the last engagement
			if dmg.tick-lastEngagementTick < minNewEngagementTicks {
				skippedEngagementWindow++
				a.logger.Debug("Skipped damage event - too close to last engagement",
					"shooter", pair.shooterSteamID,
					"target", pair.targetSteamID,
					"damageTick", dmg.tick,
					"lastEngagementTick", lastEngagementTick,
					"delta", dmg.tick-lastEngagementTick)
				continue
			}

			if !dmg.isBulletDamage {
				slog.Debug("Skipping damage event for non-bullet based weapon")
				continue
			}

			stat := stats[pair.shooterSteamID]
			stat.visibilityChecks++
			stats[pair.shooterSteamID] = stat

			// Find when the shooter first saw the target before this damage
			// Skip this if it is not a bullet damage event
			if result, ok := a.visibility.FindLastContinuousVisibilityStart(
				pair.shooterSteamID, pair.targetSteamID, dmg.tick, tickData, smokes); ok && result.IsValid {

				// Calculate TTD
				// Calculate time delta in milliseconds
				tickDelta := dmg.tick - result.StartTick
				if tickDelta <= 0 {
					a.logger.Info("Invalid tick delta - damage at same tick or before visibility",
						"shooter", pair.shooterSteamID,
						"target", pair.targetSteamID,
						"visibilityStartTick", result.StartTick,
						"damageTick", dmg.tick,
						"tickDelta", tickDelta)
					continue
				}

				// Verify tickRate is valid
				if tickRate <= 0 || tickRate > 128 { // CS2 tickrate should be between 16 and 128
					a.logger.Error("Invalid tickRate",
						"tickRate", tickRate)
					continue
				}

				timeDelta := float64(tickDelta) * msPerTick
				if timeDelta < 1000.0 { // Filter out unreasonably long TTDs
					playerTimeToDamage[pair.shooterSteamID] = append(
						playerTimeToDamage[pair.shooterSteamID],
						types.TimeToDamageResult{
							TimeDelta:  timeDelta,
							SteamID:    pair.shooterSteamID,
							PlayerName: pair.shooterName,
						},
					)
					stat := stats[pair.shooterSteamID]
					stat.ttdSamples++
					stats[pair.shooterSteamID] = stat

					acceptedTTD++
					lastEngagementTick = dmg.tick

					a.logger.Info("TTD sample recorded",
						"shooterSteamID", pair.shooterSteamID,
						"shooterName", pair.shooterName,
						"targetSteamID", pair.targetSteamID,
						"targetName", pair.targetName,
						"visibilityStartTick", result.StartTick,
						"damageTick", dmg.tick,
						"ttd", timeDelta)
				} else {
					skippedLongTTD++
					a.logger.Debug("Skipped damage event - TTD too long",
						"shooterSteamID", pair.shooterSteamID,
						"shooterName", pair.shooterName,
						"targetSteamID", pair.targetSteamID,
						"targetName", pair.targetName,
						"visibilityStartTick", result.StartTick,
						"damageTick", dmg.tick,
						"ttd", timeDelta)
				}
			} else {
				skippedNoVisibility++
				a.logger.Info("Skipped damage event - no visibility found",
					"shooterSteamID", pair.shooterSteamID,
					"shooterName", pair.shooterName,
					"targetSteamID", pair.targetSteamID,
					"targetName", pair.targetName,
					"visibilityStartTick", result.StartTick,
					"damageTick", dmg.tick)
			}
		}
	}

	a.logger.Info("TTD Analysis Complete",
		"totalDamageEvents", totalDamageEvents,
		"bulletDamageEvents", totalDamageEvents-nonBulletDamage,
		"skippedNonBullet", nonBulletDamage,
		"skippedEngagementWindow", skippedEngagementWindow,
		"skippedNoVisibility", skippedNoVisibility,
		"skippedLongTTD", skippedLongTTD,
		"acceptedTTD", acceptedTTD)

	// Calculate medians
	for steamID, results := range playerTimeToDamage {
		if len(results) == 0 {
			medianTimeToDamage[steamID] = 0
			continue
		}

		// Extract TimeDelta values
		timings := make([]float64, len(results))
		for i, result := range results {
			timings[i] = result.TimeDelta
		}

		// Sort the timings
		sort.Float64s(timings)

		// Calculate median
		middle := len(timings) / 2
		var median float64
		if len(timings)%2 == 0 {
			median = (timings[middle-1] + timings[middle]) / 2
		} else {
			median = timings[middle]
		}
		medianTimeToDamage[steamID] = median

		// Log distribution info with player names and Steam IDs
		a.logger.Info("TTD distribution",
			slog.Uint64("steamID", steamID),
			slog.String("playerName", results[0].PlayerName), // Assuming all entries have the same PlayerName for the same SteamID
			slog.Int("sampleCount", len(timings)),
			slog.Float64("median", median),
			slog.Float64("min", timings[0]),
			slog.Float64("max", timings[len(timings)-1]),
			slog.Float64("p25", timings[len(timings)/4]),
			slog.Float64("p75", timings[len(timings)*3/4]))
	}

	return medianTimeToDamage, nil
}

func getActiveWeaponName(player types.PlayerTickData) string {
	if player.ActiveWeapon != nil {
		return player.ActiveWeapon.Type.String()
	}

	return "unknown"
}
