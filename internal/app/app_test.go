package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"oilchange/internal/config"
	"oilchange/internal/enterprise"
	"oilchange/internal/model"
	"oilchange/internal/store"
)

func testdata(elem ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", "..", "testdata"}, elem...)...)
}

func TestSyncAndComputeFileDrop(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Cfg: config.Config{SQLitePath: p}, Store: st}
	ctx := context.Background()
	err = a.SyncEnterprise(ctx,
		testdata("enterprise", "fleetsummary.csv"),
		testdata("enterprise", "details.csv"),
		testdata("enterprise", "maintenance.csv"),
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	car, err := st.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastOilMiles == nil || *car.LastOilMiles != 100500 {
		t.Fatalf("last oil from shop RO, got %+v", car.LastOilMiles)
	}
	if err := a.SyncOneStep(ctx, testdata("onestep", "map.csv"), nil); err != nil {
		t.Fatal(err)
	}
	devs, _ := st.ListDevicesForCar(ctx, "27TESTA")
	if len(devs) == 0 {
		t.Fatal("map should link FACT1")
	}
	fills, _ := st.ListFills(ctx, "27TESTA")
	if len(fills) == 0 {
		t.Fatal("fills")
	}
	trusted := fills[len(fills)-1].ProviderTransactionTime
	if err := st.SaveMilesSince(ctx, model.DriveStopMiles{FactoryID: "FACT1", Since: trusted, Miles: 10}); err != nil {
		t.Fatal(err)
	}
	code, err := a.Compute(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	car, _ = st.CarByEFleets(ctx, "27TESTA")
	if car.HoldReason != nil {
		t.Logf("hold %s (exit %d)", *car.HoldReason, code)
	}
	if car.LastReadingMiles == nil && car.HoldReason == nil {
		t.Fatal("expected reading or hold")
	}
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	if err := a.Report(ctx, 5000, 0, ""); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	os.Stdout = old
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	if strings.Contains(out, "Change oil at 0") || strings.Contains(out, "Mileage due at") {
		t.Fatal(out)
	}
}

func TestLiveFleetHasMoreVehiclesThanTwoCarDemo(t *testing.T) {
	demo, err := os.Open(testdata("enterprise", "fleetsummary.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer demo.Close()
	demoCars, err := enterprise.ParseVehicles(demo)
	if err != nil {
		t.Fatal(err)
	}
	if len(demoCars) != 2 {
		t.Fatalf("old demo fleet: got %d want 2", len(demoCars))
	}

	livePath := testdata("enterprise", "fleetsummary_live.csv")
	live, err := os.Open(livePath)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	liveCars, err := enterprise.ParseVehicles(live)
	if err != nil {
		t.Fatal(err)
	}
	if len(liveCars) <= len(demoCars) {
		t.Fatalf("live fleet %d is not larger than the 2-car demo", len(liveCars))
	}
	if len(liveCars) < 100 {
		t.Fatalf("live fleet %d; want at least 100 imported cars", len(liveCars))
	}
	if len(liveCars) != 205 {
		t.Fatalf("stable live roster: got %d want 205", len(liveCars))
	}

	p := filepath.Join(t.TempDir(), "live.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Cfg: config.Config{SQLitePath: p}, Store: st}
	ctx := context.Background()
	if err := a.SyncEnterprise(ctx, livePath, "", "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListCars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 205 {
		t.Fatalf("store after live sync: got %d want 205", len(got))
	}
}

func TestOilDoneDoesNotChangeLastReading(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Store: st}
	ctx := context.Background()
	_ = st.UpsertCar(ctx, model.Car{EFleetsID: "X"})
	_ = st.WriteLastReading(ctx, "X", 111, time.Now().UTC(), model.SourceFuelDetails)
	if err := a.OilDone(ctx, "X", 50, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "shop"); err != nil {
		t.Fatal(err)
	}
	c, _ := st.CarByEFleets(ctx, "X")
	if c.LastReadingMiles == nil || *c.LastReadingMiles != 111 {
		t.Fatal("oil-done must not change last reading")
	}
	if c.LastOilMiles == nil || *c.LastOilMiles != 50 {
		t.Fatal("last oil")
	}
}
