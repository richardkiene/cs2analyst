package analyzer

import (
	"log/slog"
	"sort"
	"time"

	"github.com/richardkiene/cs2analyst/types"
	"github.com/richardkiene/cs2analyst/visibility"
)

type visibilityWindow struct {
	startTick int // When visibility began
	startTime time.Duration
	endTick   int // When visibility ended
	endTime   time.Duration
	isValid   bool // Whether this is a valid visibility period
}

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
		validWindows     int
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

	// Collect all damage events
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

		// Find visibility windows
		var lastWindowEndTick int
		var visibilityWindows []visibilityWindow

		for _, dmg := range damages {
			if dmg.tick > lastWindowEndTick {
				stat := stats[pair.shooter]
				stat.visibilityChecks++
				stats[pair.shooter] = stat
				if result, ok := a.visibility.FindLastContinuousVisibilityStart(
					pair.shooter, pair.target, dmg.tick, tickData); ok && result.IsValid {

					// Find when this visibility period ends
					endTick := dmg.tick
					for t := result.StartTick; t <= dmg.tick; t++ {
						if !a.canSeeAtTick(pair.shooter, pair.target, t, tickData) {
							endTick = t - 1
							break
						}
					}

					window := visibilityWindow{
						startTick: result.StartTick,
						startTime: result.StartTime,
						endTick:   endTick,
						isValid:   true,
					}
					visibilityWindows = append(visibilityWindows, window)
					lastWindowEndTick = endTick
					stat := stats[pair.shooter]
					stat.validWindows++
					stats[pair.shooter] = stat

					// Debug significant visibility windows (>100ms)
					windowDuration := float64(endTick-result.StartTick) * msPerTick
					if windowDuration > 100 {
						a.logger.Debug("Significant visibility window",
							"shooter", pair.shooter,
							"target", pair.target,
							"startTick", result.StartTick,
							"endTick", endTick,
							"duration_ms", windowDuration)
					}
				}
			}
		}

		// Calculate TTD for each damage event
		for _, dmg := range damages {
			var relevantWindow *visibilityWindow
			for i := len(visibilityWindows) - 1; i >= 0; i-- {
				window := visibilityWindows[i]
				if window.startTick <= dmg.tick && window.isValid {
					relevantWindow = &visibilityWindows[i]
					break
				}
			}

			if relevantWindow != nil {
				timeDelta := float64(dmg.tick-relevantWindow.startTick) * msPerTick
				if timeDelta < 1000.0 {
					playerTimeToDamage[pair.shooter] = append(
						playerTimeToDamage[pair.shooter],
						timeDelta,
					)
					stat := stats[pair.shooter]
					stat.ttdSamples++
					stats[pair.shooter] = stat

					// Debug TTD samples that are significantly different from Leetify's numbers
					// Adjust these thresholds based on the differences we're seeing
					if pair.shooter == 76561198863796909 && timeDelta < 900 { // Example player with big difference
						a.logger.Debug("Noteworthy TTD sample",
							"shooter", pair.shooter,
							"target", pair.target,
							"damageTick", dmg.tick,
							"visibilityStartTick", relevantWindow.startTick,
							"ttd", timeDelta)
					}
				}
			}
		}
	}

	// Log processing stats before median calculation
	for steamID, stat := range stats {
		a.logger.Info("Player processing stats",
			"steamID", steamID,
			"damageEvents", stat.damageEvents,
			"visibilityChecks", stat.visibilityChecks,
			"validWindows", stat.validWindows,
			"ttdSamples", stat.ttdSamples)
	}

	// Calculate and log medians
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

		// Log distribution info for players with significant differences from Leetify
		if steamID == 76561198863796909 || // Example player with big difference
			steamID == 76561199214428404 { // Another example
			a.logger.Info("TTD distribution",
				"steamID", steamID,
				"sampleCount", len(timings),
				"median", median,
				"min", timings[0],
				"max", timings[len(timings)-1],
				"p25", timings[len(timings)/4],
				"p75", timings[len(timings)*3/4])
		}
	}

	return medianTimeToDamage, nil
}

// Helper function to check visibility at a specific tick
func (a *Analyzer) canSeeAtTick(shooter, target uint64, tick int, tickData map[int]map[uint64]types.PlayerTickData) bool {
	if tickMap, ok := tickData[tick]; ok {
		if shooterData, ok := tickMap[shooter]; ok {
			if targetData, ok := tickMap[target]; ok {
				if !shooterData.IsAlive || !targetData.IsAlive || shooterData.IsBlinded {
					return false
				}

				if result, ok := a.visibility.FindLastContinuousVisibilityStart(shooter, target, tick, tickData); ok && result.IsValid {
					return true
				}

				return false
			}
		}
	}
	return false
}
