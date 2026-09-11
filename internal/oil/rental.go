package oil

import "strings"

// IsRentalLabel reports a OneStep display_name or DETAILS company-vehicle
// number that is a rental/office bucket (Rental 1, VA RENTAL 2, PDI Rental #1).
// These labels are never VA15/VA19 aliases and never a factory_id join key.
func IsRentalLabel(parts ...string) bool {
	for _, p := range parts {
		for _, tok := range splitNameTokens(p) {
			if tok == "rental" {
				return true
			}
		}
	}
	return false
}

// SkipDeviceCarJoin refuses a GPS box as a car pairing identity. Logistics
// personnel labels and rental-car display_name stickers are not factory_id
// joins. VIN rematch still uses exact 17-char OBD VIN = cars.vin only.
func SkipDeviceCarJoin(parts ...string) bool {
	return HasLogisticsPersonnel(parts...) || IsRentalLabel(parts...)
}

// RentalHolderKey is the office-bucket key for a rental-labeled punch or box.
// Prefers the DETAILS company vehicle number ("Rental 1") over a longer
// OneStep sticker ("NYC-6: Julian (PDI Rental #2)").
func RentalHolderKey(parts ...string) string {
	var fallback string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || !IsRentalLabel(p) {
			continue
		}
		if fallback == "" {
			fallback = p
		}
		// Short CVN-style labels beat GPS stickers that include a person name.
		if len(p) < len(fallback) {
			fallback = p
		}
	}
	return fallback
}
