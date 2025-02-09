package analyzer

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/richardkiene/cs2analyst/types"
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

func (a *Analyzer) Analyze(tickData map[int]map[uint64]types.PlayerTickData, tickRate float64, tickTime time.Duration) (map[uint64]float64, error) {
	playerTimeToDamage := make(map[uint64][]float64)
	medianTimeToDamage := make(map[uint64]float64)
	msPerTick := 1000.0 / tickRate

	type cachedVisibility struct {
		startTick int
		processed bool
	}
	visibilityCache := make(map[string]*cachedVisibility)

	for currentTick, playerMap := range tickData {
		for steamID, player := range playerMap {
			for targetID, damage := range player.DamageDealtToPlayer {
				if damage.HealthDamage > 0 {
					key := fmt.Sprintf("%d-%d", steamID, targetID)
					cached, exists := visibilityCache[key]

					var visibilityStartTick int
					if exists && !cached.processed {
						// Verify cached visibility is still valid
						if result, ok := a.visibility.FindLastContinuousVisibilityStart(steamID, targetID, currentTick, tickData); ok && result.IsValid {
							if result.StartTick == cached.startTick {
								visibilityStartTick = cached.startTick
							} else {
								delete(visibilityCache, key)
							}
						} else {
							delete(visibilityCache, key)
						}
					}

					if !exists || cached.processed {
						if result, ok := a.visibility.FindLastContinuousVisibilityStart(steamID, targetID, currentTick, tickData); ok && result.IsValid {
							visibilityCache[key] = &cachedVisibility{
								startTick: result.StartTick,
								processed: false,
							}
							visibilityStartTick = result.StartTick
						}
					}

					if cached, exists := visibilityCache[key]; exists && !cached.processed {
						timeDelta := float64(currentTick-visibilityStartTick) * msPerTick
						if timeDelta < 1000.0 {
							playerTimeToDamage[steamID] = append(playerTimeToDamage[steamID], timeDelta)
							cached.processed = true

							if steamID == 76561197991944713 {
								fmt.Printf("For shooter %d vs target %d: first visible tick = %d, damage tick = %d, interval = %.2f ms\n",
									steamID, targetID, visibilityStartTick, currentTick, timeDelta)
							}
						}
					}
				}
			}
		}
	}

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
