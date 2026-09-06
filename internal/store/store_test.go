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

func TestInsertOilChangeAdvancesLastOilNotLastReading(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	readAt := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	if err := s.WriteLastReading(ctx, "27TESTA", 180312, readAt, model.SourceFuelDetails); err != nil {
		t.Fatal(err)
	}
	newer := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := s.InsertOilChange(ctx, model.OilChange{EFleetsID: "27TESTA", Miles: 179598, Date: newer, Location: "Valvoline", Source: "shop_ro"}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertOilChange(ctx, model.OilChange{EFleetsID: "27TESTA", Miles: 100000, Date: older, Location: "Old Shop", Source: "shop_ro"}); err != nil {
		t.Fatal(err)
	}
	c, err := s.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if c.LastOilMiles == nil || *c.LastOilMiles != 179598 {
		t.Fatalf("last oil must stay the later RO, got %+v", c.LastOilMiles)
	}
	if c.LastOilDate == nil || !c.LastOilDate.Equal(newer) {
		t.Fatalf("last oil date %+v", c.LastOilDate)
	}
	if c.LastReadingMiles == nil || *c.LastReadingMiles != 180312 {
		t.Fatalf("InsertOilChange must not touch Last Reading, got %+v", c.LastReadingMiles)
	}
}

func TestUpsertCarReconcilesOilChangeImportedBeforeRoster(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil-before-roster.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	older := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	for _, change := range []model.OilChange{
		{EFleetsID: "27TESTA", Miles: 100000, Date: older, Location: "Old Shop", Source: "shop_ro"},
		{EFleetsID: "27TESTA", Miles: 179598, Date: newer, Location: "Valvoline", Source: "shop_ro"},
	} {
		if err := s.InsertOilChange(ctx, change); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	car, err := s.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastOilMiles == nil || *car.LastOilMiles != 179598 {
		t.Fatalf("last oil must reconcile from the latest stored maintenance row, got %+v", car.LastOilMiles)
	}
	if car.LastOilDate == nil || !car.LastOilDate.Equal(newer) {
		t.Fatalf("last oil date %+v", car.LastOilDate)
	}
	if car.LastReadingMiles != nil {
		t.Fatalf("roster reconciliation must not invent Last Reading, got %+v", car.LastReadingMiles)
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

// TestListOrderDeterministicOnSameSecond locks the storage sort contract:
// chronological with explicit tie-breakers, so two reads of the same data can
// never return same-second transactions in different orders (History tx keys,
// GPS matching, and the fill picker all replay these lists).
func TestListOrderDeterministicOnSameSecond(t *testing.T) {
	p := filepath.Join(t.TempDir(), "order.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 9, 3, 16, 0, 0, 0, time.UTC)
	hi, lo := 90500, 90400

	// Insert deliberately out of order: no-odometer punch first, then high, then low.
	for _, f := range []model.Fill{
		{EFleetsID: "27SEPB", ProviderTransactionTime: at, MerchantName: "EVGO"},
		{EFleetsID: "27SEPB", ProviderTransactionTime: at, Odometer: &hi, MerchantName: "MARATHON"},
		{EFleetsID: "27SEPB", ProviderTransactionTime: at, Odometer: &lo, MerchantName: "SUNOCO"},
		{EFleetsID: "27SEPB", ProviderTransactionTime: at.Add(-time.Hour), Odometer: &lo, MerchantName: "OLDEST"},
	} {
		if err := s.UpsertFill(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	fills, err := s.ListFills(ctx, "27SEPB")
	if err != nil {
		t.Fatal(err)
	}
	var merchants []string
	for _, f := range fills {
		merchants = append(merchants, f.MerchantName)
	}
	want := []string{"OLDEST", "EVGO", "SUNOCO", "MARATHON"}
	if len(merchants) != len(want) {
		t.Fatalf("fills %v", merchants)
	}
	for i := range want {
		if merchants[i] != want[i] {
			t.Fatalf("fills order %v want %v (time asc, then odometer, then rowid)", merchants, want)
		}
	}

	for _, tx := range []model.CardTx{
		{CardID: "x22020", At: at, RecordedEFleetsID: "27SEPB", Odometer: &hi, StationName: "MARATHON"},
		{CardID: "x22020", At: at, RecordedEFleetsID: "27SEPB", StationName: "EVGO"},
		{CardID: "x11010", At: at, RecordedEFleetsID: "27SEPA", Odometer: &lo, StationName: "SHELL"},
	} {
		if err := s.UpsertCardTx(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	txs, err := s.ListCardTxs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var stations []string
	for _, tx := range txs {
		stations = append(stations, tx.StationName)
	}
	wantTx := []string{"MARATHON", "SHELL", "EVGO"}
	if len(stations) != len(wantTx) {
		t.Fatalf("txs %v", stations)
	}
	for i := range wantTx {
		if stations[i] != wantTx[i] {
			t.Fatalf("tx order %v want %v (time desc, then higher odometer first with missing last, then card)", stations, wantTx)
		}
	}
}

// TestFillsNullOdoSchemaGuard locks the two layers that keep NULL-odometer
// punches (EV charging) from duplicating: the partial unique index added in
// migration 010, and the migration's cleanup of duplicates an older binary
// may already have written.
func TestFillsNullOdoSchemaGuard(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nullodo.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC).Format(time.RFC3339)

	if _, err := s.exec(ctx, `INSERT INTO fills (efleets_id, provider_transaction_time, odometer) VALUES (?,?,NULL)`, "27SEPB", at); err != nil {
		t.Fatal(err)
	}
	// A raw duplicate insert (bypassing the UpsertFill guard) must hit the index.
	if _, err := s.exec(ctx, `INSERT INTO fills (efleets_id, provider_transaction_time, odometer) VALUES (?,?,NULL)`, "27SEPB", at); err == nil {
		t.Fatal("partial unique index must reject a second NULL-odometer punch in the same second")
	}

	// Upgrade path: a database written by an older binary can already hold
	// duplicates. Recreate that state and re-run migrations: the duplicates
	// collapse to one row and the index comes back.
	if _, err := s.db.Exec(`DROP INDEX fills_one_null_odo_per_second`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(ctx, `INSERT INTO fills (efleets_id, provider_transaction_time, odometer) VALUES (?,?,NULL)`, "27SEPB", at); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(s.db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM fills WHERE efleets_id=? AND odometer IS NULL`, "27SEPB").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("migration must collapse legacy NULL-odometer duplicates, got %d rows", n)
	}
	if _, err := s.exec(ctx, `INSERT INTO fills (efleets_id, provider_transaction_time, odometer) VALUES (?,?,NULL)`, "27SEPB", at); err == nil {
		t.Fatal("index must be recreated after re-migration")
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

func TestListCardTxsNewestFirst(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cards-order.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	older := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	same := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	for _, tx := range []model.CardTx{
		{CardID: "C-B", At: same, RecordedEFleetsID: "27VA15"},
		{CardID: "C-A", At: same, RecordedEFleetsID: "27VA15"},
		{CardID: "C-NEW", At: newer, RecordedEFleetsID: "27VA15"},
		{CardID: "C-OLD", At: older, RecordedEFleetsID: "27VA15"},
	} {
		if err := s.UpsertCardTx(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListCardTxs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].CardID != "C-A" || got[1].CardID != "C-B" || got[2].CardID != "C-NEW" || got[3].CardID != "C-OLD" {
		t.Fatalf("newest-first stable order %+v", got)
	}
}
