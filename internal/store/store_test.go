package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
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

func TestUpsertCarAllocatesPastPDIConflictsAndGaps(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pdi-gaps.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{PDIID: "PDI-0002", EFleetsID: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "B"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCar(ctx, model.Car{PDIID: "PDI-0002", EFleetsID: "C"}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "PDI-0002", "B": "PDI-0003", "C": "PDI-0004"}
	for efleetsID, pdiID := range want {
		car, err := s.CarByEFleets(ctx, efleetsID)
		if err != nil {
			t.Fatal(err)
		}
		if car.PDIID != pdiID {
			t.Fatalf("%s PDI %s want %s", efleetsID, car.PDIID, pdiID)
		}
	}
}


func TestOpenAppliesAdditiveMigrations(t *testing.T) {
	p := filepath.Join(t.TempDir(), "migrations.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, query := range []string{
		`SELECT card_id FROM card_transactions LIMIT 0`,
		`SELECT card_id FROM card_pairings LIMIT 0`,
		`SELECT linked_car_pdi_id, active, retired_at, last_synced_at FROM onestep_devices LIMIT 0`,
	} {
		rows, err := s.db.Query(query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeviceRefreshPreservesFactoryIDPairing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "device.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1"}); err != nil {
		t.Fatal(err)
	}
	carID := "CAR1"
	if err := s.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID:          "FACT1",
		DeviceID:           "DEV1",
		DisplayName:        "old label",
		LinkedCarEFleetsID: &carID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID:   "FACT1",
		DeviceID:    "DEV2",
		DisplayName: "new label",
	}); err != nil {
		t.Fatal(err)
	}
	devices, err := s.ListDevicesForCar(ctx, carID)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].FactoryID != "FACT1" || devices[0].DeviceID != "DEV2" {
		t.Fatalf("live refresh erased factory_id pairing: %+v", devices)
	}
}

func TestConcurrentUpsertCarUniquePDI(t *testing.T) {
	p := filepath.Join(t.TempDir(), "race.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	const n = 32
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("CAR%03d", i)
			errCh <- s.UpsertCar(ctx, model.Car{EFleetsID: id, Nickname: id})
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	cars, err := s.ListCars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cars) != n {
		t.Fatalf("cars %d want %d", len(cars), n)
	}
	seen := map[string]struct{}{}
	for _, c := range cars {
		if _, ok := seen[c.PDIID]; ok {
			t.Fatalf("duplicate pdi %s", c.PDIID)
		}
		seen[c.PDIID] = struct{}{}
	}
}

func TestTwoStoresConcurrentUpsertCarUniquePDI(t *testing.T) {
	p := filepath.Join(t.TempDir(), "two.sqlite")
	a, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	}()
	b, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	const n = 24
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		st := a
		if i%2 == 1 {
			st = b
		}
		go func(i int, st *Store) {
			defer wg.Done()
			id := fmt.Sprintf("TWO%03d", i)
			errCh <- st.UpsertCar(ctx, model.Car{EFleetsID: id, Nickname: id})
		}(i, st)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	cars, err := a.ListCars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cars) != n {
		t.Fatalf("cars %d want %d", len(cars), n)
	}
	seen := map[string]struct{}{}
	for _, c := range cars {
		if _, ok := seen[c.PDIID]; ok {
			t.Fatalf("duplicate pdi %s", c.PDIID)
		}
		seen[c.PDIID] = struct{}{}
	}
}
