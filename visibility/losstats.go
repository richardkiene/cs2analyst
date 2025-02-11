package visibility

import (
	"fmt"
	"math"
)

// LosStats aggregates statistics about candidate LOS events.
type LosStats struct {
	TotalCandidates int
	BorderlineCount int
	SumDelta        float64
	MinDelta        float64
	MaxDelta        float64
}

// newLosStats returns an initialized LosStats.
func NewLosStats() *LosStats {
	return &LosStats{
		MinDelta: math.MaxFloat64,
		MaxDelta: -math.MaxFloat64,
	}
}

// updateLosStats updates the stats with a new candidate event delta.
func (s *LosStats) UpdateLosStats(delta float64, threshold float64) {
	s.TotalCandidates++
	// If the candidate delta is below the threshold, consider it borderline.
	if delta >= 0 && delta < threshold {
		s.BorderlineCount++
		s.SumDelta += delta
		if delta < s.MinDelta {
			s.MinDelta = delta
		}
		if delta > s.MaxDelta {
			s.MaxDelta = delta
		}
	}
}

// summary returns a formatted summary string.
func (s *LosStats) Summary() string {
	avgDelta := 0.0
	if s.BorderlineCount > 0 {
		avgDelta = s.SumDelta / float64(s.BorderlineCount)
	}
	return fmt.Sprintf("Total Candidates: %d, Borderline: %d, Avg Delta: %.3f, Min Delta: %.3f, Max Delta: %.3f",
		s.TotalCandidates, s.BorderlineCount, avgDelta, s.MinDelta, s.MaxDelta)
}
