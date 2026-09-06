package app

import (
	"context"
	"encoding/csv"
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

// TestNormalOilUpdateOpsContract is the executable stand-in for a normal
// Oil Change Updater + OneStep Liaison run:
// Maintenance + DETAILS -> Enterprise anchor + measured drive-stop -> report.
// Run it with: go test ./internal/app -run TestNormalOilUpdateOpsContract -v
func TestNormalOilUpdateOpsContract(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "normal-oil-update.sqlite")
	st, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Cfg: config.Config{SQLitePath: dbPath}, Store: st}

	// Real drops can arrive Maintenance-first. PR #37 requires the later roster
	// import to reconcile the already-stored oil change into cars.last_oil_*.
	if err := a.SyncEnterprise(ctx, "", "", testdata("enterprise", "maintenance.csv"), ""); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncEnterprise(ctx,
		testdata("enterprise", "fleetsummary.csv"),
		testdata("enterprise", "details.csv"),
		"",
		"",
	); err != nil {
		t.Fatal(err)
	}

	car, err := st.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastOilMiles == nil || *car.LastOilMiles != 100500 {
		t.Fatalf("last oil must come from the oil-change RO: %+v", car.LastOilMiles)
	}
	if car.LastReadingMiles != nil {
		t.Fatalf("Enterprise ingest must not invent Last Reading: %+v", car.LastReadingMiles)
	}

	var driveStopCalls int
	oneStep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		driveStopCalls++
		q := r.URL.Query()
		if got := q.Get("device_id"); got != "DEV1" {
			t.Errorf("drive-stop device_id=%q want DEV1", got)
		}
		if got := q.Get("dt_tracker_from"); got != "2026-06-11T04:00:00Z" {
			t.Errorf("drive-stop from=%q; Last Reading anchor must be latest good maintenance", got)
		}
		if q.Get("dt_tracker_to") == "" {
			t.Error("drive-stop must include an explicit to")
		}
		if q.Get("factory_id") != "" {
			t.Error("drive-stop request must use device_id, not factory_id")
		}
		_, _ = w.Write([]byte(`{"miles":12.4,"odometer":999999}`))
	}))
	defer oneStep.Close()
	client := newTestOneStepClient(oneStep)

	if err := a.SyncOneStep(ctx, testdata("onestep", "map.csv"), client); err != nil {
		t.Fatal(err)
	}
	if driveStopCalls != 1 {
		t.Fatalf("drive-stop calls=%d want 1; dead/personnel boxes must not be queried", driveStopCalls)
	}
	if code, err := a.Compute(ctx, false); err != nil || code != model.ExitHolds {
		// TESTB has no device; that fleet HOLD must not prevent TESTA's valid write.
		t.Fatalf("compute code=%d err=%v", code, err)
	}

	car, err = st.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastReadingMiles == nil || *car.LastReadingMiles != 100612 {
		t.Fatalf("Last Reading must be maint odo 100600 + measured 12.4, never OneStep odo: %+v", car.LastReadingMiles)
	}
	if car.LastReadingSource == nil || *car.LastReadingSource != model.SourceShopRO {
		t.Fatalf("Last Reading source=%v want %s", car.LastReadingSource, model.SourceShopRO)
	}

	// Older maintenance can be replayed but last oil only moves forward.
	if err := a.OilDone(ctx, "27TESTA", 90000, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "old shop"); err != nil {
		t.Fatal(err)
	}
	car, _ = st.CarByEFleets(ctx, "27TESTA")
	if car.LastOilMiles == nil || *car.LastOilMiles != 100500 {
		t.Fatalf("older oil row moved last oil backward: %+v", car.LastOilMiles)
	}

	reportPath := filepath.Join(t.TempDir(), "automations-copy.csv")
	if err := a.Report(ctx, 5000, 0, reportPath); err != nil {
		t.Fatal(err)
	}
	rows := readReportRows(t, reportPath)
	row := rows["27TESTA"]
	if row["last_oil_miles"] != "100500" || row["last_reading_miles"] != "100612" {
		t.Fatalf("sheet-facing values: %+v", row)
	}
	for header := range row {
		h := strings.ToLower(header)
		if strings.Contains(h, "remaining") || strings.Contains(h, "due") {
			t.Fatalf("sheet-facing report must not write remaining/due column %q", header)
		}
	}

	// A newer trusted Enterprise anchor without its exact drive-stop measurement
	// must HOLD and retain (but no longer present as current) the prior reading.
	newAnchor := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	if err := st.UpsertShopRO(ctx, model.ShopRO{
		ROID: "RO-NEXT", EFleetsID: "27TESTA", Odometer: 100620, At: newAnchor, LocationName: "Shop",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Compute(ctx, false); err != nil {
		t.Fatal(err)
	}
	car, _ = st.CarByEFleets(ctx, "27TESTA")
	if car.HoldReason == nil || *car.HoldReason != model.HoldNoDriveStop {
		t.Fatalf("missing measured miles must HOLD NO_DRIVESTOP: %+v", car.HoldReason)
	}
	if car.LastReadingMiles == nil || *car.LastReadingMiles != 100612 {
		t.Fatalf("HOLD must retain prior audit value, not invent miles: %+v", car.LastReadingMiles)
	}
}

func newTestOneStepClient(server *httptest.Server) *onestep.Client {
	client := onestep.NewClient(server.URL, "")
	client.HTTP = server.Client()
	return client
}

func readReportRows(t *testing.T, path string) map[string]map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	all, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Fatalf("report rows=%d", len(all))
	}
	out := map[string]map[string]string{}
	for _, values := range all[1:] {
		row := map[string]string{}
		for i, header := range all[0] {
			if i < len(values) {
				row[header] = values[i]
			}
		}
		out[row["efleets_id"]] = row
	}
	return out
}
