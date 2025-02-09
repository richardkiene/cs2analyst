package analyzer

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/richardkiene/cs2analyst/collector"
	"github.com/richardkiene/cs2analyst/visibility"
)

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

func (a *Analyzer) Analyze(tickData map[int]map[uint64]collector.PlayerTickData, tickRate float64, tickTime time.Duration) (map[uint64]float64, error) {
	playerTimeToDamage := make(map[uint64][]float64)
	medianTimeToDamage := make(map[uint64]float64)
	msPerTick := 1000.0 / tickRate

	// Track the first damage in each engagement
	processedVisibility := make(map[string]bool)

	for currentTick, playerMap := range tickData {
		for steamID, player := range playerMap {
			for targetID, damage := range player.DamageDealtToPlayer {
				if damage.HealthDamage > 0 {
					if lastVisibilityTick, ok := a.visibility.FindLastContinuousVisibilityStart(steamID, targetID, currentTick, tickData); ok {
						visKey := fmt.Sprintf("%d-%d-%d", steamID, targetID, lastVisibilityTick)
						if !processedVisibility[visKey] {
							timeDelta := float64(currentTick-lastVisibilityTick) * msPerTick
							if timeDelta < 1000.0 {
								// Only log if this is the first hit in an engagement
								playerTimeToDamage[steamID] = append(playerTimeToDamage[steamID], timeDelta)
								processedVisibility[visKey] = true

								if steamID == 76561197991944713 {
									fmt.Printf("For shooter %d vs target %d: first visible tick = %d, damage tick = %d, interval = %.2f ms\n",
										steamID, targetID, lastVisibilityTick, currentTick, timeDelta)
								}
							}
						}
					}
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
		if len(timings)%2 == 0 {
			medianTimeToDamage[steamID] = (timings[middle-1] + timings[middle]) / 2
		} else {
			medianTimeToDamage[steamID] = timings[middle]
		}
	}

	return medianTimeToDamage, nil
}
