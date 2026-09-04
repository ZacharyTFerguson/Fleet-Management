package syncsupabase

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

// SetHold leaves last_reading_* in sqlite on purpose (history, LOWER_READING_REFUSED).
// The exporter blanks them on HOLD (export.WriteCSV). The Oil Desk mirror and the
// shared fleet_cars table are read by clients that never see that rule, so a HOLD
// row here must not carry a live-looking Last Reading either. Last oil is not a
// reading and stays.
func TestFromCarsBlanksStaleReadingOnHold(t *testing.T) {
	hold := model.HoldNoDriveStop
	stale := 180312
	oilM := 179598
	readAt := time.Date(2026, 8, 30, 14, 0, 0, 0, time.UTC)
	src := model.SourceFuelDetails
	rows := FromCars([]model.Car{{
		PDIID:             "PDI-0001",
		EFleetsID:         "26LSZW",
		Nickname:          "Bing-2",
		LastOilMiles:      &oilM,
		LastReadingMiles:  &stale,
		LastReadingAt:     &readAt,
		LastReadingSource: &src,
		HoldReason:        &hold,
	}})
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	r := rows[0]
	if r.HoldReason == nil || *r.HoldReason != hold {
		t.Fatalf("hold_reason must survive: %+v", r.HoldReason)
	}
	if r.LastOilMiles == nil || *r.LastOilMiles != oilM {
		t.Fatalf("last oil is not a reading; keep it: %+v", r.LastOilMiles)
	}
	if r.LastReadingMiles != nil {
		t.Fatalf("HOLD row exposes stale last_reading_miles=%d as current odo", *r.LastReadingMiles)
	}
	if r.LastReadingAt != nil || r.LastReadingSource != nil {
		t.Fatalf("HOLD row exposes stale last_reading_at/source: %v %v", r.LastReadingAt, r.LastReadingSource)
	}
}
