package oil

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestScorePunchesExpectedVsGasCard(t *testing.T) {
	maint := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	p1 := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	p2 := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	p3 := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	in := BoxScoreIn{
		EFleetsID: "27VA15",
		HasMaint:  true,
		MaintOdo:  10000,
		MaintAt:   maint,
		HasDevice: true,
		Punches: []GasCardPunch{
			{EFleetsID: "27VA15", Recorded: 10120, At: p1},          // expected 10100 → overage 20
			{EFleetsID: "27VA15", Recorded: 10205, At: p2},          // expected 10200 → overage 5 (gap down)
			{EFleetsID: "27VA15", Recorded: 99999, At: p3, UnusualY: true}, // suspect, no trend
		},
		MilesToPunch: map[int64]float64{
			p1.Unix(): 100,
			p2.Unix(): 200,
			p3.Unix(): 300,
		},
	}
	out := ScorePunches(in)
	if len(out.Rows) != 3 {
		t.Fatalf("rows %d", len(out.Rows))
	}
	if out.Rows[0].Status != LedgerTrusted || out.Rows[0].Overage != 20 || out.Rows[0].Shortage != 0 {
		t.Fatalf("p1 %+v", out.Rows[0])
	}
	if out.Rows[0].Expected == nil || *out.Rows[0].Expected != 10100 {
		t.Fatalf("expected %v", out.Rows[0].Expected)
	}
	if out.Rows[0].Difference == nil || *out.Rows[0].Difference != 20 {
		t.Fatalf("signed difference recorded−expected: %v", out.Rows[0].Difference)
	}
	if out.Rows[1].Overage != 5 || out.Rows[1].Trend != TrendDown || !out.Rows[1].InTrend {
		t.Fatalf("p2 trend %+v", out.Rows[1])
	}
	if out.Rows[1].Difference == nil || *out.Rows[1].Difference != 5 {
		t.Fatalf("p2 difference %v", out.Rows[1].Difference)
	}
	if out.Rows[2].Status != LedgerSuspect || out.Rows[2].InTrend {
		t.Fatalf("suspect must stay out of trend: %+v", out.Rows[2])
	}
	if out.TrendTrusted != 2 || out.TrendDown != 1 || out.SumOverage != 25 {
		t.Fatalf("rollup %+v", out)
	}
	if len(out.AbsDiffSeries) != 2 || out.AbsDiffSeries[0] != 20 || out.AbsDiffSeries[1] != 5 {
		t.Fatalf("abs_diff series %+v", out.AbsDiffSeries)
	}
}

func TestScorePunchesShortageIsNegativeDifference(t *testing.T) {
	maint := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	p1 := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	out := ScorePunches(BoxScoreIn{
		HasMaint: true, MaintOdo: 10000, MaintAt: maint, HasDevice: true,
		Punches:      []GasCardPunch{{Recorded: 10080, At: p1}},
		MilesToPunch: map[int64]float64{p1.Unix(): 100}, // expected 10100
	})
	if out.Rows[0].Shortage != 20 || out.Rows[0].Overage != 0 {
		t.Fatalf("%+v", out.Rows[0])
	}
	if out.Rows[0].Difference == nil || *out.Rows[0].Difference != -20 {
		t.Fatalf("shortage must be negative difference: %v", out.Rows[0].Difference)
	}
}

func TestScorePunchesHoldWhenMilesMissing(t *testing.T) {
	at := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	out := ScorePunches(BoxScoreIn{
		HasMaint:  true,
		MaintOdo:  5000,
		MaintAt:   at.Add(-time.Hour),
		HasDevice: true,
		Punches:   []GasCardPunch{{Recorded: 5100, At: at}},
	})
	if out.Rows[0].Status != LedgerHold || out.Rows[0].HoldReason != model.HoldNoDriveStop {
		t.Fatalf("%+v", out.Rows[0])
	}
	if out.Rows[0].Expected != nil || out.Rows[0].InTrend {
		t.Fatal("must not invent expected miles")
	}
}

func TestScorePunchesHoldWhenMaintMissing(t *testing.T) {
	at := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	out := ScorePunches(BoxScoreIn{
		HasDevice: true,
		Punches:   []GasCardPunch{{Recorded: 5100, At: at}},
		MilesToPunch: map[int64]float64{at.Unix(): 100},
	})
	if out.Rows[0].Status != LedgerHold || out.Rows[0].Expected != nil || out.Rows[0].Difference != nil {
		t.Fatalf("missing maint must HOLD, not invent: %+v", out.Rows[0])
	}
}

func TestScorePunchesHoldWhenNoDevice(t *testing.T) {
	at := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	out := ScorePunches(BoxScoreIn{
		HasMaint: true, MaintOdo: 5000, MaintAt: at.Add(-time.Hour),
		Punches:      []GasCardPunch{{Recorded: 5100, At: at}},
		MilesToPunch: map[int64]float64{at.Unix(): 100},
	})
	if out.Rows[0].Status != LedgerHold || out.Rows[0].HoldReason != model.HoldNoDevice {
		t.Fatalf("%+v", out.Rows[0])
	}
	if out.Rows[0].Expected != nil || out.Rows[0].Difference != nil {
		t.Fatal("no device pairing must not invent expected")
	}
}

func TestScorePunchesNeverUsesZeroForMissingGPS(t *testing.T) {
	at := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	out := ScorePunches(BoxScoreIn{
		HasMaint: true, MaintOdo: 5000, MaintAt: at.Add(-time.Hour), HasDevice: true,
		Punches: []GasCardPunch{{Recorded: 5000, At: at}},
	})
	if out.Rows[0].Expected != nil {
		t.Fatal("missing drive-stop must not become expected=maint+0")
	}
}

func TestGoodMaintenanceSkipsAbandonedShop(t *testing.T) {
	shopAt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	fillAt := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	odo := 12000
	ros := []model.ShopRO{{EFleetsID: "X", Odometer: 80000, At: shopAt}}
	fills := []model.Fill{{EFleetsID: "X", Odometer: &odo, ProviderTransactionTime: fillAt}}
	if _, _, ok := GoodMaintenance(ros, fills); ok {
		t.Fatal("abandoned shop spike must not be the expected base")
	}
	ros[0].Odometer = 11900
	if odo2, _, ok := GoodMaintenance(ros, fills); !ok || odo2 != 11900 {
		t.Fatalf("good shop %d %v", odo2, ok)
	}
}
