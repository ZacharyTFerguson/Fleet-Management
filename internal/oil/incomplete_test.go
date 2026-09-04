package oil

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

// Maintenance Detail rows with no RO Completed/Created date are stored with a
// zero timestamp (ParseShopROs). A zero second is not a trusted second: the
// formula is odo at a known second plus drive-stop miles since that second, and
// SyncOneStep skips a zero FillTime, so choosing it strands the car on
// NO_DRIVESTOP even though a dated fill and measured miles exist.
func TestUndatedShopRODoesNotReplaceDatedFillAnchor(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
		ShopROs: []model.ShopRO{{
			EFleetsID:    "CAR1",
			Odometer:     100010,
			At:           time.Time{},
			LocationName: "Shop (date missing in export)",
		}},
		Devices: []model.OneStepDevice{{FactoryID: "F1", DeviceID: "D1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(10), Miles: 12.4,
		}},
	}
	out := EvaluateHolds(in)
	if out.FillTime.IsZero() {
		t.Fatalf("zero-time RO became the trusted anchor: %+v", out)
	}
	if !out.FillTime.Equal(at(10)) || out.EnterpriseOdo != 100000 {
		t.Fatalf("dated fill must stay the anchor, got odo=%d at=%s holds=%v", out.EnterpriseOdo, out.FillTime, out.Holds)
	}
	if out.SkipWrite {
		t.Fatalf("measured miles exist after the dated fill; HOLD %v is stranding the car", out.Holds)
	}
	if out.Reading != 100012 {
		t.Fatalf("reading %d", out.Reading)
	}
}
