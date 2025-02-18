package weapons

import (
	"encoding/csv"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

// WeaponDetail represents a CS2 weapon with all its attributes
type WeaponDetail struct {
	ID                     common.EquipmentType
	Name                   string
	Price                  int
	KillAward              int
	Damage                 int
	Bullets                int
	ArmorPenetration       float64
	DamageFalloff          float64
	HeadshotMultiplier     float64
	FireRate               float64
	PenetrationPower       float64
	MagazineSize           int
	AmmoInReserve          int
	Mobility               int
	TaggingPower           float64
	BulletRange            int
	HoldToShoot            bool
	Tracers                string
	AccurateRangeStand     float64
	AccurateRangeCrouch    float64
	StandingInaccuracy     float64
	CrouchingInaccuracy    float64
	RunningInaccuracy      float64
	LadderInaccuracy       float64
	JumpApexInaccuracy     float64
	LandingInaccuracy      float64
	FiringInaccuracy       float64
	RecoveryTimeCrouch     float64
	RecoveryTimeStand      float64
	RecoilAmount           float64
	RecoilAngleVariance    float64
	RecoilAmountVariance   float64
	RecoilPattern          string
	FatalHeadshotRange     float64
	FatalHeadshotRangeHelm float64
}

// getEquipmentID maps weapon names to their corresponding EquipmentType
func getEquipmentID(name string) common.EquipmentType {
	nameLower := strings.ToLower(name)
	switch {
	// Pistols
	case strings.Contains(nameLower, "p2000"):
		return common.EqP2000
	case strings.Contains(nameLower, "glock"):
		return common.EqGlock
	case strings.Contains(nameLower, "p250"):
		return common.EqP250
	case strings.Contains(nameLower, "deagle"), strings.Contains(nameLower, "desert eagle"):
		return common.EqDeagle
	case strings.Contains(nameLower, "five-seven"), strings.Contains(nameLower, "fiveseven"):
		return common.EqFiveSeven
	case strings.Contains(nameLower, "dual berettas"), strings.Contains(nameLower, "dualies"):
		return common.EqDualBerettas
	case strings.Contains(nameLower, "tec-9"), strings.Contains(nameLower, "tec9"):
		return common.EqTec9
	case strings.Contains(nameLower, "cz"):
		return common.EqCZ
	case strings.Contains(nameLower, "usp"):
		return common.EqUSP
	case strings.Contains(nameLower, "r8"), strings.Contains(nameLower, "revolver"):
		return common.EqRevolver

	// SMGs
	case strings.Contains(nameLower, "mp7"):
		return common.EqMP7
	case strings.Contains(nameLower, "mp9"):
		return common.EqMP9
	case strings.Contains(nameLower, "bizon"):
		return common.EqBizon
	case strings.Contains(nameLower, "mac-10"), strings.Contains(nameLower, "mac10"):
		return common.EqMac10
	case strings.Contains(nameLower, "ump"):
		return common.EqUMP
	case strings.Contains(nameLower, "p90"):
		return common.EqP90
	case strings.Contains(nameLower, "mp5"):
		return common.EqMP5

	// Heavy
	case strings.Contains(nameLower, "sawed-off"), strings.Contains(nameLower, "sawedoff"):
		return common.EqSawedOff
	case strings.Contains(nameLower, "nova"):
		return common.EqNova
	case strings.Contains(nameLower, "mag-7"), strings.Contains(nameLower, "mag7"):
		return common.EqMag7
	case strings.Contains(nameLower, "xm1014"):
		return common.EqXM1014
	case strings.Contains(nameLower, "m249"):
		return common.EqM249
	case strings.Contains(nameLower, "negev"):
		return common.EqNegev

	// Rifles
	case strings.Contains(nameLower, "galil"):
		return common.EqGalil
	case strings.Contains(nameLower, "famas"):
		return common.EqFamas
	case strings.Contains(nameLower, "ak-47"), strings.Contains(nameLower, "ak47"):
		return common.EqAK47
	case strings.Contains(nameLower, "m4a4"):
		return common.EqM4A4
	case strings.Contains(nameLower, "m4a1"):
		return common.EqM4A1
	case strings.Contains(nameLower, "ssg"), strings.Contains(nameLower, "scout"):
		return common.EqScout
	case strings.Contains(nameLower, "sg553"), strings.Contains(nameLower, "sg 553"):
		return common.EqSG553
	case strings.Contains(nameLower, "aug"):
		return common.EqAUG
	case strings.Contains(nameLower, "awp"):
		return common.EqAWP
	case strings.Contains(nameLower, "scar-20"), strings.Contains(nameLower, "scar20"):
		return common.EqScar20
	case strings.Contains(nameLower, "g3sg1"):
		return common.EqG3SG1

	default:
		return common.EqUnknown
	}
}

// ParseCS2Weapons reads a CS2 weapons CSV file and returns WeaponData
// CSV Source: https://docs.google.com/spreadsheets/d/11tDzUNBq9zIX6_9Rel__fdAUezAQzSnh5AVYzCP060c/edit?usp=sharing
func ParseCS2Weapons(filename string) (map[common.EquipmentType]WeaponDetail, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Skip header row
	_, err = reader.Read()
	if err != nil {
		return nil, err
	}

	weapons := make(map[common.EquipmentType]WeaponDetail)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		weapon, err := parseWeaponRecord(record)
		if err != nil {
			return nil, err
		}

		// Set the equipment ID based on the weapon name
		weapon.ID = getEquipmentID(weapon.Name)

		weapons[weapon.ID] = weapon
	}

	return weapons, nil
}

// parseWeaponRecord converts a CSV record into a Weapon struct
func parseWeaponRecord(record []string) (WeaponDetail, error) {
	// Helper function to clean and parse numeric values
	cleanNumeric := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch r {
			case '$', '%', 'm', 'x':
				return -1
			default:
				return r
			}
		}, s)
	}

	// Helper function to parse percentage
	parsePercentage := func(s string) float64 {
		s = cleanNumeric(s)
		val, _ := strconv.ParseFloat(s, 64)
		return val / 100
	}

	// Helper function to parse float
	parseFloat := func(s string) float64 {
		s = cleanNumeric(s)
		val, _ := strconv.ParseFloat(s, 64)
		return val
	}

	// Helper function to parse integer
	parseInt := func(s string) int {
		s = cleanNumeric(s)
		s = strings.ReplaceAll(s, ",", "")
		val, _ := strconv.Atoi(s)
		return val
	}

	weapon := WeaponDetail{
		Name:                   record[0],
		Price:                  parseInt(record[1]),
		KillAward:              parseInt(record[2]),
		Damage:                 parseInt(record[3]),
		Bullets:                parseInt(record[4]),
		ArmorPenetration:       parsePercentage(record[5]),
		DamageFalloff:          parsePercentage(record[6]),
		HeadshotMultiplier:     parseFloat(record[7]),
		FireRate:               parseFloat(record[8]),
		PenetrationPower:       parsePercentage(record[9]),
		MagazineSize:           parseInt(record[10]),
		AmmoInReserve:          parseInt(record[11]),
		Mobility:               parseInt(record[12]),
		TaggingPower:           parsePercentage(record[13]),
		BulletRange:            parseInt(record[14]),
		HoldToShoot:            strings.ToLower(record[15]) == "yes",
		Tracers:                record[16],
		AccurateRangeStand:     parseFloat(record[17]),
		AccurateRangeCrouch:    parseFloat(record[18]),
		StandingInaccuracy:     parseFloat(record[19]),
		CrouchingInaccuracy:    parseFloat(record[20]),
		RunningInaccuracy:      parseFloat(record[21]),
		LadderInaccuracy:       parseFloat(record[22]),
		JumpApexInaccuracy:     parseFloat(record[23]),
		LandingInaccuracy:      parseFloat(record[24]),
		FiringInaccuracy:       parseFloat(record[25]),
		RecoveryTimeCrouch:     parseFloat(record[26]),
		RecoveryTimeStand:      parseFloat(record[27]),
		RecoilAmount:           parseFloat(record[28]),
		RecoilAngleVariance:    parseFloat(record[29]),
		RecoilAmountVariance:   parseFloat(record[30]),
		RecoilPattern:          record[31],
		FatalHeadshotRange:     parseFloat(record[32]),
		FatalHeadshotRangeHelm: parseFloat(record[33]),
	}

	return weapon, nil
}
