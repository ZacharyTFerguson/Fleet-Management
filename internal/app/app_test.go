package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"oilchange/internal/config"
	"oilchange/internal/enterprise"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
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

func TestSyncOneStepFetchesFromTrustedEnterpriseAnchor(t *testing.T) {
	p := filepath.Join(t.TempDir(), "onestep.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "CAR1", Nickname: "VA1"}); err != nil {
		t.Fatal(err)
	}
	odo := 100000
	trustedAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertFill(ctx, model.Fill{
		EFleetsID:                    "CAR1",
		ProviderCompanyVehicleNumber: "VA1",
		Odometer:                     &odo,
		ProviderTransactionTime:      trustedAt,
		Source:                       model.SourceFuelDetails,
	}); err != nil {
		t.Fatal(err)
	}
	carID := "CAR1"
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID:          "FACT1",
		DeviceID:           "DEV1",
		LinkedCarEFleetsID: &carID,
	}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/api/public/route/drive-stop" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("factory_id"); got != "FACT1" {
			t.Errorf("factory_id %q", got)
		}
		if got := r.URL.Query().Get("from"); got != trustedAt.Format(time.RFC3339) {
			t.Errorf("from %q", got)
		}
		_, _ = w.Write([]byte(`{"miles":12.4,"odometer":999999}`))
	}))
	defer srv.Close()
	client := onestep.NewClient(srv.URL, "")
	client.HTTP = srv.Client()

	a := &App{Store: st}
	if err := a.SyncOneStep(ctx, "", client); err != nil {
		t.Fatal(err)
	}
	miles, err := st.ListMilesSince(ctx, []string{"FACT1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(miles) != 1 || miles[0].Miles != 12.4 || !miles[0].Since.Equal(trustedAt) {
		t.Fatalf("stored drive-stop miles: %+v", miles)
	}
	code, err := a.Compute(ctx, false)
	if err != nil || code != model.ExitOK {
		t.Fatalf("compute code=%d err=%v", code, err)
	}
	car, err := st.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastReadingMiles == nil || *car.LastReadingMiles != 100012 {
		t.Fatalf("last reading must use Enterprise odo plus miles-since: %+v", car.LastReadingMiles)
	}
}


func TestSyncOneStepReturnsDriveStopFailures(t *testing.T) {
	p := filepath.Join(t.TempDir(), "onestep-error.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "CAR1", Nickname: "VA1"}); err != nil {
		t.Fatal(err)
	}
	odo := 100000
	if err := st.UpsertFill(ctx, model.Fill{
		EFleetsID:                    "CAR1",
		ProviderCompanyVehicleNumber: "VA1",
		Odometer:                     &odo,
		ProviderTransactionTime:      time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		Source:                       model.SourceFuelDetails,
	}); err != nil {
		t.Fatal(err)
	}
	carID := "CAR1"
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID:          "FACT1",
		DeviceID:           "DEV1",
		LinkedCarEFleetsID: &carID,
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	client := onestep.NewClient(srv.URL, "")
	client.HTTP = srv.Client()
	a := &App{Store: st}
	err = a.SyncOneStep(ctx, "", client)
	if err == nil || !strings.Contains(err.Error(), "factory_id FACT1") {
		t.Fatalf("expected surfaced OneStep failure, got %v", err)
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
		t.Fatalf("old demo fleet got %d want 2", len(demoCars))
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
	if err := a.SyncEnterprise(ctx, livePath, "","",""); err != nil {
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

func TestAdapterSelectionOrder(t *testing.T) {
	a := &App{Cfg: config.Config{
		EFleetsCDP:     "http://127.0.0.1:9222",
		EFleetsUser:    "u",
		EFleetsPass:    "p",
		EFleetsCust:    "1",
		EFleetsDetails: "https://example.invalid/DETAILS.csv",
	}}
	ad, err := a.adapter("veh.csv", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ad.(enterprise.FileAdapter); !ok {
		t.Fatalf("files first, got %T", ad)
	}
	ad, err = a.adapter("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ad.(*enterprise.ChromeSessionAdapter); !ok {
		t.Fatalf("CDP over password, got %T", ad)
	}
	a.Cfg.EFleetsCDP = ""
	ad, err = a.adapter("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ad.(*enterprise.HTTPAdapter); !ok {
		t.Fatalf("password HTTP last, got %T", ad)
	}
}

func TestHasFileUsesCapturedURLWithoutFetch(t *testing.T) {
	a := &App{}
	chrome := &enterprise.ChromeSessionAdapter{DetailsURL: "https://example.invalid/DETAILS.csv"}
	if !a.hasFile(chrome, enterprise.ReportFuelDetails) {
		t.Fatal("DETAILS URL must count")
	}
	if a.hasFile(chrome, enterprise.ReportShopRO) {
		t.Fatal("missing MAINT URL must not count")
	}
	if a.hasFile(enterprise.FileAdapter{Fuel: "details.csv"}, enterprise.ReportFuelDetails) {
		t.Fatal("file adapter is driven by CLI paths")
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

func TestSyncDevicesLinksAPIByFactoryIDNotDisplayName(t *testing.T) {
	p := filepath.Join(t.TempDir(), "link.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "OTHER", Nickname: "WrongCar"}); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(t.TempDir(), "map.csv")
	if err := os.WriteFile(mapPath, []byte("factory_id,device_id,efleets_id,display_name\nFACT1,OLDDEV,27TESTA,ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device"):
			// display_name matches OTHER's nickname — must not join on that.
			_, _ = w.Write([]byte(`[{"factory_id":"FACT1","device_id":"DEVLIVE","display_name":"WrongCar","odometer":999}]`))
		case strings.Contains(r.URL.Path, "drive-stop"):
			_, _ = w.Write([]byte(`{"miles":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := onestep.NewClient(srv.URL, "tok")
	client.HTTP = srv.Client()
	a := &App{Cfg: config.Config{OneStepToken: "tok"}, Store: st}
	n, err := a.SyncDevices(ctx, mapPath, client)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("devices upserted %d", n)
	}
	devs, err := st.ListDevicesForCar(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].FactoryID != "FACT1" || devs[0].DeviceID != "DEVLIVE" {
		t.Fatalf("want FACT1 linked to 27TESTA from API inventory, got %+v", devs)
	}
	wrong, err := st.ListDevicesForCar(ctx, "OTHER")
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 0 {
		t.Fatalf("must not join display_name WrongCar: %+v", wrong)
	}
}
