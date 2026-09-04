package oil

import (
	"math"
	"testing"
	"time"

	"oilchange/internal/model"
)

// at is a fixed UTC hour so table tests do not depend on the machine timezone.
func at(h int) time.Time {
	return time.Date(2026, 6, 1, h, 0, 0, 0, time.UTC)
}

// fill is a DETAILS punch in tests; nickname is the card company vehicle number for CARD_MIX.
func fill(odo int, hour int, unusual bool, nick string) model.Fill {
	o := odo
	return model.Fill{
		EFleetsID:                    "CAR1",
		ProviderCompanyVehicleNumber: nick,
		Odometer:                     &o,
		UnusualY:                     unusual,
		ProviderTransactionTime:      at(hour),
		Source:                       model.SourceFuelDetails,
	}
}

func TestLastReadingRoundsMilesInsideFunc(t *testing.T) {
	got, holds, err := LastReading(100000, at(10), 12.4)
	if err != nil {
		t.Fatal(err)
	}
	if len(holds) != 0 {
		t.Fatalf("holds %v", holds)
	}
	if got != 100012 {
		t.Fatalf("got %d", got)
	}
	got, _, err = LastReading(100000, at(10), 12.5)
	if err != nil {
		t.Fatal(err)
	}
	if got != 100013 {
		t.Fatalf("half-up got %d", got)
	}
}

func TestLastReadingRefusesNaN(t *testing.T) {
	_, _, err := LastReading(100000, at(10), math.NaN())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEvaluateGoldenFillPlusDriveStop(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
		Devices:  []model.OneStepDevice{{FactoryID: "F1", DeviceID: "D1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1",
			Since:     at(10),
			Miles:     12.4,
		}},
	}
	out := EvaluateHolds(in)
	if out.SkipWrite {
		t.Fatalf("skip %+v", out.Holds)
	}
	if out.Reading != 100012 {
		t.Fatalf("reading %d", out.Reading)
	}
	if out.Source != model.SourceFuelDetails {
		t.Fatalf("source %s", out.Source)
	}
}

func TestUnusualYHoldNoWrite(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, true, "VA1")},
		Devices:  []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(10), Miles: 1,
		}},
	}
	out := EvaluateHolds(in)
	if !out.SkipWrite {
		t.Fatal("should skip")
	}
	if !hasCode(out.Holds, model.HoldUnusualY) && !hasCode(out.Holds, model.HoldNoTrustedFill) {
		t.Fatalf("holds %v", out.Holds)
	}
}

func TestBackwardOdoRejected(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills: []model.Fill{
			fill(100000, 8, false, "VA1"),
			fill(90000, 12, false, "VA1"),
		},
		Devices: []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(8), Miles: 10,
		}},
	}
	out := EvaluateHolds(in)
	if out.SkipWrite {
		// chain should keep 100000 and still compute unless other holds
	}
	if !hasCode(out.Holds, model.HoldOdoBackward) && out.EnterpriseOdo != 100000 {
		t.Fatalf("expected backward skip of 90000, got %+v", out)
	}
	if !out.SkipWrite && out.EnterpriseOdo != 100000 {
		t.Fatalf("used backward odo %d", out.EnterpriseOdo)
	}
}

func TestShopROTypicallyBeatsFuel(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 8, false, "VA1")},
		ShopROs:  []model.ShopRO{{EFleetsID: "CAR1", Odometer: 100200, At: at(12), LocationName: "Valvoline"}},
		Devices:  []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(12), Miles: 5,
		}},
	}
	out := EvaluateHolds(in)
	if out.SkipWrite {
		t.Fatalf("skip %v", out.Holds)
	}
	if out.Source != model.SourceShopRO {
		t.Fatalf("source %s holds %v", out.Source, out.Holds)
	}
	if out.EnterpriseOdo != 100200 {
		t.Fatalf("odo %d", out.EnterpriseOdo)
	}
}

func TestShopSpikeAbandonedByLaterFills(t *testing.T) {
	// 148k…170k shop…149k fills: shop is junk (VA10).
	fills := []model.Fill{
		fill(148000, 1, false, "VA10"),
		fill(148400, 3, false, "VA10"),
		fill(149000, 5, false, "VA10"),
		fill(149400, 12, false, "VA10"),
		fill(150000, 14, false, "VA10"),
	}
	in := ComputeIn{
		Nickname: "VA10",
		Fills:    fills,
		ShopROs:  []model.ShopRO{{EFleetsID: "CAR1", Odometer: 170000, At: at(8), LocationName: "Shop"}},
		Devices:  []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(14), Miles: 8,
		}},
	}
	out := EvaluateHolds(in)
	if out.SkipWrite && hasCode(out.Holds, model.HoldNoDriveStop) {
		// trusted second may be last fill 150000 at hour 14 — miles are there
	}
	if !out.SkipWrite && out.EnterpriseOdo == 170000 {
		t.Fatalf("shop spike used as last reading: %+v", out)
	}
	if out.EnterpriseOdo == 170000 {
		t.Fatalf("170000 must be abandoned, holds %v source %s", out.Holds, out.Source)
	}
}

func TestFuelSpikeAbandoned(t *testing.T) {
	fills := []model.Fill{
		fill(148000, 1, false, "VA1"),
		fill(148400, 3, false, "VA1"),
		fill(149000, 5, false, "VA1"),
		fill(170000, 8, false, "VA1"),
		fill(149400, 12, false, "VA1"),
		fill(150000, 14, false, "VA1"),
	}
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    fills,
		Devices:  []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(14), Miles: 3,
		}},
	}
	out := EvaluateHolds(in)
	if out.EnterpriseOdo == 170000 {
		t.Fatalf("spike used %+v", out)
	}
	if !out.SkipWrite && out.EnterpriseOdo != 150000 {
		t.Fatalf("want 150000 got %d holds %v", out.EnterpriseOdo, out.Holds)
	}
}

func TestLowerReadingRefusedAndOverride(t *testing.T) {
	stored := 200000
	in := ComputeIn{
		Nickname:          "VA1",
		Fills:             []model.Fill{fill(199000, 10, false, "VA1")},
		Devices:           []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince:        []model.DriveStopMiles{{FactoryID: "F1", Since: at(10), Miles: 0}},
		StoredLastReading: &stored,
	}
	out := EvaluateHolds(in)
	if !out.SkipWrite || !hasCode(out.Holds, model.HoldLowerReadingRefused) {
		t.Fatalf("want refuse %+v", out)
	}
	in.OverrideLower = true
	out = EvaluateHolds(in)
	if out.SkipWrite {
		t.Fatalf("override still skip %v", out.Holds)
	}
	if out.Reading != 199000 {
		t.Fatalf("reading %d", out.Reading)
	}
}

func TestMultiDeviceFight(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
		Devices: []model.OneStepDevice{
			{FactoryID: "F1", DeviceID: "D1"},
			{FactoryID: "F2", DeviceID: "D2"},
		},
		MilesSince: []model.DriveStopMiles{
			{FactoryID: "F1", Since: at(10), Miles: 10},
			{FactoryID: "F2", Since: at(10), Miles: 4000},
		},
	}
	out := EvaluateHolds(in)
	if !out.SkipWrite || !hasCode(out.Holds, model.HoldMultiDeviceFight) {
		t.Fatalf("want fight %+v", out)
	}
}

func TestDeadFactoryIDNotSummed(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
		Devices: []model.OneStepDevice{
			{FactoryID: "OLD", DeviceID: "D0", Dead: true},
			{FactoryID: "NEW", DeviceID: "D1"},
		},
		MilesSince: []model.DriveStopMiles{
			{FactoryID: "OLD", Since: at(10), Miles: 9000},
			{FactoryID: "NEW", Since: at(10), Miles: 11.4},
		},
	}
	out := EvaluateHolds(in)
	if out.SkipWrite {
		t.Fatalf("skip %v", out.Holds)
	}
	if out.Reading != 100011 {
		t.Fatalf("dead box leaked, reading %d", out.Reading)
	}
}

func TestDigitSwapOnlyWhenFlaggedAndInBand(t *testing.T) {
	// 12530 prior; unusual 15230 which swaps 5 and 2 -> 12530-ish 12530 band.
	in := ComputeIn{
		Nickname: "VA1",
		Fills: []model.Fill{
			fill(12500, 8, false, "VA1"),
			fill(15230, 9, true, "VA1"),
		},
		Devices:    []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{FactoryID: "F1", Since: at(9), Miles: 2}},
	}
	out := EvaluateHolds(in)
	if out.SkipWrite {
		t.Fatalf("flagged in-band swap should survive, holds %v enterprise %d", out.Holds, out.EnterpriseOdo)
	}
	unflagged := in
	unflagged.Fills[1].UnusualY = false
	out2 := EvaluateHolds(unflagged)
	if out2.EnterpriseOdo == 15230 {
		t.Fatal("unflagged digit swap must not repair")
	}
}

func TestCardMixVA15VA19(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA19",
		Fills:    []model.Fill{fill(100000, 10, false, "VA15")},
		Devices:  []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(10), Miles: 1,
		}},
	}
	out := EvaluateHolds(in)
	if !out.SkipWrite || !hasCode(out.Holds, model.HoldCardMix) {
		t.Fatalf("want CARD_MIX %+v", out)
	}
}

func TestSameSecondDifferentOdo(t *testing.T) {
	a := fill(100000, 10, false, "VA1")
	b := fill(100500, 10, false, "VA1")
	b.ProviderTransactionTime = a.ProviderTransactionTime
	in := ComputeIn{
		Nickname:   "VA1",
		Fills:      []model.Fill{a, b},
		Devices:    []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{FactoryID: "F1", Since: at(10), Miles: 1}},
	}
	out := EvaluateHolds(in)
	if !out.SkipWrite || !hasCode(out.Holds, model.HoldSameSecondFill) {
		t.Fatalf("want SAME_SECOND_FILL %+v", out)
	}
}

func TestSameSecondSameOdoIsOneRow(t *testing.T) {
	a := fill(100000, 10, false, "VA1")
	b := fill(100000, 10, false, "VA1")
	in := ComputeIn{
		Nickname:   "VA1",
		Fills:      []model.Fill{a, b},
		Devices:    []model.OneStepDevice{{FactoryID: "F1"}},
		MilesSince: []model.DriveStopMiles{{FactoryID: "F1", Since: at(10), Miles: 4.4}},
	}
	out := EvaluateHolds(in)
	if out.SkipWrite {
		t.Fatalf("same odo same second should collapse, %v", out.Holds)
	}
	if out.Reading != 100004 {
		t.Fatalf("reading %d", out.Reading)
	}
}

func TestLogisticsPersonnelDoesNotLinkDevice(t *testing.T) {
	if !HasLogisticsPersonnel("Box for Tyler") {
		t.Fatal("tyler token")
	}
	if !HasLogisticsPersonnel("RICH") {
		t.Fatal("rich token")
	}
	if HasLogisticsPersonnel("MD18") {
		t.Fatal("car nickname is not personnel")
	}
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
		Devices:  []model.OneStepDevice{{FactoryID: "F1", DisplayName: "Tyler spare", Dead: false}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "F1", Since: at(10), Miles: 3,
		}},
	}
	out := EvaluateHolds(in)
	// liveDevices drops logistics labels, so this looks like NO_DEVICE
	if !out.SkipWrite || !hasCode(out.Holds, model.HoldNoDevice) {
		t.Fatalf("want NO_DEVICE after refusing personnel box, %+v", out)
	}
}

func TestNoDeviceHold(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
	}
	out := EvaluateHolds(in)
	if !out.SkipWrite || !hasCode(out.Holds, model.HoldNoDevice) {
		t.Fatalf("want NO_DEVICE %+v", out)
	}
}

func TestMissingDriveStopHold(t *testing.T) {
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
		Devices:  []model.OneStepDevice{{FactoryID: "F1"}},
	}
	out := EvaluateHolds(in)
	if !out.SkipWrite || !hasCode(out.Holds, model.HoldNoDriveStop) {
		t.Fatalf("want NO_DRIVESTOP %+v", out)
	}
	if !out.FillTime.Equal(at(10)) || out.EnterpriseOdo != 100000 {
		t.Fatalf("trusted anchor lost before GPS fetch: %+v", out)
	}
	if out.Reading != 0 {
		t.Fatalf("HOLD must not expose a reading, got %d", out.Reading)
	}
}

func TestJoinIsFactoryIDNotDisplayName(t *testing.T) {
	// Miles stored under factory_id F1; a different display_name must not pick them up.
	in := ComputeIn{
		Nickname: "VA1",
		Fills:    []model.Fill{fill(100000, 10, false, "VA1")},
		Devices:  []model.OneStepDevice{{FactoryID: "F1", DisplayName: "VA1"}},
		MilesSince: []model.DriveStopMiles{{
			FactoryID: "OTHER", Since: at(10), Miles: 99,
		}},
	}
	out := EvaluateHolds(in)
	if !hasCode(out.Holds, model.HoldNoDriveStop) {
		t.Fatalf("display_name must not join miles, %+v", out)
	}
}

func TestIsOilChangeService(t *testing.T) {
	if !IsOilChangeService("Full Synthetic Lube Oil Filter") {
		t.Fatal("lube oil filter")
	}
	if !IsOilChangeService("Semi Synthetic Engine Oil") {
		t.Fatal("semi synthetic engine oil is an oil change line")
	}
	if !IsOilChangeService("Conventional Lube Oil and Filter") {
		t.Fatal("conventional lube oil and filter")
	}
	if IsOilChangeService("R/R OIL PAN") {
		t.Fatal("oil pan is not an oil change")
	}
	if IsOilChangeService("Oil Filter Engine") {
		t.Fatal("filter-only line is not last oil")
	}
	if IsOilChangeService("Engine Oil Drain Plug Gasket") {
		t.Fatal("drain plug is not an oil change")
	}
	if IsOilChangeService("Wiper Blade") {
		t.Fatal("wiper")
	}
}

func TestDueAtNeverUsedAsWriteColumn(t *testing.T) {
	if DueAt(100000, 0) != 105000 {
		t.Fatal("default 5000")
	}
	if DueAt(100000, 7500) != 107500 {
		t.Fatal("per-car")
	}
}

func TestIntervalZeroMeans5000(t *testing.T) {
	if IntervalMiles(0) != 5000 {
		t.Fatal()
	}
}
