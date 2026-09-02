package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestSQLiteRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	c, err := s.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if c.PDIID != "PDI-0001" {
		t.Fatalf("pdi %s", c.PDIID)
	}
	odo := 100000
	if err := s.UpsertFill(ctx, model.Fill{EFleetsID: "27TESTA", Odometer: &odo, ProviderTransactionTime: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFill(ctx, model.Fill{EFleetsID: "27TESTA", Odometer: &odo, ProviderTransactionTime: time.Now().UTC()}); err != nil {
		t.Fatal("idempotent fill")
	}
	if err := s.SetHold(ctx, "27TESTA", model.HoldUnusualY, "test"); err != nil {
		t.Fatal(err)
	}
	c, _ = s.CarByEFleets(ctx, "27TESTA")
	if c.HoldReason == nil || *c.HoldReason != model.HoldUnusualY {
		t.Fatalf("hold %+v", c)
	}
	if err := s.WriteLastReading(ctx, "27TESTA", 100010, time.Now().UTC(), model.SourceFuelDetails); err != nil {
		t.Fatal(err)
	}
	c, _ = s.CarByEFleets(ctx, "27TESTA")
	if c.LastReadingMiles == nil || *c.LastReadingMiles != 100010 {
		t.Fatal("write")
	}
	if c.HoldReason != nil {
		t.Fatal("hold should clear on write")
	}
}

func TestOpaquePDINoStatePrefix(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_ = s.UpsertCar(ctx, model.Car{EFleetsID: "A", Nickname: "CT1", Region: "CT"})
	c, _ := s.CarByEFleets(ctx, "A")
	if c.PDIID == "CT1" || len(c.PDIID) < 4 {
		t.Fatalf("%s", c.PDIID)
	}
}
