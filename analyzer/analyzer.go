package analyzer

import (
	"log/slog"
	"sort"
	"time"

	"github.com/richardkiene/cs2analyst/types"
	"github.com/richardkiene/cs2analyst/visibility"
)

type visibilityWindow struct {
	startTick int  // When visibility began
	endTick   int  // When visibility ended
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

	// Get all damage events in chronological order
	var entries []struct {
		tick     int
		steamID  uint64
		targetID uint64
		damage   int
	}

	// Collect all damage events
	for tick, playerMap := range tickData {
		for steamID, player := range playerMap {
			for targetID, damage := range player.DamageDealtToPlayer {
				if damage.HealthDamage > 0 {
					entries = append(entries, struct {
						tick     int
						steamID  uint64
						targetID uint64
						damage   int
					}{
						tick:     tick,
						steamID:  steamID,
						targetID: targetID,
						damage:   damage.HealthDamage,
					})
				}
			}
		}
	}

	// Sort by tick for deterministic processing
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].tick != entries[j].tick {
			return entries[i].tick < entries[j].tick
		}
		if entries[i].steamID != entries[j].steamID {
			return entries[i].steamID < entries[j].steamID
		}
		return entries[i].targetID < entries[j].targetID
	})

	// Process each damage event
	for _, entry := range entries {
		// For each damage event, find when we first saw the target
		if result, ok := a.visibility.FindLastContinuousVisibilityStart(
			entry.steamID, entry.targetID, entry.tick, tickData); ok && result.IsValid {

			// Calculate time between first sight and damage
			timeDelta := float64(entry.tick-result.StartTick) * msPerTick

			// Only include TTD under 1 second (same as Leetify)
			if timeDelta < 1000.0 {
				playerTimeToDamage[entry.steamID] = append(
					playerTimeToDamage[entry.steamID],
					timeDelta,
				)

				// Debug logging
				a.logger.Debug("TTD calculated",
					"shooter", entry.steamID,
					"target", entry.targetID,
					"damageTick", entry.tick,
					"firstSightTick", result.StartTick,
					"ttd", timeDelta,
				)
			}
		}
	}

	// Log samples before calculating median
	for steamID, samples := range playerTimeToDamage {
		a.logger.Info("TTD samples",
			"steamID", steamID,
			"sampleCount", len(samples),
			"samples", samples,
		)
	}

	// Calculate median TTD for each player
	for steamID, timings := range playerTimeToDamage {
		if len(timings) == 0 {
			medianTimeToDamage[steamID] = 0
			continue
		}

		sort.Float64s(timings)
		middle := len(timings) / 2
		if len(timings)%2 == 0 {
			medianTimeToDamage[steamID] = (timings[middle-1] + timings[middle]) / 2
		} else {
			medianTimeToDamage[steamID] = timings[middle]
		}
	}

	return medianTimeToDamage, nil
}
