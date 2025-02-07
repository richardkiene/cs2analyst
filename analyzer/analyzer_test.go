package analyzer

import (
	"math/rand"
	"testing"
	"time"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/collector"
	"github.com/stretchr/testify/assert"
)

// TestAnalyze verifies basic functionality
func TestAnalyze(t *testing.T) {
	analyzer, err := New()
	assert.NoError(t, err)

	// Basic test case
	tickData := map[int]map[uint64]collector.PlayerTickData{
		0: {
			1: {
				SteamID: 1,
				IsAlive: true,
				DamageDealtToPlayer: map[uint64]collector.DamageDealt{
					2: {HealthDamage: 100},
				},
			},
			2: {
				SteamID: 2,
				IsAlive: true,
			},
		},
	}

	result, err := analyzer.Analyze(tickData, 64.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestAnalyzeEdgeCases tests various edge cases and extreme scenarios
func TestAnalyzeEdgeCases(t *testing.T) {
	analyzer, err := New()
	assert.NoError(t, err)

	tests := []struct {
		name     string
		tickData map[int]map[uint64]collector.PlayerTickData
		tickRate float64
	}{
		{
			name: "Empty Game No Players",
			tickData: map[int]map[uint64]collector.PlayerTickData{
				0: {},
				1: {},
			},
			tickRate: 64,
		},
		{
			name: "Single Player No Damage",
			tickData: map[int]map[uint64]collector.PlayerTickData{
				0: {
					1: {
						SteamID:             1,
						IsAlive:             true,
						DamageDealtToPlayer: make(map[uint64]collector.DamageDealt),
					},
				},
			},
			tickRate: 64,
		},
		{
			name:     "All Players Dead",
			tickData: createAllDeadPlayersData(),
			tickRate: 64,
		},
		{
			name:     "All Players Blinded",
			tickData: createAllBlindedPlayersData(),
			tickRate: 64,
		},
		{
			name:     "Simultaneous Damage Events",
			tickData: createSimultaneousDamageData(),
			tickRate: 64,
		},
		{
			name:     "Players Appearing and Disappearing",
			tickData: createIntermittentPlayersData(),
			tickRate: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzer.Analyze(tt.tickData, tt.tickRate)
			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// TestAnalyzePerformance runs performance-focused test cases
func TestAnalyzePerformance(t *testing.T) {
	analyzer, err := New()
	assert.NoError(t, err)

	tests := []struct {
		name       string
		numTicks   int
		numPlayers int
		damageFreq float64
		tickRate   float64
	}{
		{"Small Game", 100, 2, 0.1, 64},
		{"Medium Game", 500, 5, 0.05, 64},
		{"Large Game", 1000, 10, 0.02, 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tickData := generateTestData(tt.numTicks, tt.numPlayers, tt.damageFreq)
			result, err := analyzer.Analyze(tickData, tt.tickRate)
			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// Test data generator functions
func createAllDeadPlayersData() map[int]map[uint64]collector.PlayerTickData {
	data := make(map[int]map[uint64]collector.PlayerTickData)
	data[0] = make(map[uint64]collector.PlayerTickData)

	for i := uint64(1); i <= 5; i++ {
		data[0][i] = collector.PlayerTickData{
			SteamID: i,
			IsAlive: false,
			DamageDealtToPlayer: map[uint64]collector.DamageDealt{
				i + 1: {HealthDamage: 100},
			},
		}
	}
	return data
}

func createAllBlindedPlayersData() map[int]map[uint64]collector.PlayerTickData {
	data := make(map[int]map[uint64]collector.PlayerTickData)
	data[0] = make(map[uint64]collector.PlayerTickData)

	for i := uint64(1); i <= 5; i++ {
		data[0][i] = collector.PlayerTickData{
			SteamID:   i,
			IsAlive:   true,
			IsBlinded: true,
			DamageDealtToPlayer: map[uint64]collector.DamageDealt{
				i + 1: {HealthDamage: 100},
			},
		}
	}
	return data
}

func createSimultaneousDamageData() map[int]map[uint64]collector.PlayerTickData {
	data := make(map[int]map[uint64]collector.PlayerTickData)
	data[0] = make(map[uint64]collector.PlayerTickData)

	for i := uint64(1); i <= 4; i++ {
		data[0][i] = collector.PlayerTickData{
			SteamID:             i,
			IsAlive:             true,
			Position:            r3.Vector{X: float64(i) * 100, Y: float64(i) * 100, Z: 64},
			DamageDealtToPlayer: make(map[uint64]collector.DamageDealt),
		}

		for j := uint64(1); j <= 4; j++ {
			if i != j {
				data[0][i].DamageDealtToPlayer[j] = collector.DamageDealt{HealthDamage: 25}
			}
		}
	}

	return data
}

func createIntermittentPlayersData() map[int]map[uint64]collector.PlayerTickData {
	data := make(map[int]map[uint64]collector.PlayerTickData)

	for tick := 0; tick < 10; tick++ {
		data[tick] = make(map[uint64]collector.PlayerTickData)

		// Player 1 always present
		data[tick][1] = collector.PlayerTickData{
			SteamID:             1,
			IsAlive:             true,
			DamageDealtToPlayer: make(map[uint64]collector.DamageDealt),
		}

		// Player 2 intermittent
		if tick%2 == 0 {
			data[tick][2] = collector.PlayerTickData{
				SteamID:             2,
				IsAlive:             true,
				DamageDealtToPlayer: make(map[uint64]collector.DamageDealt),
			}
		}

		if tick == 5 {
			data[tick][1].DamageDealtToPlayer[2] = collector.DamageDealt{HealthDamage: 50}
		}
	}

	return data
}

func generateTestData(numTicks, numPlayers int, damageFrequency float64) map[int]map[uint64]collector.PlayerTickData {
	rand.Seed(time.Now().UnixNano())
	tickData := make(map[int]map[uint64]collector.PlayerTickData)

	for tick := 0; tick < numTicks; tick++ {
		tickData[tick] = make(map[uint64]collector.PlayerTickData)
		for playerID := uint64(1); playerID <= uint64(numPlayers); playerID++ {
			playerData := collector.PlayerTickData{
				SteamID:             playerID,
				IsAlive:             true,
				Position:            r3.Vector{X: rand.Float64() * 1000, Y: rand.Float64() * 1000, Z: rand.Float64() * 100},
				DamageDealtToPlayer: make(map[uint64]collector.DamageDealt),
			}

			if rand.Float64() < damageFrequency {
				targetID := uint64(rand.Intn(numPlayers) + 1)
				if targetID != playerID {
					playerData.DamageDealtToPlayer[targetID] = collector.DamageDealt{
						HealthDamage: rand.Intn(100) + 1,
					}
				}
			}

			tickData[tick][playerID] = playerData
		}
	}
	return tickData
}

// BenchmarkAnalyze runs benchmarks for different scenarios
func BenchmarkAnalyze(b *testing.B) {
	analyzer, err := New()
	if err != nil {
		b.Fatal(err)
	}

	benchmarks := []struct {
		name       string
		numTicks   int
		numPlayers int
		damageFreq float64
		tickRate   float64
	}{
		{"Small Game", 100, 2, 0.1, 64},
		{"Medium Game", 500, 5, 0.05, 64},
		{"Large Game", 1000, 10, 0.02, 128},
	}

	for _, bb := range benchmarks {
		tickData := generateTestData(bb.numTicks, bb.numPlayers, bb.damageFreq)

		b.Run(bb.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, err := analyzer.Analyze(tickData, bb.tickRate)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
