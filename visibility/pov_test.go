package visibility

import (
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

func TestIsShooterPointingAtTarget(t *testing.T) {
	tests := []struct {
		name     string
		shooter  types.PlayerTickData
		target   types.PlayerTickData
		expected bool
	}{
		{
			name: "Shooter is directly facing target",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 0, // Facing east
				ViewAngleY: 0, // Level aim
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 10, Y: 0, Z: 0},
			},
			expected: true,
		},
		{
			name: "Shooter is slightly off but within FOV",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 10, // Slightly off center
				ViewAngleY: 0,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 10, Y: 0, Z: 0},
			},
			expected: true,
		},
		{
			name: "Shooter is facing away",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 180, // Facing west, away from target
				ViewAngleY: 0,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 10, Y: 0, Z: 0},
			},
			expected: false,
		},
		{
			name: "Shooter is looking too far up",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 0,
				ViewAngleY: 89, // Looking almost straight up
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 10, Y: 0, Z: 0},
			},
			expected: false,
		},
		{
			name: "Shooter is looking slightly below target but within FOV",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 0, Y: 0, Z: 0},
				ViewAngleX: 0,
				ViewAngleY: -10, // Looking slightly below the target
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 10, Y: 0, Z: 2},
			},
			expected: true,
		},
		{
			name: "Back Alley v Apps",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: -1165.9681396484375, Y: 578.2523193359375, Z: -79.96875},
				ViewAngleX: 0.1654815673828125,
				ViewAngleY: 0.2176666259765625, // Looking slightly below the target
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -438.730712890625, Y: 591.7733764648438, Z: -80.4307861328125},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsShooterPointingAtTarget(tt.shooter, tt.target)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.name, tt.expected, result)
			}
		})
	}
}
