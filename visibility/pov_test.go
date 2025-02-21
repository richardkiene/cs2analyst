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
			name: "Back Alley v Apps tick 84144",
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
