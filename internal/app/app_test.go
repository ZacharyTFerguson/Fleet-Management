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
