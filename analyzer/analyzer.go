package analyzer

import (
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

func (a *Analyzer) Analyze(tickData map[int]map[uint64]types.PlayerTickData, tickRate float64, tickTime time.Duration) (map[uint64]float64, error) {
	playerTimeToDamage := make(map[uint64][]float64)
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
		shooter uint64
		target  uint64
	}

	type damageEvent struct {
		tick   int
		damage int
	}

	damagesByPair := make(map[playerPair][]damageEvent)
	var allPairs []playerPair
	pairsSeen := make(map[playerPair]bool)

	// Collect damage events
	for tick, playerMap := range tickData {
		for steamID, player := range playerMap {
			for targetID, damage := range player.DamageDealtToPlayer {
				if damage.HealthDamage > 0 {
					pair := playerPair{shooter: steamID, target: targetID}
					damagesByPair[pair] = append(damagesByPair[pair], damageEvent{
						tick:   tick,
						damage: damage.HealthDamage,
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

	// Sort pairs for deterministic processing
	sort.Slice(allPairs, func(i, j int) bool {
		if allPairs[i].shooter != allPairs[j].shooter {
			return allPairs[i].shooter < allPairs[j].shooter
		}
		return allPairs[i].target < allPairs[j].target
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
				continue
			}

			stat := stats[pair.shooter]
			stat.visibilityChecks++
			stats[pair.shooter] = stat

			// Find when the shooter first saw the target before this damage
			if result, ok := a.visibility.FindLastContinuousVisibilityStart(
				pair.shooter, pair.target, dmg.tick, tickData); ok && result.IsValid {

				// Calculate TTD
				timeDelta := float64(dmg.tick-result.StartTick) * msPerTick
				if timeDelta < 1000.0 { // Filter out unreasonably long TTDs
					playerTimeToDamage[pair.shooter] = append(
						playerTimeToDamage[pair.shooter],
						timeDelta,
					)
					stat := stats[pair.shooter]
					stat.ttdSamples++
					stats[pair.shooter] = stat

					lastEngagementTick = dmg.tick

					// Debug logging
					// Setting to Warn for now
					a.logger.Warn("TTD sample recorded",
						"shooter", pair.shooter,
						"target", pair.target,
						"visibilityStartTick", result.StartTick,
						"firstVisibleShooterPos", result.ShooterPos,
						"firstVisibleTargetPos", result.VictimPos,
						"damageTick", dmg.tick,
						"ttd", timeDelta)
				} else if timeDelta >= 1000.0 && timeDelta < 1500.00 && pair.shooter == 76561198863796909 {
					a.logger.Warn("TTD sample in suspect zone",
						"shooter", pair.shooter,
						"target", pair.target,
						"visibilityStartTick", result.StartTick,
						"firstVisibleShooterPos", result.ShooterPos,
						"firstVisibleTargetPos", result.VictimPos,
						"damageTick", dmg.tick,
						"ttd", timeDelta)
				}
			}
		}
	}

	// Calculate medians
	for steamID, timings := range playerTimeToDamage {
		if len(timings) == 0 {
			medianTimeToDamage[steamID] = 0
			continue
		}

		sort.Float64s(timings)
		middle := len(timings) / 2
		var median float64
		if len(timings)%2 == 0 {
			median = (timings[middle-1] + timings[middle]) / 2
		} else {
			median = timings[middle]
		}
		medianTimeToDamage[steamID] = median

		// Log distribution info
		a.logger.Info("TTD distribution",
			"steamID", steamID,
			"sampleCount", len(timings),
			"median", median,
			"min", timings[0],
			"max", timings[len(timings)-1],
			"p25", timings[len(timings)/4],
			"p75", timings[len(timings)*3/4])
	}

	return medianTimeToDamage, nil
}
