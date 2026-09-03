package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
		func() {
			rows, err := s.db.QueryContext(context.Background(), query)
			if err != nil {
				t.Fatalf("%s: %v", query, err)
			}
			defer func() {
				if err := rows.Close(); err != nil {
					t.Error(err)
				}
			}()
		}()
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

// TestTwoStoresSetHoldVsWriteLastReadingPreservesReading is the cross-process
// shape of the HOLD race: a `sync --interval` tick and a manual `compute` each
// open their own Store on the same SQLite file, so only SQLite's write lock
// (not Store.mu) separates SetHold from WriteLastReading. A HOLD must never
// touch last_reading_*, and no snapshot may ever show a partial triple.
func TestTwoStoresSetHoldVsWriteLastReadingPreservesReading(t *testing.T) {
	p := filepath.Join(t.TempDir(), "two-hold.sqlite")
	holder, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	writer, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	reader, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ctx := context.Background()
	if err := writer.UpsertCar(ctx, model.Car{EFleetsID: "CAR1"}); err != nil {
		t.Fatal(err)
	}

	const n = 30
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	atFor := func(i int) time.Time { return base.Add(time.Duration(i) * time.Minute) }
	milesFor := func(i int) int { return 100000 + i }
	reasons := []string{model.HoldNoDevice, model.HoldNoDriveStop, model.HoldNoTrustedFill}

	// checkTriple rejects any snapshot whose last_reading_* columns are not
	// either all NULL or exactly one committed (miles, at, source) write.
	checkTriple := func(c *model.Car) error {
		if c.LastReadingMiles == nil && c.LastReadingAt == nil && c.LastReadingSource == nil {
			return nil
		}
		if c.LastReadingMiles == nil || c.LastReadingAt == nil || c.LastReadingSource == nil {
			return fmt.Errorf("partial last_reading triple: %+v", *c)
		}
		i := *c.LastReadingMiles - 100000
		if i < 0 || i >= n {
			return fmt.Errorf("unknown last_reading_miles %d", *c.LastReadingMiles)
		}
		if !c.LastReadingAt.Equal(atFor(i)) {
			return fmt.Errorf("miles %d paired with at %s, want %s", *c.LastReadingMiles, c.LastReadingAt, atFor(i))
		}
		if *c.LastReadingSource != model.SourceFuelDetails {
			return fmt.Errorf("source %q", *c.LastReadingSource)
		}
		return nil
	}

	errCh := make(chan error, 3*n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			errCh <- holder.SetHold(ctx, "CAR1", reasons[i%len(reasons)], fmt.Sprintf("tick %d", i%2))
		}(i)
		go func(i int) {
			defer wg.Done()
			errCh <- writer.WriteLastReading(ctx, "CAR1", milesFor(i), atFor(i), model.SourceFuelDetails)
		}(i)
		go func() {
			defer wg.Done()
			c, err := reader.CarByEFleets(ctx, "CAR1")
			if err != nil {
				errCh <- err
				return
			}
			errCh <- checkTriple(c)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	car, err := reader.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkTriple(car); err != nil {
		t.Fatal(err)
	}
	if car.LastReadingMiles == nil {
		t.Fatalf("%d WriteLastReading calls committed but the reading is NULL: %+v", n, *car)
	}
	holds, err := reader.OpenHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case car.HoldReason == nil && len(holds) != 0:
		t.Fatalf("reading committed with %d open holds", len(holds))
	case car.HoldReason != nil && len(holds) != 1:
		t.Fatalf("hold_reason %s with %d open events, want exactly 1", *car.HoldReason, len(holds))
	case car.HoldReason != nil && holds[0].Reason != *car.HoldReason:
		t.Fatalf("hold_reason %s but open event says %s", *car.HoldReason, holds[0].Reason)
	}
}

func TestSetHoldIdempotentDoesNotStackOpenEvents(t *testing.T) {
	p := filepath.Join(t.TempDir(), "stack.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHold(ctx, "CAR2", model.HoldCardMix, "other car"); err != nil {
		t.Fatal(err)
	}

	// Same reason + detail every tick: one open event, first `at` kept.
	for i := 0; i < 5; i++ {
		if err := s.SetHold(ctx, "CAR1", model.HoldNoDevice, "no live factory_id linked"); err != nil {
			t.Fatal(err)
		}
	}
	open := openFor(t, s, "CAR1")
	if len(open) != 1 || open[0].Reason != model.HoldNoDevice {
		t.Fatalf("5 identical SetHold calls left %d open events: %+v", len(open), open)
	}
	firstAt := open[0].At

	// Different detail, same reason: previous open is closed, still exactly one.
	if err := s.SetHold(ctx, "CAR1", model.HoldNoDevice, "box marked dead"); err != nil {
		t.Fatal(err)
	}
	open = openFor(t, s, "CAR1")
	if len(open) != 1 || open[0].Detail != "box marked dead" {
		t.Fatalf("changed detail should replace the open event: %+v", open)
	}

	// Different reason: still exactly one, and it matches cars.hold_reason.
	if err := s.SetHold(ctx, "CAR1", model.HoldNoDriveStop, ""); err != nil {
		t.Fatal(err)
	}
	open = openFor(t, s, "CAR1")
	car, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].Reason != model.HoldNoDriveStop || car.HoldReason == nil || *car.HoldReason != model.HoldNoDriveStop {
		t.Fatalf("hold_reason %v open=%+v", car.HoldReason, open)
	}
	if !open[0].At.After(firstAt) && !open[0].At.Equal(firstAt) {
		t.Fatalf("new hold event at %s predates first %s", open[0].At, firstAt)
	}

	// Legacy rows from builds that stacked: collapse to the oldest matching one.
	for i := 0; i < 3; i++ {
		if _, err := s.rawExec(ctx, `INSERT INTO hold_events (efleets_id, reason, detail, at, open) VALUES (?,?,?,?,TRUE)`,
			"CAR1", model.HoldNoDriveStop, "", time.Date(2026, 1, 1, i, 0, 0, 0, time.UTC).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	if len(openFor(t, s, "CAR1")) != 4 {
		t.Fatal("test setup: expected 4 stacked opens")
	}
	if err := s.SetHold(ctx, "CAR1", model.HoldNoDriveStop, ""); err != nil {
		t.Fatal(err)
	}
	open = openFor(t, s, "CAR1")
	if len(open) != 1 {
		t.Fatalf("legacy stack not collapsed: %+v", open)
	}

	// Another car's hold is untouched throughout.
	other := openFor(t, s, "CAR2")
	if len(other) != 1 || other[0].Reason != model.HoldCardMix {
		t.Fatalf("CAR2 hold disturbed: %+v", other)
	}
	total, err := s.OpenHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(total) != 2 {
		t.Fatalf("want one open hold per held car, got %d", len(total))
	}
}

func openFor(t *testing.T, s *Store, efleetsID string) []model.HoldEvent {
	t.Helper()
	all, err := s.OpenHolds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []model.HoldEvent
	for _, h := range all {
		if h.EFleetsID == efleetsID {
			out = append(out, h)
		}
	}
	return out
}

func TestSetHoldKeepsLastReadingTriple(t *testing.T) {
	p := filepath.Join(t.TempDir(), "keep.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 4, 9, 30, 0, 0, time.UTC)
	if err := s.WriteLastReading(ctx, "CAR1", 123456, at, model.SourceShopRO); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHold(ctx, "CAR1", model.HoldLowerReadingRefused, "computed 123000 is lower"); err != nil {
		t.Fatal(err)
	}
	c, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if c.HoldReason == nil || *c.HoldReason != model.HoldLowerReadingRefused {
		t.Fatalf("hold %+v", c.HoldReason)
	}
	if c.LastReadingMiles == nil || *c.LastReadingMiles != 123456 ||
		c.LastReadingAt == nil || !c.LastReadingAt.Equal(at) ||
		c.LastReadingSource == nil || *c.LastReadingSource != model.SourceShopRO {
		t.Fatalf("SetHold must not touch last_reading_*: %+v", *c)
	}
}

func TestClearHoldsClearsReasonAndEventsTogether(t *testing.T) {
	p := filepath.Join(t.TempDir(), "clear.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHold(ctx, "CAR1", model.HoldNoDevice, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearHolds(ctx, "CAR1"); err != nil {
		t.Fatal(err)
	}
	c, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if c.HoldReason != nil {
		t.Fatalf("hold_reason should be cleared with its events: %s", *c.HoldReason)
	}
	if len(openFor(t, s, "CAR1")) != 0 {
		t.Fatal("open events remain after ClearHolds")
	}
}

func TestHoldAndReadingWritesRefuseUnknownCarAndZero(t *testing.T) {
	p := filepath.Join(t.TempDir(), "guards.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.WriteLastReading(ctx, "GHOST", 1000, now, model.SourceFuelDetails); !errors.Is(err, ErrUnknownCar) {
		t.Fatalf("unknown car write: %v", err)
	}
	if err := s.SetHold(ctx, "GHOST", model.HoldNoDevice, ""); !errors.Is(err, ErrUnknownCar) {
		t.Fatalf("unknown car hold: %v", err)
	}
	if err := s.WriteLastReading(ctx, "CAR1", 0, now, model.SourceFuelDetails); err == nil {
		t.Fatal("0 miles must not be stored as a reading")
	}
	if err := s.WriteLastReading(ctx, "CAR1", 1000, now, "onestep_odometer"); err == nil {
		t.Fatal("device odometer is not a legal last_reading_source")
	}
	if err := s.SetHold(ctx, "CAR1", "", ""); err == nil {
		t.Fatal("empty hold reason")
	}
	holds, err := s.OpenHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(holds) != 0 {
		t.Fatalf("rejected writes must not leave orphan hold events: %+v", holds)
	}
	c, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if c.LastReadingMiles != nil || c.HoldReason != nil {
		t.Fatalf("rejected writes changed the car: %+v", *c)
	}
}

func TestInsertOilChangeIsAtomic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1"}); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := s.InsertOilChange(ctx, model.OilChange{EFleetsID: "CAR1", Miles: 50000, Date: day, Source: "oil-done"}); err != nil {
		t.Fatal(err)
	}
	// Make the cars denormalisation fail for one specific write; the history
	// row inserted in the same call must roll back with it.
	if _, err := s.rawExec(ctx, `CREATE TRIGGER fail_oil BEFORE UPDATE ON cars FOR EACH ROW
		WHEN NEW.last_oil_miles = 60000 BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	err = s.InsertOilChange(ctx, model.OilChange{EFleetsID: "CAR1", Miles: 60000, Date: day.AddDate(0, 1, 0), Source: "oil-done"})
	if err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("expected injected failure, got %v", err)
	}
	var n int
	if err := s.rawQueryRow(ctx, `SELECT COUNT(*) FROM oil_changes WHERE efleets_id=?`, "CAR1").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("failed oil change left %d history rows, want 1", n)
	}
	c, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if c.LastOilMiles == nil || *c.LastOilMiles != 50000 {
		t.Fatalf("last_oil_miles %+v", c.LastOilMiles)
	}
	ok, err := s.HasOilChange(ctx, "CAR1", 60000, day.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("rolled-back oil change must not be visible to HasOilChange")
	}
}
