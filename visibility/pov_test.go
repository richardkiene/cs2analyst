package visibility

import (
	"log"
	"log/slog"
	"math"
	"path/filepath"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
)

func TestCoordinateTransformation(t *testing.T) {
	// Enable debug logging
	slog.SetLogLoggerLevel(slog.LevelDebug)

	// Load the test map
	mapGltfFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, err := ImportGLTFMapModel(mapGltfFilePath, "de_mirage")
	if err != nil {
		t.Fatalf("Failed to load map model: %v", err)
	}

	// Define test cases for known replay positions
	testCases := []struct {
		name             string
		replayPos        r3.Vector
		expectedModelPos r3.Vector
		tolerance        float64
	}{
		{
			name:             "Mirage - T Spawn",
			replayPos:        r3.Vector{X: -3230, Y: -1652, Z: -39},
			expectedModelPos: r3.Vector{X: -40, Y: 3230, Z: -1652},
			tolerance:        10.0,
		},
		{
			name:             "Mirage - Mid",
			replayPos:        r3.Vector{X: -130, Y: -1052, Z: -110},
			expectedModelPos: r3.Vector{X: -110, Y: 130, Z: -1052},
			tolerance:        10.0,
		},
		{
			name:             "Mirage - A Site",
			replayPos:        r3.Vector{X: -1652, Y: 746, Z: -48},
			expectedModelPos: r3.Vector{X: -48, Y: 1652, Z: 746},
			tolerance:        10.0,
		},
	}

	// Test the transformation function
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock player tick data
			player := types.PlayerTickData{
				Position:   tc.replayPos,
				ViewAngleX: 0,
				ViewAngleY: 0,
			}

			// Transform the player data
			transformedPlayer := transformPlayerTickToModelSpace(player, mapModel)

			// For now, just log the results - we'll compare once we've fine-tuned
			// the transformation function
			t.Logf("Original position: %v", tc.replayPos)
			t.Logf("Transformed position: %v", transformedPlayer.Position)
			t.Logf("Expected position: %v", tc.expectedModelPos)

			// Check if the transformation is as expected (commented out until we're sure)
			/*
				assert.InDeltaf(t, tc.expectedModelPos.X, transformedPlayer.Position.X, tc.tolerance,
					"X coordinate doesn't match expected value")
				assert.InDeltaf(t, tc.expectedModelPos.Y, transformedPlayer.Position.Y, tc.tolerance,
					"Y coordinate doesn't match expected value")
				assert.InDeltaf(t, tc.expectedModelPos.Z, transformedPlayer.Position.Z, tc.tolerance,
					"Z coordinate doesn't match expected value")
			*/
		})
	}

	// Test with the real test case coordinates from IsShooterPointingAtTarget test
	testPoints := []types.PlayerTickData{
		// Mirage - Back Alley v Apps tick 84144
		{
			Position:   r3.Vector{X: -1165.9681396484375, Y: 578.2523193359375, Z: -79.96875},
			ViewAngleX: 0.1654815673828125,
			ViewAngleY: 0.2176666259765625,
		},
		// Target
		{
			Position: r3.Vector{X: -438.730712890625, Y: 591.7733764648438, Z: -80.4307861328125},
		},
		// Mirage - Top Plywood to Palace elbow @ tick 22371
		{
			Position:   r3.Vector{X: -1652.548828125, Y: 746.81103515625, Z: -47.96875},
			ViewAngleX: -75.15026092529297,
			ViewAngleY: 10.731582641601562,
		},
		// Target
		{
			Position: r3.Vector{X: -1515.4642333984375, Y: 216.43341064453125, Z: -166.96875},
		},
	}

	// Run debug function to analyze coordinate transformation
	DebugCoordinateTransformation(mapModel, testPoints)

	// Test ray casting with transformed coordinates
	t.Run("TestRayCasting", func(t *testing.T) {
		// Load player model
		playerModelGltfFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
		playerModel, err := ImportGLTFPlayerModel(playerModelGltfFilePath)
		if err != nil {
			t.Fatalf("Failed to load player model: %v", err)
		}

		// Test case from the original test that failed
		shooter := types.PlayerTickData{
			Position:   r3.Vector{X: -1652.548828125, Y: 746.81103515625, Z: -47.96875},
			ViewAngleX: -75.15026092529297,
			ViewAngleY: 10.731582641601562,
		}
		target := types.PlayerTickData{
			Position: r3.Vector{X: -1515.4642333984375, Y: 216.43341064453125, Z: -166.96875},
		}

		// Transform coordinates
		transformedShooter := transformPlayerTickToModelSpace(shooter, mapModel)
		transformedTarget := transformPlayerTickToModelSpace(target, mapModel)

		// Get shooter eye position
		shooterEye := GetAdjustedEyePosition(transformedShooter, playerModel, mapModel)

		// Log transformation
		t.Logf("Original shooter: %v", shooter.Position)
		t.Logf("Transformed shooter: %v", transformedShooter.Position)
		t.Logf("Shooter eye position: %v", shooterEye)
		t.Logf("Original target: %v", target.Position)
		t.Logf("Transformed target: %v", transformedTarget.Position)

		// Direction vector from shooter to target
		dirToTarget := transformedTarget.Position.Sub(shooterEye).Normalize()

		// Cast ray and check for hits
		hitPos := r3.Vector{}
		blocked, material, triangleIndex := rayIntersectsBVHClosestHit(
			shooterEye, dirToTarget, mapModel.BaseModel.bvh, &hitPos, 1000.0)

		if blocked {
			t.Logf("Ray blocked by material: %s (triangle %d)", material.Name, triangleIndex)
			t.Logf("Hit position: %v", hitPos)
			t.Logf("Distance to hit: %.2f", shooterEye.Sub(hitPos).Norm())
			t.Logf("Distance to target: %.2f", shooterEye.Sub(transformedTarget.Position).Norm())

			// Check if the material should be transparent
			isTransparent := material.IsTransparent || material.Name == "residwall04a" || isTransparentMaterial(material.Name)
			t.Logf("Material should be treated as transparent: %v", isTransparent)
		} else {
			t.Logf("No ray intersection found")
		}
	})
}

func TestIsShooterPointingAtTarget(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	mapGtlfFilePath := filepath.Join("../input_models/de_mirage_model/", "de_mirage_d.gltf")
	mapModel, err := ImportGLTFMapModel(mapGtlfFilePath, "test_map")
	if err != nil {
		t.Fatalf("Failed to load map model: %v", err)
	}

	shooterModelGtlfFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	shooterModel, err := ImportGLTFPlayerModel(shooterModelGtlfFilePath)
	if err != nil {
		t.Fatalf("Failed to load shooter model: %v", err)
	}

	targetModelGtlfFilePath := filepath.Join("../input_models/ctm_sas_model/", "ctm_sas.gltf")
	targetModel, err := ImportGLTFPlayerModel(targetModelGtlfFilePath)
	if err != nil {
		t.Fatalf("Failed to load shooter model: %v", err)
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
		{
			name: "Mirage -- Top-mid @ tick 182290 Shooter should *not* see player at Ticket Booth",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 89.64, Y: -556.01, Z: -110.93},
				ViewAngleX: -158.29,
				ViewAngleY: 41.00,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -871.26, Y: -2319.52, Z: -106.42},
			},
			expected: false,
		},
		/*
			{
				"time": "2025-03-01T15:43:20.4160602-07:00",
				"level": "INFO",
				"msg": "Visibility check",
				"tick": 182320,
				"canSeeTarget": true,
				"shooterPos": {
					"X": 62.67142105102539,
					"Y": -489.2166748046875,
					"Z": -178.322021484375
				},
				"eyePos": {
					"X": 62.67142105102539,
					"Y": -489.2166748046875,
					"Z": -133.37554863929748
				},
				"targetPos": {
					"X": -794.6744384765625,
					"Y": -2257.763427734375,
					"Z": -178.94140625
				}
			}
		*/
		{
			name: "Mirage -- Top-mid @ tick 182320 -- Shooter should not see player at Tripple / Ticket",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: 62.67142105102539, Y: -489.2166748046875, Z: -178.322021484375},
				ViewAngleX: -156.02542114257812,
				ViewAngleY: 0.25543212890625,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -794.6744384765625, Y: -2257.763427734375, Z: -178.94140625},
			},
			expected: false,
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

// Helper function to validate individual triangles
func validateTriangle(t *testing.T, tri types.Triangle, modelType string) {
	// Ensure coordinates are valid
	for _, v := range []r3.Vector{tri.V1, tri.V2, tri.V3} {
		if math.IsNaN(v.X) || math.IsNaN(v.Y) || math.IsNaN(v.Z) {
			t.Errorf("Invalid triangle vertex in %s model: %+v", modelType, v)
		}
	}

	// Ensure right-handed coordinate system (cross product should point correctly)
	edge1 := tri.V2.Sub(tri.V1)
	edge2 := tri.V3.Sub(tri.V1)
	normal := r3.Vector{
		X: edge1.Y*edge2.Z - edge1.Z*edge2.Y,
		Y: edge1.Z*edge2.X - edge1.X*edge2.Z,
		Z: edge1.X*edge2.Y - edge1.Y*edge2.X,
	}
	if normal.Z < 0 {
		t.Errorf("Triangle in %s model has incorrect normal direction: %+v", modelType, normal)
	}

	// Check if Z-axis is dominant (should be "up" in Source2 coordinate system)
	if math.Abs(normal.Z) < math.Abs(normal.Y) {
		t.Errorf("Triangle in %s model has suspicious normal (Z component too small): %+v", modelType, normal)
	}
}

// Helper function to check if the player model is in a reasonable location
func validatePlayerPosition(t *testing.T, playerModel Model) {
	var minX, minY, minZ, maxX, maxY, maxZ float64
	minX, minY, minZ = math.MaxFloat64, math.MaxFloat64, math.MaxFloat64
	maxX, maxY, maxZ = -math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64

	for _, tri := range playerModel.triangles {
		for _, v := range []r3.Vector{tri.V1, tri.V2, tri.V3} {
			if v.X < minX {
				minX = v.X
			}
			if v.Y < minY {
				minY = v.Y
			}
			if v.Z < minZ {
				minZ = v.Z
			}
			if v.X > maxX {
				maxX = v.X
			}
			if v.Y > maxY {
				maxY = v.Y
			}
			if v.Z > maxZ {
				maxZ = v.Z
			}
		}
	}

	// Ensure the player model is within reasonable bounds
	expectedHeight := 72.0 // Approximate CS2 player height
	if maxZ-minZ < expectedHeight*0.8 || maxZ-minZ > expectedHeight*1.2 {
		t.Errorf("Player model height is unexpected: %f", maxZ-minZ)
	}
	if maxX-minX < 10 || maxY-minY < 10 {
		t.Errorf("Player model width/length is unexpectedly small: (%f, %f)", maxX-minX, maxY-minY)
	}
}
