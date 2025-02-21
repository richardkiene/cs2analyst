package visibility

import (
	"log"
	"math"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

func TestIsShooterPointingAtTarget(t *testing.T) {
	mapModel, err := LoadMapModel("")
	if err != nil {
		t.Fatalf("Failed to load map model: %v", err)
	}

	shooterModel, err := LoadPlayerModel("")
	if err != nil {
		t.Fatalf("Failed to load shooter model: %v", err)
	}
	targetModel, err := LoadPlayerModel("")
	if err != nil {
		t.Fatalf("Failed to load target model: %v", err)
	}

	tests := []struct {
		name     string
		shooter  types.PlayerTickData
		target   types.PlayerTickData
		expected bool
	}{
		{
			name: "Mirage -- Back Alley v Apps tick 84144",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: -1165.9681396484375, Y: 578.2523193359375, Z: -79.96875},
				ViewAngleX: 0.1654815673828125,
				ViewAngleY: 0.2176666259765625,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -438.730712890625, Y: 591.7733764648438, Z: -80.4307861328125},
			},
			expected: true,
		},
		/*{
		  "time": "2025-02-21T07:46:19.9408698-07:00",
		  "level": "DEBUG",
		  "msg": "Recorded damage event",
		  "tick": 136764,
		  "shooterSteamID": 76561199811297728,
		  "shooterPosition": { "X": -1652.548828125, "Y": 746.81103515625, "Z": -47.96875 },
		  "shooterViewAngleX": -75.15026092529297,
		  "shooterViewAngleY": 10.731582641601562,
		  "targetSteamID": 76561199002420143,
		  "targetPosition": { "X": -1515.4642333984375, "Y": 216.43341064453125, "Z": -166.96875 },
		  "targetViewAngleX": 106.509033203125,
		  "targetViewAngleY": -12.48699951171875
		}*/
		{
			name: "Mirage -- Apps to Arches @ tick 136764 -- Shooter looking through grate window",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 130.04379272460938, Y: 130.04379272460938, Z: -39.96875},
				ViewAngleX: -105.64865112304688,
				ViewAngleY: 5.7420654296875,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 16.8408145904541, Y: -2324.8759765625, Z: -39.96875},
			},
			expected: true,
		},
		/*{
			"time": "2025-02-21T08:01:57.3524557-07:00",
			"level": "DEBUG",
			"msg": "Recorded damage event",
			"tick": 22371,
			"shooterSteamID": "76561198970966860",
			"shooterPosition": {
				"X": 130.04379272460938,
				"Y": -1922.3170166015625,
				"Z": -39.96875
			},
			"shooterViewAngleX": -105.64865112304688,
			"shooterViewAngleY": 5.7420654296875,
			"targetSteamID": "76561198863796909",
			"targetPosition": {
				"X": 16.8408145904541,
				"Y": -2324.8759765625,
				"Z": -39.96875
			},
			"targetViewAngleX": 74.34481811523438,
			"targetViewAngleY": 3.966400146484375
		}*/
		{
			name: "Mirage -- Top Plywood to Palace elbow @ tick 22371 -- Shooter looking directly at target with wall left and doorway forward",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: -1652.548828125, Y: 746.81103515625, Z: -47.96875},
				ViewAngleX: -75.15026092529297,
				ViewAngleY: 10.731582641601562,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -1515.4642333984375, Y: 216.43341064453125, Z: -166.96875},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			computedYaw := math.Atan2(tt.target.Position.Y-tt.shooter.Position.Y, tt.target.Position.X-tt.shooter.Position.X) * (180.0 / math.Pi)
			computedYaw = NormalizeAngle(computedYaw)
			shooterYaw := NormalizeAngle(float64(tt.shooter.ViewAngleX))

			computedPitch := math.Atan2(tt.target.Position.Z-tt.shooter.Position.Z,
				math.Sqrt(math.Pow(tt.target.Position.X-tt.shooter.Position.X, 2)+math.Pow(tt.target.Position.Y-tt.shooter.Position.Y, 2))) * (180.0 / math.Pi)

			pitchDifference := math.Abs(computedPitch - float64(tt.shooter.ViewAngleY))
			yawDifference := math.Abs(computedYaw - shooterYaw)
			if yawDifference > 180 {
				yawDifference = 360 - yawDifference // Normalize shortest angular difference
			}

			log.Printf("%s: Computed Yaw: %.2f, Shooter Yaw: %.2f, Difference: %.2f", tt.name, computedYaw, shooterYaw, yawDifference)
			log.Printf("%s: Computed Pitch: %.2f, Shooter Pitch: %.2f, Difference: %.2f", tt.name, computedPitch, tt.shooter.ViewAngleY, pitchDifference)

			result := IsShooterPointingAtTarget(tt.shooter, tt.target, *shooterModel, *targetModel, *mapModel)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.name, tt.expected, result)
			}
		})
	}
}
