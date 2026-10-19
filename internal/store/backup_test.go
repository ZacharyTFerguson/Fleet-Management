package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestApplyCarSnapshotRosterOnlyDoesNotWipeDest(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "snap.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 8, 1, 15, 4, 5, 0, time.UTC)
	if err := s.UpsertCar(ctx, model.Car{
		EFleetsID: "27TESTA", Nickname: "VA19", VIN: "1HGCM82633A004352", Region: "VA", IntervalMiles: 7500,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertOilChange(ctx, model.OilChange{EFleetsID: "27TESTA", Miles: 100500, Date: at, Source: "shop_ro"}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteLastReading(ctx, "27TESTA", 100010, at, model.SourceFuelDetails); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHold(ctx, "27TESTA", model.HoldNoDriveStop, "gps missing"); err != nil {
		t.Fatal(err)
	}

	// Roster-only incoming row: no last oil, no Last Reading, no HOLD, blank identity, interval 0.
	if err := s.ApplyCarSnapshot(ctx, model.Car{EFleetsID: "27TESTA"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if got.VIN != "1HGCM82633A004352" || got.Nickname != "VA19" || got.Region != "VA" {
		t.Fatalf("identity wiped: %+v", got)
	}
	if got.LastOilMiles == nil || *got.LastOilMiles != 100500 {
		t.Fatalf("last oil wiped: %+v", got.LastOilMiles)
	}
	if got.LastReadingMiles == nil || *got.LastReadingMiles != 100010 {
		t.Fatalf("last reading wiped: %+v", got.LastReadingMiles)
	}
	if got.HoldReason == nil || *got.HoldReason != model.HoldNoDriveStop {
		t.Fatalf("HOLD wiped: %+v", got.HoldReason)
	}
	if got.IntervalMiles != 7500 {
		t.Fatalf("interval reset to %d", got.IntervalMiles)
	}
}

func TestApplyCarSnapshotComputedClearsHold(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "snap-clear.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHold(ctx, "27TESTA", model.HoldNoDriveStop, "old"); err != nil {
		t.Fatal(err)
	}
	reading := 101200
	src := model.SourceFuelDetails
	if err := s.ApplyCarSnapshot(ctx, model.Car{
		EFleetsID:         "27TESTA",
		LastReadingMiles:  &reading,
		LastReadingAt:     &at,
		LastReadingSource: &src,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastReadingMiles == nil || *got.LastReadingMiles != 101200 {
		t.Fatalf("reading %+v", got.LastReadingMiles)
	}
	if got.HoldReason != nil {
		t.Fatalf("successful compute must clear dest HOLD, got %+v", got.HoldReason)
	}
}
