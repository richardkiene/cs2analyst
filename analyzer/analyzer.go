package analyzer

import (
	"log/slog"
	"runtime"
	"sort"
	"sync"

	"github.com/richardkiene/cs2analyst/collector"
	"github.com/richardkiene/cs2analyst/progress"
	"github.com/richardkiene/cs2analyst/visibility"
	"gonum.org/v1/gonum/stat"
)

type Analyzer struct {
	visibility      visibility.Visibility
	logger          slog.Logger
	progressCreator func(progress.Config) progress.ProgressIndicator
}

func New(progressCreator func(progress.Config) progress.ProgressIndicator) (*Analyzer, error) {
	a := &Analyzer{
		visibility:      *visibility.New(""),
		logger:          *slog.Default(),
		progressCreator: progressCreator,
	}

	los, err := a.visibility.NewLineOfSightSystem("", "")
	if err != nil {
		a.logger.Error("Failed to initialize LineOfSightSystem", "error", err)
		return nil, err
	}

	a.visibility.LosSystem = *los

	return a, nil
}

func (a *Analyzer) Analyze(tickData map[int]map[uint64]collector.PlayerTickData, tickRate float64) (map[uint64]float64, error) {
	// Initialize maps
	playerTimeToDamage := make(map[uint64][]float64)
	medianTimeToDamage := make(map[uint64]float64)
	msPerTick := 1000.0 / tickRate

	// First pass: collect all damage events in original tick order
	var allDamages []damageEvent
	for currentTick, playerMap := range tickData {
		for steamID, player := range playerMap {
			for targetID, damage := range player.DamageDealtToPlayer {
				if damage.HealthDamage > 0 {
					allDamages = append(allDamages, damageEvent{
						tick:     currentTick,
						steamID:  steamID,
						targetID: targetID,
						damage:   damage,
					})
				}
			}
		}
	}

	// Create progress bar for analysis
	progress := a.progressCreator(progress.Config{
		Description: "Analyzing damage events",
		Total:       int64(len(allDamages)),
		ShowBytes:   false,
	})

	// Process damage events in parallel while maintaining order
	var wg sync.WaitGroup
	workerCount := (runtime.GOMAXPROCS(0) + 1) / 2
	if workerCount < 1 {
		workerCount = 1
	}

	// Split work into chunks
	chunkSize := (len(allDamages) + workerCount - 1) / workerCount
	results := make([][]struct {
		steamID uint64
		timing  float64
	}, workerCount)

	// Create channel for progress updates
	progressChan := make(chan int, workerCount)

	// Process chunks in parallel
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			start := workerID * chunkSize
			end := start + chunkSize
			if end > len(allDamages) {
				end = len(allDamages)
			}

			localResults := make([]struct {
				steamID uint64
				timing  float64
			}, 0, end-start)

			// Process each damage event in original order
			for _, dmg := range allDamages[start:end] {
				if lastVisibilityTick, ok := a.visibility.FindLastContinuousVisibilityStart(
					dmg.steamID, dmg.targetID, dmg.tick, tickData); ok {

					timeDelta := float64(dmg.tick-lastVisibilityTick) * msPerTick
					if timeDelta < 1000.0 {
						localResults = append(localResults, struct {
							steamID uint64
							timing  float64
						}{dmg.steamID, timeDelta})
					}
				}
				progressChan <- 1 // Signal that one damage event has been processed
			}
			results[workerID] = localResults
		}(i)
	}

	// Handle progress updates in a separate goroutine
	go func() {
		for range progressChan {
			progress.Add(1)
		}
	}()

	wg.Wait()
	close(progressChan)

	// Combine results in original order
	for _, workerResults := range results {
		for _, result := range workerResults {
			playerTimeToDamage[result.steamID] = append(
				playerTimeToDamage[result.steamID],
				result.timing,
			)
		}
	}

	// Calculate medians using gonum
	for steamID, timings := range playerTimeToDamage {
		if len(timings) == 0 {
			medianTimeToDamage[steamID] = 0
			continue
		}
		sort.Float64s(timings)
		medianTimeToDamage[steamID] = stat.Quantile(0.5, stat.Empirical, timings, nil)
	}

	return medianTimeToDamage, nil
}

// damageEvent represents a single damage event with its context
type damageEvent struct {
	tick     int
	steamID  uint64
	targetID uint64
	damage   collector.DamageDealt
}
