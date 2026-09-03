package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
	"oilchange/internal/store"
)

// assertComputeInvariant is the post-Compute contract: every car is either on
// HOLD (hold_reason set, exactly one open hold event with that reason) or Live
// (full last_reading triple, positive miles, no open events). Both-NULL is
// forbidden after a Compute that returned no error. Returns (live, held).
func assertComputeInvariant(t *testing.T, st *store.Store) (int, int) {
	t.Helper()
	ctx := context.Background()
	cars, err := st.ListCars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	holds, err := st.OpenHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	openBy := map[string][]model.HoldEvent{}
	for _, h := range holds {
		openBy[h.EFleetsID] = append(openBy[h.EFleetsID], h)
	}
	live, held := 0, 0
	for _, c := range cars {
		ev := openBy[c.EFleetsID]
		delete(openBy, c.EFleetsID)
		switch {
		case c.HoldReason != nil:
			held++
			if len(ev) != 1 {
				t.Errorf("%s hold_reason=%s has %d open events, want exactly 1: %+v", c.EFleetsID, *c.HoldReason, len(ev), ev)
				continue
			}
			if ev[0].Reason != *c.HoldReason {
				t.Errorf("%s hold_reason=%s but open event says %s", c.EFleetsID, *c.HoldReason, ev[0].Reason)
			}
			if c.LastReadingMiles != nil && *c.LastReadingMiles <= 0 {
				t.Errorf("%s stale reading is 0, not NULL", c.EFleetsID)
			}
		case c.LastReadingMiles == nil && c.LastReadingAt == nil && c.LastReadingSource == nil:
			t.Errorf("%s has neither hold_reason nor a Last Reading after Compute", c.EFleetsID)
		default:
			live++
			if c.LastReadingMiles == nil || c.LastReadingAt == nil || c.LastReadingSource == nil {
				t.Errorf("%s partial last_reading triple: %+v", c.EFleetsID, c)
				continue
			}
			if *c.LastReadingMiles <= 0 {
				t.Errorf("%s last_reading_miles %d", c.EFleetsID, *c.LastReadingMiles)
			}
			if *c.LastReadingSource != model.SourceFuelDetails && *c.LastReadingSource != model.SourceShopRO {
				t.Errorf("%s last_reading_source %q", c.EFleetsID, *c.LastReadingSource)
			}
			if len(ev) != 0 {
				t.Errorf("%s is Live but has open hold events: %+v", c.EFleetsID, ev)
			}
		}
	}
	for id, ev := range openBy {
		t.Errorf("open hold events for %s, which is not a car: %+v", id, ev)
	}
	return live, held
}

// rawSQLite opens a second connection to the store file so tests can count
// rows or inject failures without widening the Store API.
func rawSQLite(t *testing.T, p string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+p+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// driveStopServer answers every drive-stop request with a fixed trip sum.
func driveStopServer(t *testing.T, miles float64) *onestep.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "drive-stop") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"miles":%g}`, miles)
	}))
	t.Cleanup(srv.Close)
	c := onestep.NewClient(srv.URL, "")
	c.HTTP = srv.Client()
	return c
}

func TestComputeLiveFleetInvariantAndNoHoldStacking(t *testing.T) {
	p := filepath.Join(t.TempDir(), "live-compute.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Cfg: config.Config{SQLitePath: p}, Store: st}
	ctx := context.Background()
	if err := a.SyncEnterprise(ctx, testdata("enterprise", "fleetsummary_live.csv"), "", "", ""); err != nil {
		t.Fatal(err)
	}
	cars, err := st.ListCars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cars) != 205 {
		t.Fatalf("live roster %d", len(cars))
	}
	// One trusted fill for most of the roster; every 50th car has none so the
	// fleet splits into Live and HOLD.
	fillAt := time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)
	for i, c := range cars {
		if i%50 == 0 {
			continue
		}
		odo := 100000 + 100*i
		if err := st.UpsertFill(ctx, model.Fill{
			EFleetsID:                    c.EFleetsID,
			ProviderCompanyVehicleNumber: c.Nickname,
			Odometer:                     &odo,
			ProviderTransactionTime:      fillAt,
			Source:                       model.SourceFuelDetails,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// One live box per car so the fleet splits into Live and HOLD instead of
	// every car landing on NO_DEVICE.
	var sb strings.Builder
	sb.WriteString("factory_id,device_id,efleets_id,display_name\n")
	for _, c := range cars {
		fmt.Fprintf(&sb, "F-%s,D-%s,%s,box\n", c.EFleetsID, c.EFleetsID, c.EFleetsID)
	}
	mapPath := filepath.Join(t.TempDir(), "map.csv")
	if err := os.WriteFile(mapPath, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncOneStep(ctx, mapPath, driveStopServer(t, 7)); err != nil {
		t.Fatal(err)
	}

	code, err := a.Compute(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if code != model.ExitOK && code != model.ExitHolds {
		t.Fatalf("exit %d", code)
	}
	live, held := assertComputeInvariant(t, st)
	if live == 0 {
		t.Fatal("live fleet with fills, boxes and miles-since produced no Last Reading")
	}
	if held == 0 {
		t.Fatal("cars without fills must be on HOLD, not silently NULL")
	}
	if code != model.ExitHolds {
		t.Fatalf("held=%d but exit %d", held, code)
	}
	t.Logf("live fleet: %d Live, %d HOLD", live, held)

	db := rawSQLite(t, p)
	eventsAfterFirst := countRows(t, db, "hold_events")
	before, _ := st.ListCars(ctx)

	// Repeated ticks: same decisions, no new hold rows, readings unchanged.
	for i := 0; i < 3; i++ {
		if _, err := a.Compute(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	live2, held2 := assertComputeInvariant(t, st)
	if live2 != live || held2 != held {
		t.Fatalf("recompute changed the split: %d/%d -> %d/%d", live, held, live2, held2)
	}
	if got := countRows(t, db, "hold_events"); got != eventsAfterFirst {
		t.Fatalf("hold_events grew from %d to %d across identical computes", eventsAfterFirst, got)
	}
	after, _ := st.ListCars(ctx)
	for i := range before {
		b, c := before[i], after[i]
		if (b.LastReadingMiles == nil) != (c.LastReadingMiles == nil) ||
			(b.LastReadingMiles != nil && *b.LastReadingMiles != *c.LastReadingMiles) ||
			(b.HoldReason == nil) != (c.HoldReason == nil) ||
			(b.HoldReason != nil && *b.HoldReason != *c.HoldReason) {
			t.Fatalf("%s changed on identical recompute: %+v -> %+v", b.EFleetsID, b, c)
		}
	}
}

// TestComputePartialFailureLeavesNoHybrid injects a failure on one car's
// Last Reading write. The other cars must still be decided, the failing car
// must be left exactly as it was (never a half-written triple or an orphan
// hold event), and the error must name the car.
func TestComputePartialFailureLeavesNoHybrid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "partial.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Store: st}
	ctx := context.Background()
	at := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"CARA", "CARB", "CARC"} {
		if err := st.UpsertCar(ctx, model.Car{EFleetsID: id, Nickname: id}); err != nil {
			t.Fatal(err)
		}
		odo := 100000 + 1000*i
		if err := st.UpsertFill(ctx, model.Fill{EFleetsID: id, ProviderCompanyVehicleNumber: id, Odometer: &odo, ProviderTransactionTime: at, Source: model.SourceFuelDetails}); err != nil {
			t.Fatal(err)
		}
		link := id
		if err := st.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F-" + id, DeviceID: "D-" + id, LinkedCarEFleetsID: &link}); err != nil {
			t.Fatal(err)
		}
		if err := st.SaveMilesSince(ctx, model.DriveStopMiles{FactoryID: "F-" + id, Since: at, Miles: 12}); err != nil {
			t.Fatal(err)
		}
	}

	db := rawSQLite(t, p)
	if _, err := db.Exec(`CREATE TRIGGER fail_carb BEFORE UPDATE ON cars FOR EACH ROW
		WHEN NEW.efleets_id='CARB' AND (NEW.last_reading_miles IS NOT NULL OR NEW.hold_reason IS NOT NULL)
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}

	code, err := a.Compute(ctx, false)
	if err == nil || !strings.Contains(err.Error(), "CARB") || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("want error naming CARB, got code=%d err=%v", code, err)
	}
	if code != model.ExitError {
		t.Fatalf("exit %d want %d", code, model.ExitError)
	}
	for _, id := range []string{"CARA", "CARC"} {
		c, err := st.CarByEFleets(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if c.LastReadingMiles == nil || c.LastReadingAt == nil || c.LastReadingSource == nil || c.HoldReason != nil {
			t.Fatalf("%s should have been decided despite CARB failing: %+v", id, c)
		}
	}
	b, err := st.CarByEFleets(ctx, "CARB")
	if err != nil {
		t.Fatal(err)
	}
	if b.LastReadingMiles != nil || b.LastReadingAt != nil || b.LastReadingSource != nil || b.HoldReason != nil {
		t.Fatalf("failed car must be left untouched, got %+v", b)
	}
	holds, err := st.OpenHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(holds) != 0 {
		t.Fatalf("no car is on HOLD, yet open events exist: %+v", holds)
	}
	if n := countRows(t, db, "hold_events"); n != 0 {
		t.Fatalf("rolled-back write left %d hold_events rows", n)
	}

	// Once the fault is gone the next Compute completes the fleet.
	if _, err := db.Exec(`DROP TRIGGER fail_carb`); err != nil {
		t.Fatal(err)
	}
	code, err = a.Compute(ctx, false)
	if err != nil || code != model.ExitOK {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if live, held := assertComputeInvariant(t, st); live != 3 || held != 0 {
		t.Fatalf("live=%d held=%d", live, held)
	}
}

// TestComputeStopsOnCancelledContext: cancellation is not a per-car error to
// skip past; the loop stops and reports ctx.Err().
func TestComputeStopsOnCancelledContext(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cancel.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Store: st}
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "CARA"}); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	code, err := a.Compute(cctx, false)
	if code != model.ExitError || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	c, err := st.CarByEFleets(ctx, "CARA")
	if err != nil {
		t.Fatal(err)
	}
	if c.HoldReason != nil || c.LastReadingMiles != nil {
		t.Fatalf("cancelled compute must not decide cars: %+v", c)
	}
}
