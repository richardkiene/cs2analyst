package visibility

import (
	"math"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

// IsShooterPointingAtTarget determines if the shooter is aiming at the target within a 100-degree FOV.
func IsShooterPointingAtTarget(shooter, target types.PlayerTickData) bool {
	// Compute the direction vector from shooter to target
	dirVector := r3.Vector{
		X: target.Position.X - shooter.Position.X,
		Y: target.Position.Y - shooter.Position.Y,
		Z: target.Position.Z - shooter.Position.Z,
	}

	// Compute the expected yaw (horizontal angle)
	expectedYaw := math.Atan2(dirVector.Y, dirVector.X) * (180.0 / math.Pi)
	if expectedYaw < 0 {
		expectedYaw += 360 // Normalize to [0, 360]
	}

	// Compute the expected pitch (vertical angle)
	horizontalDistance := math.Sqrt(dirVector.X*dirVector.X + dirVector.Y*dirVector.Y)
	expectedPitch := math.Atan2(dirVector.Z, horizontalDistance) * (180.0 / math.Pi)

	// Convert shooter’s pitch to match expected pitch format
	shooterPitch := float64(shooter.ViewAngleY)
	if shooterPitch > 90 {
		shooterPitch -= 360 // Convert to range [-90, 90]
	}

	// Define the allowed field of view (100-degree total, meaning ±50 degrees from center)
	fovThreshold := 50.0

	// Check if the yaw difference is within the field of view
	yawDifference := math.Abs(expectedYaw - float64(shooter.ViewAngleX))
	if yawDifference > 180 {
		yawDifference = 360 - yawDifference // Normalize to shortest angular difference
	}

	// Check if the pitch difference is within the field of view
	pitchDifference := math.Abs(expectedPitch - shooterPitch)

	// Shooter must be within both yaw and pitch threshold
	return yawDifference <= fovThreshold && pitchDifference <= fovThreshold
}
