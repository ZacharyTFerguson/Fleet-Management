// Package places is the Gas Stations–only Canon Place catalog and marker drafts.
// Maintenance / shop / other types are rejected. OneStep miles are never invented here.
package places

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	TypeGas        = "001"
	BrandUnknown   = "UNKWN"
	GroupGas       = "Gas_Stations"
	DefaultTopTier = "A"
	DefaultGrade   = "A"
	ZoneRadiusM    = 25 // canopy / fill pad, not a shop lot
)

var brandHints = []struct {
	code string
	need string
}{
	{"WAWAA", "WAWA"},
	{"SHETZ", "SHETZ"},
	{"SHELL", "SHELL"},
	{"EXXON", "EXXON"},
	{"MOBIL", "MOBIL"},
	{"BPUSA", "BP"},
	{"CHEVR", "CHEVRON"},
	{"CIRCL", "CIRCLE K"},
	{"QTRAC", "QT "},
	{"SUNOC", "SUNOCO"},
	{"VALER", "VALERO"},
	{"MARAT", "MARATHON"},
	{"SPEED", "SPEEDWAY"},
	{"RACET", "RACE TRAC"},
	{"THNK", "THINK"},
	{"COSTC", "COSTCO"},
	{"SAMCL", "SAM'S"},
	{"7ELEV", "7-ELEVEN"},
	{"7ELEV", "7 ELEVEN"},
	{"7ELEV", "7ELEVEN"},
}

var codeRe = regexp.MustCompile(`^A\d{6}$`)

// Place is one gas station catalog row. Label is GeneralCode_Type_Branding_TopTier_TopTierGrade.
type Place struct {
	GeneralCode     string   `json:"general_code"`
	TypeCode        string   `json:"type_code"`
	BrandCode       string   `json:"brand_code"`
	TopTier         string   `json:"toptier"`
	TopTierGrade    string   `json:"toptier_grade"`
	Label           string   `json:"label"`
	Name            string   `json:"name"`
	Address         string   `json:"address"`
	MerchantID      string   `json:"merchant_id"`
	Lat             *float64 `json:"lat,omitempty"`
	Lng             *float64 `json:"lng,omitempty"`
	OneStepMarkerID string   `json:"onestep_marker_id,omitempty"`
	OneStepZoneID   string   `json:"onestep_zone_id,omitempty"`
	HoldReason      string   `json:"hold_reason,omitempty"`
	Source          string   `json:"source,omitempty"`
}

// Draft is the human-review payload before any OneStep write.
type Draft struct {
	Kind    string     `json:"kind"`
	Group   string     `json:"group"`
	Name    string     `json:"name"`
	Address string     `json:"address"`
	Lat     *float64   `json:"lat,omitempty"`
	Lng     *float64   `json:"lng,omitempty"`
	Zone    ZoneDraft  `json:"zone"`
	Canon   Place      `json:"canon"`
}

type ZoneDraft struct {
	Shape    string    `json:"shape"`
	RadiusM  int       `json:"radius_m"`
	Prefer   string    `json:"prefer"`
	ZoneType string    `json:"zone_type,omitempty"`
	Vertices []float64 `json:"vertices,omitempty"`
}

func LabelOf(general, typeCode, brand, top, grade string) string {
	return strings.ToUpper(general) + "_" + typeCode + "_" + brand + "_" + top + "_" + grade
}

// ParseCanonLabel reads GeneralCode_Type_Branding_TopTier_TopTierGrade. Gas (001) only.
func ParseCanonLabel(label string) (general, typeCode, brand, top, grade string, ok bool) {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(label)), "_")
	if len(parts) < 5 {
		return "", "", "", "", "", false
	}
	if !codeRe.MatchString(parts[0]) || parts[1] != TypeGas {
		return "", "", "", "", "", false
	}
	return parts[0], parts[1], parts[2], parts[3], parts[4], true
}

func NextGeneralCode(last string) (string, error) {
	if last == "" {
		return "A000001", nil
	}
	last = strings.ToUpper(strings.TrimSpace(last))
	if !codeRe.MatchString(last) {
		return "", fmt.Errorf("bad general_code %q", last)
	}
	n := 0
	for _, r := range last[1:] {
		n = n*10 + int(r-'0')
	}
	n++
	if n > 999999 {
		return "", fmt.Errorf("general_code overflow")
	}
	return fmt.Sprintf("A%06d", n), nil
}

func BrandFromName(name string) string {
	u := " " + strings.ToUpper(name) + " "
	for _, h := range brandHints {
		if strings.Contains(u, " "+h.need+" ") || strings.Contains(u, h.need) {
			return h.code
		}
	}
	return BrandUnknown
}

func GasOnly(typeCode string) error {
	if typeCode != TypeGas {
		return fmt.Errorf("OneStep place work is Gas Stations only (type 001); refusing type %q", typeCode)
	}
	return nil
}

func NewGasPlace(general, name, address, merchant, source string) (Place, error) {
	if err := GasOnly(TypeGas); err != nil {
		return Place{}, err
	}
	general = strings.ToUpper(strings.TrimSpace(general))
	if !codeRe.MatchString(general) {
		return Place{}, fmt.Errorf("bad general_code")
	}
	brand := BrandFromName(name)
	p := Place{
		GeneralCode:  general,
		TypeCode:     TypeGas,
		BrandCode:    brand,
		TopTier:      DefaultTopTier,
		TopTierGrade: DefaultGrade,
		Name:         strings.TrimSpace(name),
		Address:      strings.TrimSpace(address),
		MerchantID:   strings.TrimSpace(merchant),
		Source:       source,
	}
	p.Label = LabelOf(p.GeneralCode, p.TypeCode, p.BrandCode, p.TopTier, p.TopTierGrade)
	return p, nil
}

func DraftFor(p Place) Draft {
	return Draft{
		Kind:    "gas_station_marker",
		Group:   GroupGas,
		Name:    p.Label,
		Address: p.Address,
		Lat:     p.Lat,
		Lng:     p.Lng,
		Zone: ZoneDraft{
			Shape:   "circle",
			RadiusM: ZoneRadiusM,
			Prefer:  "canopy_pad",
		},
		Canon: p,
	}
}

var junkMerchant = regexp.MustCompile(`(?i)\b(TRACKER|OFFICE|HQ|MAINT|SHOP|REPAIR|LUBE|OIL CHANGE|DEALER)\b`)

// IsGasMerchant is a conservative gas-station filter. Shop/maintenance names are excluded.
func IsGasMerchant(name, address string) bool {
	n := strings.TrimSpace(name)
	if n == "" {
		return false
	}
	if junkMerchant.MatchString(n) {
		return false
	}
	if BrandFromName(n) != BrandUnknown {
		return true
	}
	// Fuel-looking leftover: has a street-ish address and no shop words.
	if strings.TrimSpace(address) == "" {
		return false
	}
	letters := 0
	for _, r := range n {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 3
}

func StationKey(name, address string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " ")) + "|" + strings.ToLower(strings.Join(strings.Fields(address), " "))
}
