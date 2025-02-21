package analyzer

import (
	"fmt"
	"log/slog"
	"os"
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

	// Debug counters
	skippedEngagementWindow := 0
	skippedNoVisibility := 0
	skippedLongTTD := 0
	acceptedTTD := 0
	totalDamageEvents := 0

	a.logger.Info("Starting analysis",
		"tickRate", tickRate,
		"msPerTick", msPerTick,
		"tickTime", tickTime)

	// Collect damage events
	for tick, playerMap := range tickData {
		for steamID, player := range playerMap {
			for targetID, damage := range player.DamageDealtToPlayer {
				if damage.HealthDamage > 0 {

					a.logger.Debug("Recorded damage event",
						"tick", tick,
						"shooterSteamID", steamID,
						"shooterPosition", player.Position,
						"shooterViewAngleX", player.ViewAngleX,
						"shooterViewAngleY", player.ViewAngleY,
						"targetSteamID", targetID,
						"targetPosition", playerMap[targetID].Position,
						"targetViewAngleX", playerMap[targetID].ViewAngleX,
						"targetViewAngleY", playerMap[targetID].ViewAngleY,
					)

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
					// TODO: This is a debug hack for now
					totalDamageEvents++
					stats[steamID] = stat
				}
			}
		}
	}

	a.logger.Info("Initial damage events collected", "totalDamageEvents", totalDamageEvents)

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
				skippedEngagementWindow++
				a.logger.Debug("Skipped damage event - too close to last engagement",
					"shooter", pair.shooter,
					"target", pair.target,
					"damageTick", dmg.tick,
					"lastEngagementTick", lastEngagementTick,
					"delta", dmg.tick-lastEngagementTick)
				continue
			}

			stat := stats[pair.shooter]
			stat.visibilityChecks++
			stats[pair.shooter] = stat

			// Find when the shooter first saw the target before this damage
			if result, ok := a.visibility.FindLastContinuousVisibilityStart(
				pair.shooter, pair.target, dmg.tick, tickData); ok && result.IsValid {

				// Calculate TTD
				// Calculate time delta in milliseconds
				tickDelta := dmg.tick - result.StartTick
				if tickDelta < 0 {
					a.logger.Warn("Invalid tick delta - damage before visibility",
						"shooter", pair.shooter,
						"target", pair.target,
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
					playerTimeToDamage[pair.shooter] = append(
						playerTimeToDamage[pair.shooter],
						timeDelta,
					)
					stat := stats[pair.shooter]
					stat.ttdSamples++
					stats[pair.shooter] = stat

					acceptedTTD++
					lastEngagementTick = dmg.tick

					a.logger.Info("TTD sample recorded",
						"shooter", pair.shooter,
						"target", pair.target,
						"visibilityStartTick", result.StartTick,
						"damageTick", dmg.tick,
						"ttd", timeDelta)
				} else {
					skippedLongTTD++
					a.logger.Debug("Skipped damage event - TTD too long",
						"shooter", pair.shooter,
						"target", pair.target,
						"visibilityStartTick", result.StartTick,
						"damageTick", dmg.tick,
						"ttd", timeDelta)
				}
			} else {
				skippedNoVisibility++
				a.logger.Debug("Skipped damage event - no visibility found",
					"shooter", pair.shooter,
					"target", pair.target,
					"damageTick", dmg.tick)
			}
		}
	}

	a.logger.Info("TTD Analysis Complete",
		"totalDamageEvents", totalDamageEvents,
		"skippedEngagementWindow", skippedEngagementWindow,
		"skippedNoVisibility", skippedNoVisibility,
		"skippedLongTTD", skippedLongTTD,
		"acceptedTTD", acceptedTTD)

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

func (a *Analyzer) GenerateDebugVisualization(
	tick int,
	shooterSteamID uint64,
	targetSteamID uint64,
	tickData map[int]map[uint64]types.PlayerTickData,
) error {
	// Get tick data
	tickPlayers, exists := tickData[tick]
	if !exists {
		return fmt.Errorf("tick %d not found in data", tick)
	}

	// Get shooter data
	shooter, exists := tickPlayers[shooterSteamID]
	if !exists {
		return fmt.Errorf("shooter %d not found at tick %d", shooterSteamID, tick)
	}

	// Get target data
	target, exists := tickPlayers[targetSteamID]
	if !exists {
		return fmt.Errorf("target %d not found at tick %d", targetSteamID, tick)
	}

	if !shooter.IsAlive || !target.IsAlive {
		return fmt.Errorf("either shooter or target is not alive at tick %d", tick)
	}

	// Get hit points and debug info by calling CanSeeTarget
	_, hitPoints, debugInfo := visibility.CanSeeTarget(
		shooter,
		target,
		a.visibility.LosSystem.PlayerModel,
		a.visibility.LosSystem.MapModel,
		tick,
	)

	// Log detailed debug information
	a.logger.Info("Generating debug visualization",
		"tick", tick,
		"shooter", shooterSteamID,
		"target", targetSteamID,
		"numFOVChecks", len(debugInfo.FOVCheckResults),
		"numRayIntersections", len(debugInfo.RayIntersections),
		"numMarginResults", len(debugInfo.MarginResults))

	// Log FOV check results
	for i, fovResult := range debugInfo.FOVCheckResults {
		a.logger.Info("FOV Check Result",
			"index", i,
			"inFOV", fovResult.InFOV,
			"horizontalAngle", fovResult.HorizontalAngle,
			"verticalAngle", fovResult.VerticalAngle,
			"candidatePoint", fovResult.CandidatePoint)
	}

	// Log margin results
	for i, marginResult := range debugInfo.MarginResults {
		a.logger.Info("Margin Result",
			"index", i,
			"margin", marginResult.Margin,
			"tolerance", marginResult.Tolerance,
			"hitFound", marginResult.HitFound,
			"hitDistance", marginResult.HitDistance,
			"point", marginResult.Point)
	}

	// Create the debug visualization with all debug information
	err := visibility.CreateShooterCentricFOVUsingTargetDistance(
		tick,
		a.visibility.LosSystem.MapModel,
		a.visibility.LosSystem.PlayerModel,
		shooter,
		target,
		0.0,  // no extra padding needed for debug
		true, // include FOV cone
		hitPoints,
		debugInfo.RayIntersections, // Add ray intersections for visualization
		false,
	)

	if err != nil {
		return fmt.Errorf("failed to create debug visualization: %w", err)
	}

	// Create a debug file with specific FOV check results
	fileName := fmt.Sprintf("debug_visibility/fov_results_tick_%d.txt", tick)
	f, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create FOV results file: %w", err)
	}
	defer f.Close()

	// Write detailed FOV and visibility information
	fmt.Fprintf(f, "Debug Visibility Information for Tick %d\n", tick)
	fmt.Fprintf(f, "Shooter: %d, Target: %d\n\n", shooterSteamID, targetSteamID)
	fmt.Fprintf(f, "Shooter Position: %v\n", shooter.Position)
	fmt.Fprintf(f, "Shooter Eye Position: %v\n", visibility.GetEyePosition(shooter, a.visibility.LosSystem.PlayerModel))
	fmt.Fprintf(f, "Target Position: %v\n", target.Position)
	fmt.Fprintf(f, "Shooter View Angles: (%.2f, %.2f)\n\n", shooter.ViewAngleX, shooter.ViewAngleY)

	fmt.Fprintf(f, "FOV Check Results:\n")
	for i, result := range debugInfo.FOVCheckResults {
		fmt.Fprintf(f, "Point %d:\n", i)
		fmt.Fprintf(f, "  Candidate Point: %v\n", result.CandidatePoint)
		fmt.Fprintf(f, "  In FOV: %v\n", result.InFOV)
		fmt.Fprintf(f, "  Horizontal Angle: %.2f\n", result.HorizontalAngle)
		fmt.Fprintf(f, "  Vertical Angle: %.2f\n\n", result.VerticalAngle)
	}

	fmt.Fprintf(f, "Margin Results:\n")
	for i, result := range debugInfo.MarginResults {
		fmt.Fprintf(f, "Point %d:\n", i)
		fmt.Fprintf(f, "  Point: %v\n", result.Point)
		fmt.Fprintf(f, "  Margin: %.2f\n", result.Margin)
		fmt.Fprintf(f, "  Tolerance: %.2f\n", result.Tolerance)
		fmt.Fprintf(f, "  Hit Found: %v\n", result.HitFound)
		fmt.Fprintf(f, "  Hit Distance: %.2f\n\n", result.HitDistance)
	}

	return nil
}
