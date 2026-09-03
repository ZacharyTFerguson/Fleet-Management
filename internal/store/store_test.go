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

func TestRemigrateReopenSameSQLite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "reopen.sqlite")
	s1, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s1.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open("sqlite", p)
	if err != nil {
		t.Fatalf("second Open (stock remigrate bug): %v", err)
	}
	defer s2.Close()
	c, err := s2.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if c.EFleetsID != "27TESTA" {
		t.Fatalf("%+v", c)
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

func TestConcurrentHoldAndReadingStayConsistent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hold-reading.sqlite")
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
	const n = 40
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				errCh <- s.SetHold(ctx, "CAR1", model.HoldNoDriveStop, "test")
				return
			}
			errCh <- s.WriteLastReading(ctx, "CAR1", 100000+i, time.Now().UTC(), model.SourceFuelDetails)
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	car, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	holds, err := s.OpenHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if car.HoldReason == nil && len(holds) != 0 {
		t.Fatalf("reading committed with %d open holds", len(holds))
	}
	if car.HoldReason != nil && len(holds) == 0 {
		t.Fatal("hold_reason committed without an open hold event")
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

func TestCardTxRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cards.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	gal := 10.0
	tx := model.CardTx{CardID: "CARD-MIX-99", At: time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC), StationName: "SHELL", RecordedEFleetsID: "27VA19", Gallons: &gal}
	if err := s.UpsertCardTx(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCardTx(ctx, tx); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListCardTxs(ctx, "CARD-MIX-99")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("idempotent upsert, got %d", len(got))
	}
}
