package app

import (
	"context"
	"testing"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

func TestRebuildBoxScoreMaintPlusDriveStop(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	maint := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	punch := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	if err := a.Store.UpsertCar(ctx, model.Car{EFleetsID: "27VA15", Nickname: "VA15"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertShopRO(ctx, model.ShopRO{ROID: "RO1", EFleetsID: "27VA15", Odometer: 10000, At: maint, ServiceDesc: "oil change"}); err != nil {
		t.Fatal(err)
	}
	odo := 10120
	if err := a.Store.UpsertFill(ctx, model.Fill{EFleetsID: "27VA15", Odometer: &odo, ProviderTransactionTime: punch, MerchantName: "SHELL"}); err != nil {
		t.Fatal(err)
	}
	link := "27VA15"
	if err := a.Store.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "FACT1", DeviceID: "dev1", LinkedCarEFleetsID: &link, Active: true}); err != nil {
		t.Fatal(err)
	}

	out, err := a.RebuildBoxScore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Vehicles) != 1 || len(out.Vehicles[0].Rows) != 1 {
		t.Fatalf("%+v", out)
	}
	row := out.Vehicles[0].Rows[0]
	if row.Status != oil.LedgerHold || row.HoldReason != model.HoldNoDriveStop || row.Expected != nil {
		t.Fatalf("missing miles must HOLD, not invent: %+v", row)
	}

	if err := a.Store.SaveDriveStopWindow(ctx, "FACT1", maint, punch, 100); err != nil {
		t.Fatal(err)
	}
	out, err = a.RebuildBoxScore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row = out.Vehicles[0].Rows[0]
	if row.Status != oil.LedgerTrusted || row.Expected == nil || *row.Expected != 10100 || row.Overage != 20 {
		t.Fatalf("expected maint+drive-stop: %+v", row)
	}
	if row.Difference == nil || *row.Difference != 20 {
		t.Fatalf("difference recorded−expected: %+v", row)
	}
	if out.SumOverage != 20 || out.TrustedN != 1 {
		t.Fatalf("rollup %+v", out)
	}
}

func TestListBoxScoreRefreshesStaleLedgerAfterMeasuredWindow(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	maint := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	punch := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	if err := a.Store.UpsertCar(ctx, model.Car{EFleetsID: "27VA15", Nickname: "VA15"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertShopRO(ctx, model.ShopRO{
		ROID: "RO1", EFleetsID: "27VA15", Odometer: 10000, At: maint,
	}); err != nil {
		t.Fatal(err)
	}
	odo := 10120
	if err := a.Store.UpsertFill(ctx, model.Fill{
		EFleetsID: "27VA15", Odometer: &odo, ProviderTransactionTime: punch,
	}); err != nil {
		t.Fatal(err)
	}
	link := "27VA15"
	if err := a.Store.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT1", DeviceID: "dev1", LinkedCarEFleetsID: &link, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	first, err := a.ListBoxScore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.HoldN != 1 || first.Vehicles[0].Rows[0].Expected != nil {
		t.Fatalf("missing drive-stop must cache a HOLD, got %+v", first)
	}

	if err := a.Store.SaveDriveStopWindow(ctx, "FACT1", maint, punch, 100); err != nil {
		t.Fatal(err)
	}
	refreshed, err := a.ListBoxScore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row := refreshed.Vehicles[0].Rows[0]
	if row.Status != oil.LedgerTrusted || row.Expected == nil || *row.Expected != 10100 {
		t.Fatalf("desk returned stale cached HOLD after measured miles arrived: %+v", row)
	}
	if row.Recorded != 10120 || row.Difference == nil || *row.Difference != 20 {
		t.Fatalf("recorded must be gas card; expected must be maint + drive-stop: %+v", row)
	}
}

func TestRebuildBoxScoreRowsNewestFirst(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	maint := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	older := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	if err := a.Store.UpsertCar(ctx, model.Car{EFleetsID: "27VA15", Nickname: "VA15"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertShopRO(ctx, model.ShopRO{
		ROID: "RO1", EFleetsID: "27VA15", Odometer: 10000, At: maint,
	}); err != nil {
		t.Fatal(err)
	}
	odoOld := 10120
	odoNew := 10205
	if err := a.Store.UpsertFill(ctx, model.Fill{EFleetsID: "27VA15", Odometer: &odoOld, ProviderTransactionTime: older}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertFill(ctx, model.Fill{EFleetsID: "27VA15", Odometer: &odoNew, ProviderTransactionTime: newer}); err != nil {
		t.Fatal(err)
	}
	link := "27VA15"
	if err := a.Store.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "FACT1", DeviceID: "dev1", LinkedCarEFleetsID: &link, Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SaveDriveStopWindow(ctx, "FACT1", maint, older, 100); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SaveDriveStopWindow(ctx, "FACT1", maint, newer, 200); err != nil {
		t.Fatal(err)
	}

	out, err := a.RebuildBoxScore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Vehicles) != 1 || len(out.Vehicles[0].Rows) != 2 {
		t.Fatalf("%+v", out)
	}
	if !out.Vehicles[0].Rows[0].PunchAt.Equal(newer) || !out.Vehicles[0].Rows[1].PunchAt.Equal(older) {
		t.Fatalf("desk rows must be newest-first %+v", out.Vehicles[0].Rows)
	}
}
