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
				Position:   r3.Vector{X: -1652.548828125, Y: 746.81103515625, Z: -47.96875},
				ViewAngleX: -75.15026092529297,
				ViewAngleY: 10.731582641601562,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -1515.4642333984375, Y: 216.43341064453125, Z: -166.96875},
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
				Position:   r3.Vector{X: 130.04379272460938, Y: -1922.3170166015625, Z: -39.96875},
				ViewAngleX: -105.64865112304688,
				ViewAngleY: 5.7420654296875,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: 16.8408145904541, Y: -2324.8759765625, Z: -39.96875},
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
		/*
			{
				"time": "2025-03-07T20:19:44.7099471-07:00",
				"level": "DEBUG",
				"msg": "Recorded damage event",
				"tick": 29290,
				"shooterSteamID": "76561197991944713",
				"shooterPosition": {
					"X": -980.9349365234375,
					"Y": -2327.1201171875,
					"Z": -167.96875
				},
				"shooterViewAngleX": 168.4791259765625,
				"shooterViewAngleY": 9.140960693359375,
				"targetSteamID": "76561199811297728",
				"targetPosition": {
					"X": -1585.734130859375,
					"Y": -2191.7490234375,
					"Z": -253.405517578125
				},
				"targetViewAngleX": -10.427734375,
				"targetViewAngleY": -6.9893646240234375
			}
		*/
		{
			name: "Mirage -- Tick 29290 -- Unknown Vision Failure",
			shooter: types.PlayerTickData{
				Position:   r3.Vector{X: -980.9349365234375, Y: -2327.1201171875, Z: -167.96875},
				ViewAngleX: 168.4791259765625,
				ViewAngleY: 9.140960693359375,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{X: -1585.734130859375, Y: -2191.7490234375, Z: -253.405517578125},
			},
			expected: true,
		},
		/*
			{
				"time": "2025-03-10T18:34:17.1545313-07:00",
				"level": "DEBUG",
				"msg": "Recorded damage event",
				"tick": 94276,
				"shooterSteamID": "76561199214428404",
				"shooterPosition": {
					"X": -111.48406982421875,
					"Y": -1497.803955078125,
					"Z": -53.96875
				},
				"shooterViewAngleX": -134.33016967773438,
				"shooterViewAngleY": 12.628097534179688,
				"targetSteamID": "76561199811297728",
				"targetPosition": {
					"X": -519.3766479492188,
					"Y": -1885.4940185546875,
					"Z": -179.96875
				},
				"targetViewAngleX": -124.2502212524414,
				"targetViewAngleY": 0.7326507568359375
			}*/
		{
			name: "Mirage -- Tick 94276 -- Unknown Vision Failure",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -111.48406982421875,
					Y: -1497.803955078125,
					Z: -53.96875,
				},
				ViewAngleX: -134.33016967773438,
				ViewAngleY: 12.628097534179688,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -519.3766479492188,
					Y: -1885.4940185546875,
					Z: -179.96875,
				},
			},
			expected: true,
		},
		/*{
			"time": "2025-03-10T18:34:16.9876226-07:00",
			"level": "DEBUG",
			"msg": "Recorded damage event",
			"tick": 160327,
			"shooterSteamID": "76561199214428404",
			"shooterPosition": {
				"X": -1865.9390869140625,
				"Y": -624.790283203125,
				"Z": -167.96875
			},
			"shooterViewAngleX": 101.528076171875,
			"shooterViewAngleY": -1.093475341796875,
			"targetSteamID": "76561198970966860",
			"targetPosition": {
				"X": -2182.810546875,
				"Y": 827.809814453125,
				"Z": -123.00994873046875
			},
			"targetViewAngleX": -101.16519927978516,
			"targetViewAngleY": 3.62994384765625
		}*/
		{
			name: "Mirage -- Tick 160327-- Unknown Vision Failure",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -1865.9390869140625,
					Y: -624.790283203125,
					Z: -167.96875,
				},
				ViewAngleX: 101.528076171875,
				ViewAngleY: -1.093475341796875,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -2182.810546875,
					Y: 827.809814453125,
					Z: -123.00994873046875,
				},
				ViewAngleX: -101.16519927978516,
				ViewAngleY: 3.62994384765625,
			},
			expected: true,
		},
		// {"time":"2025-03-11T12:19:18.5859001-07:00","level":"INFO","msg":"TTD sample recorded","shooter":76561199002420143,"target":76561198237889474,"visibilityStartTick":109074,"damageTick":109075,"ttd":15.625}
		/*
			{
				"time": "2025-03-11T12:11:45.5369366-07:00",
				"level": "DEBUG",
				"msg": "Recorded damage event",
				"tick": 109075,
				"shooterSteamID": "76561199002420143",
				"shooterPosition": {
					"X": -1149.90234375,
					"Y": -612.816162109375,
					"Z": -167.96875
				},
				"shooterViewAngleX": -32.27027893066406,
				"shooterViewAngleY": 14.440841674804688,
				"targetSteamID": "76561198237889474",
				"targetPosition": {
					"X": -856.7142333984375,
					"Y": -788.9706420898438,
					"Z": -221.96875
				},
				"targetViewAngleX": 85.61611938476562,
				"targetViewAngleY": -4.375640869140625
			}*/
		{
			name: "Mirage -- Tick 109075 -- Limited Visibility Problem Window to bottom bench",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -1149.90234375,
					Y: -612.816162109375,
					Z: -167.96875,
				},
				ViewAngleX: -32.27027893066406,
				ViewAngleY: 14.440841674804688,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -856.7142333984375,
					Y: -788.9706420898438,
					Z: -221.96875,
				},
				ViewAngleX: 85.61611938476562,
				ViewAngleY: -4.375640869140625,
			},
			expected: true,
		},
		{
			name: "Mirage -- Tick 109040 -- Limited Visibility Problem Window to bottom bench",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -1177.66,
					Y: -674.00,
					Z: -168.13,
				},
				ViewAngleX: 12.69,
				ViewAngleY: 4.59,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -852.83,
					Y: -788.79,
					Z: -220.81,
				},
				ViewAngleX: 84.77,
				ViewAngleY: -3.16,
			},
			expected: true,
		},
		//{"time":"2025-03-12T12:35:29.7951345-07:00","level":"INFO","msg":"Skipped damage event - no visibility found","shooter":76561197991944713,"target":76561198970966860,"damageTick":165745}
		/*
			{
				"time": "2025-03-12T12:35:24.2584654-07:00",
				"level": "INFO",
				"msg": "Recorded damage event",
				"tick": 165745,
				"shooterSteamID": "76561197991944713",
				"shooterPosition": {
					"X": -516.4881591796875,
					"Y": -1721.796142578125,
					"Z": -177.74822998046875
				},
				"shooterViewAngleX": 11.045379638671875,
				"shooterViewAngleY": 1.7543792724609375,
				"targetSteamID": "76561198970966860",
				"targetPosition": {
					"X": 562.7240600585938,
					"Y": -1574.236328125,
					"Z": -263.96875
				},
				"targetViewAngleX": 177.5665283203125,
				"targetViewAngleY": -10.548934936523438
			}
		*/
		{
			name: "Mirage -- Tick 165745 -- A site to hidden player A main",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -516.4881591796875,
					Y: -1721.796142578125,
					Z: -177.74822998046875,
				},
				ViewAngleX: 11.045379638671875,
				ViewAngleY: 1.7543792724609375,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: 562.7240600585938,
					Y: -1574.236328125,
					Z: -263.96875,
				},
				ViewAngleX: 177.5665283203125,
				ViewAngleY: -10.548934936523438,
			},
			expected: false,
		},
		/*
					{"time":"2025-03-12T14:07:41.5383067-07:00","level":"INFO","msg":"Skipped damage event - no visibility found","shooter":76561199139199601,"target":76561198237889474,"damageTick":123516}
			{"time":"2025-03-12T14:07:41.8435284-07:00","level":"INFO","msg":"Skipped damage event - no visibility found","shooter":76561199139199601,"target":76561198237889474,"damageTick":123529}
			{"time":"2025-03-12T14:07:42.1680284-07:00","level":"INFO","msg":"Skipped damage event - no visibility found","shooter":76561199139199601,"target":76561198237889474,"damageTick":123542}
			{"time":"2025-03-12T14:07:42.508283-07:00","level":"INFO","msg":"Skipped damage event - no visibility found","shooter":76561199139199601,"target":76561198237889474,"damageTick":123554}
			{"time":"2025-03-12T14:07:42.8593305-07:00","level":"INFO","msg":"Skipped damage event - no visibility found","shooter":76561199139199601,"target":76561198237889474,"damageTick":123567}

			{"time":"2025-03-12T14:06:04.1746025-07:00","level":"INFO","msg":"Recorded bullet damage event","tick":123516,"shooterSteamID":"76561199139199601","shooterPosition":{"X":-1164.585693359375,"Y":-355.029541015625,"Z":-55.96875},"shooterViewAngleX":-91.15459442138672,"shooterViewAngleY":21.40789794921875,"targetSteamID":"76561198237889474","targetPosition":{"X":-876.6902465820312,"Y":94.74779510498047,"Z":-167.3389892578125},"targetViewAngleX":165.22579956054688,"targetViewAngleY":0.8514404296875,"shooterActiveWeapon.Type":405,"healthDamage":3}

		*/
		{
			name: "Mirage -- Tick 123516 -- Bullet Damage Event -- Wrong shooter",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -1164.585693359375,
					Y: -355.029541015625,
					Z: -55.96875,
				},
				ViewAngleX: -91.15459442138672,
				ViewAngleY: 21.40789794921875,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -876.6902465820312,
					Y: 94.74779510498047,
					Z: -167.3389892578125,
				},
				ViewAngleX: 165.22579956054688,
				ViewAngleY: 0.8514404296875,
			},
			expected: false,
		},
		{
			name: "Mirage -- Tick 123516 -- Bullet Damage Event -- Correct shooter",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -1424.292236328125,
					Y: 242.8572235107422,
					Z: -167.96875,
				},
				ViewAngleX: -12.0921630859375,
				ViewAngleY: 1.9274139404296875,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -876.6902465820312,
					Y: 94.74779510498047,
					Z: -167.3389892578125,
				},
				ViewAngleX: 165.22579956054688,
				ViewAngleY: 0.8514404296875,
			},
			expected: true,
		},
		{
			name: "Mirage Stairs to Main -- Tick 153202 -- Bullet Damage Event -- Head Barely Visible",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -768.6340942382812,
					Y: -1738.987548828125,
					Z: -179.46734619140625,
				},
				ViewAngleX: 7.985687255859375,
				ViewAngleY: 2.8049468994140625,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: 743.318115234375,
					Y: -1526.90771484375,
					Z: -263.9549560546875,
				},
				ViewAngleX: -172.77511596679688,
				ViewAngleY: -3.396148681640625,
			},
			expected: true,
		},
		/*{
			name: "Mirage -- Tick_105230_BulletDamage_UnknownWeapon_PlayerAsBot -- Probably bad test",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -261.43310546875,
					Y: -572.3412475585938,
					Z: -250.73382568359375,
				},
				ViewAngleX: -137.89044189453125,
				ViewAngleY: -2.989654541015625,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -1125.22021484375,
					Y: 784.2235107421875,
					Z: -79.96875,
				},
				ViewAngleX: -59.609413146972656,
				ViewAngleY: 1.2874603271484375,
			},
			expected: true,
		},*/
		/*
			{
				"time": "2025-03-14T15:09:46.4548483-07:00",
				"level": "INFO",
				"msg": "Recorded damage event",
				"tick": 188655,
				"shooterSteamID": "76561198863796909",
				"shooterPosition": {
					"X": -581.8778686523438,
					"Y": -1738.0191650390625,
					"Z": -179.3704833984375
				},
				"shooterViewAngleX": 95.38467407226562,
				"shooterViewAngleY": 4.001434326171875,
				"targetSteamID": "76561198237889474",
				"targetPosition": {
					"X": -655.5184326171875,
					"Y": -1005.0106811523438,
					"Z": -215.96875
				},
				"targetViewAngleX": -8.961410522460938,
				"targetViewAngleY": -5.090789794921875,
				"shooterActiveWeapon": "FAMAS",
				"isBulletDamage": true,
				"healthDamage": 19
			}
		*/
		{
			name: "Mirage -- tick 188655 -- A Site to Con limited vis",
			shooter: types.PlayerTickData{
				Position: r3.Vector{
					X: -581.8778686523438,
					Y: -1738.0191650390625,
					Z: -179.3704833984375,
				},
				ViewAngleX: 95.38467407226562,
				ViewAngleY: 4.001434326171875,
			},
			target: types.PlayerTickData{
				Position: r3.Vector{
					X: -655.5184326171875,
					Y: -1005.0106811523438,
					Z: -215.96875,
				},
				ViewAngleX: -8.961410522460938,
				ViewAngleY: -5.090789794921875,
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

			// TODO: both active smokes and tick should come from the test data
			activeSmokes := []types.ActiveSmoke{}
			tick := 1
			result := IsShooterPointingAtTarget(tt.shooter, tt.target, *shooterModel, *targetModel, *mapModel, activeSmokes, tick)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.name, tt.expected, result)
			}
		})
	}
}
