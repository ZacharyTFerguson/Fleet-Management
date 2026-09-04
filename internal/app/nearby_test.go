package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/config"
	"oilchange/internal/enterprise"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
	"oilchange/internal/store"
)

const nearbyPumpLat, nearbyPumpLng = 40.61716, -79.722

func nearbySheetzTx(card string, fill time.Time) model.CardTx {
	return model.CardTx{
		CardID:            card,
		At:                fill.UTC(),
		StationName:       "SHEETZ",
		StationAddress:    "203 CRAIGDELL RD, LOWER BURRELL, PA",
		RecordedEFleetsID: "TRACKER",
	}
}

// seedGeocodedSheetz adds one exclusive GPS-first sit so SHEETZ is mapped.
// Nearby leftover fills must not use this box at fill time.
func seedGeocodedSheetz(t *testing.T, ctx context.Context, st *store.Store, visits *[]model.StopVisit) {
	t.Helper()
	ny := enterprise.NY()
	geoCar := "27VA15"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: geoCar, Nickname: "VA15"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-GEO", DeviceID: "DEV-GEO", LinkedCarEFleetsID: &geoCar, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	fill := time.Date(2026, 5, 1, 14, 0, 0, 0, ny)
	if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-GEO", fill)); err != nil {
		t.Fatal(err)
	}
	*visits = append(*visits, model.StopVisit{
		FactoryID: "FACT-GEO", DeviceID: "DEV-GEO", EFleetsID: geoCar,
		HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng,
		From: fill.Add(-5 * time.Minute), To: fill.Add(10 * time.Minute),
	})
}

func coverFactory(visits *[]model.StopVisit, factoryID, deviceID string) {
	ny := enterprise.NY()
	*visits = append(*visits, model.StopVisit{
		FactoryID: factoryID, DeviceID: deviceID,
		From: time.Date(2026, 5, 1, 0, 0, 0, 0, ny),
		To:   time.Date(2026, 7, 1, 0, 0, 0, 0, ny),
	})
}

func TestCardsNearbyPersistsCertainLinkedEraNotUnpaired(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cache := filepath.Join(t.TempDir(), "gps-stops.json")
	a := &App{Cfg: config.Config{SQLitePath: p}, Store: st, GPSStopsPath: cache}
	ctx := context.Background()
	ny := enterprise.NY()
	car := "26LSZW"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car, Nickname: "Bing-2"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-CAR", DeviceID: "DEV-CAR", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	unpaired := "FACT-LOOSE"
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: unpaired, DeviceID: "DEV-LOOSE", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	var visits []model.StopVisit
	seedGeocodedSheetz(t, ctx, st, &visits)
	for i := 0; i < 3; i++ {
		fill := time.Date(2026, 6, 10+i*3, 14, 20, 0, 0, ny)
		if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-HOME", fill)); err != nil {
			t.Fatal(err)
		}
		// No EFleetsID: GPS-first cannot name this leftover swipe; nearby still joins via device link.
		visits = append(visits, model.StopVisit{
			FactoryID: "FACT-CAR", DeviceID: "DEV-CAR",
			HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng,
			From: fill.Add(-5 * time.Minute), To: fill.Add(15 * time.Minute),
		})
	}
	coverFactory(&visits, unpaired, "DEV-LOOSE")
	coverFactory(&visits, "FACT-GEO", "DEV-GEO")
	coverFactory(&visits, "FACT-CAR", "DEV-CAR")
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	res, err := a.CardsNearby(ctx, CardsNearbyOpts{Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.CoverageComplete || res.Certain != 1 {
		t.Fatalf("certain %d complete=%v %+v", res.Certain, res.CoverageComplete, res)
	}
	eras, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(eras) != 1 || eras[0].EFleetsID != car || eras[0].CardID != "CARD-HOME" {
		t.Fatalf("eras %+v", eras)
	}
	if eras[0].HolderType != cards.HolderCar {
		t.Fatalf("holder %s", eras[0].HolderType)
	}
	got, err := st.CarByEFleets(ctx, car)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastReadingMiles != nil {
		t.Fatalf("Last Reading must stay empty: %+v", got.LastReadingMiles)
	}
}

func TestCardsNearbyPersonEraNotPersisted(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cache := filepath.Join(t.TempDir(), "gps-stops.json")
	a := &App{Store: st, GPSStopsPath: cache}
	ctx := context.Background()
	ny := enterprise.NY()
	car := "26LSZW"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-CAR", DeviceID: "DEV", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	var visits []model.StopVisit
	seedGeocodedSheetz(t, ctx, st, &visits)
	for i := 0; i < 3; i++ {
		fill := time.Date(2026, 6, 10+i*3, 14, 20, 0, 0, ny)
		if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-PERSON", fill)); err != nil {
			t.Fatal(err)
		}
		visits = append(visits, model.StopVisit{
			FactoryID: "FACT-CAR", DeviceID: "DEV",
			HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng,
			From: fill.Add(-5 * time.Minute), To: fill.Add(15 * time.Minute),
		})
	}
	coverFactory(&visits, "FACT-GEO", "DEV-GEO")
	coverFactory(&visits, "FACT-CAR", "DEV")
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceEras(ctx, []model.CardEra{{
		CardID: "CARD-PERSON", HolderType: cards.HolderPerson, HolderKey: "JANE DOE",
		From: time.Date(2026, 6, 1, 0, 0, 0, 0, ny), To: time.Date(2026, 7, 1, 0, 0, 0, 0, ny),
	}}); err != nil {
		t.Fatal(err)
	}
	res, err := a.CardsNearby(ctx, CardsNearbyOpts{Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Certain != 1 {
		t.Fatalf("PERSON card can still be watched as certain, but must not persist: %+v", res)
	}
	eras, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(eras) != 1 || eras[0].HolderType != cards.HolderPerson {
		t.Fatalf("PERSON era must remain: %+v", eras)
	}
}

func TestCardsNearbyDoesNotWriteLastReading(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "gps.json")}
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "26LSZW"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CardsNearby(ctx, CardsNearbyOpts{Persist: true}); err != nil {
		t.Fatal(err)
	}
	car, err := st.CarByEFleets(ctx, "26LSZW")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastReadingMiles != nil {
		t.Fatalf("Last Reading must stay empty: %+v", car.LastReadingMiles)
	}
}

func TestCardsNearbyIncompleteCoverageDoesNotPersist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cache := filepath.Join(t.TempDir(), "gps-stops.json")
	a := &App{Store: st, GPSStopsPath: cache}
	ctx := context.Background()
	ny := enterprise.NY()
	car := "26LSZW"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-CAR", DeviceID: "DEV-CAR", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-LOOSE", DeviceID: "DEV-LOOSE", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	var visits []model.StopVisit
	seedGeocodedSheetz(t, ctx, st, &visits)
	for i := 0; i < 3; i++ {
		fill := time.Date(2026, 6, 10+i*3, 14, 20, 0, 0, ny)
		if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-HOME", fill)); err != nil {
			t.Fatal(err)
		}
		visits = append(visits, model.StopVisit{
			FactoryID: "FACT-CAR", DeviceID: "DEV-CAR",
			HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng,
			From: fill.Add(-5 * time.Minute), To: fill.Add(15 * time.Minute),
		})
	}
	// FACT-GEO and FACT-LOOSE span the window; FACT-CAR only has short sits — not a fetch.
	coverFactory(&visits, "FACT-GEO", "DEV-GEO")
	coverFactory(&visits, "FACT-LOOSE", "DEV-LOOSE")
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	res, err := a.CardsNearby(ctx, CardsNearbyOpts{Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.CoverageComplete || res.Certain != 0 {
		t.Fatalf("incomplete must stay watch-only: %+v", res)
	}
	eras, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(eras) != 0 {
		t.Fatalf("persist must skip incomplete coverage: %+v", eras)
	}
}

func TestCardsNearbyLiveStopsPacesOneDevicePerRequest(t *testing.T) {
	prev := nearbyDriveStopMinInterval
	nearbyDriveStopMinInterval = 40 * time.Millisecond
	defer func() { nearbyDriveStopMinInterval = prev }()

	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var hits int32
	var lastDevice string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device"):
			_, _ = w.Write([]byte(`{"result_list":[
				{"factory_id":"FACT-A","device_id":"DEV-A","active":true},
				{"factory_id":"FACT-B","device_id":"DEV-B","active":true},
				{"factory_id":"FACT-C","device_id":"DEV-C","active":true}
			]}`))
		case strings.Contains(r.URL.Path, "/route/drive-stop"):
			atomic.AddInt32(&hits, 1)
			did := r.URL.Query().Get("device_id")
			if did == "" {
				t.Error("drive-stop missing device_id")
			}
			if strings.Contains(did, ",") {
				t.Errorf("must not invent multi-device_id batch param: %q", did)
			}
			if lastDevice != "" && did == lastDevice {
				// ok to retry same box; just ensure one id per call
			}
			lastDevice = did
			_, _ = w.Write([]byte(`{"drive_stop_list":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	cache := filepath.Join(t.TempDir(), "gps-stops.json")
	a := &App{Store: st, GPSStopsPath: cache, OneStep: c}
	ctx := context.Background()
	ny := enterprise.NY()
	for _, id := range []string{"FACT-A", "FACT-B", "FACT-C"} {
		if err := st.UpsertDevice(ctx, model.OneStepDevice{
			FactoryID: id, DeviceID: "DEV-" + id[len(id)-1:], Active: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	fill := time.Date(2026, 6, 16, 14, 0, 0, 0, ny)
	if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-HOME", fill)); err != nil {
		t.Fatal(err)
	}
	// Empty cache → all three boxes need a spanning fetch.
	start := time.Now()
	if _, err := a.CardsNearby(ctx, CardsNearbyOpts{LiveStops: true}); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	got := atomic.LoadInt32(&hits)
	if got < 3 {
		t.Fatalf("expected ≥3 single-device drive-stop GETs, got %d", got)
	}
	// 3 calls with 40ms min gap → at least two waits (~80ms).
	if elapsed < 70*time.Millisecond {
		t.Fatalf("live multi-box pull must pace (~1/s class); elapsed=%s hits=%d", elapsed, got)
	}
}

func TestCardsNearbyDoesNotHitOneStepWithoutLive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "unexpected", 500)
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "missing-cache.json"), OneStep: c}
	ctx := context.Background()
	car := "26LSZW"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-CAR", DeviceID: "DEV", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	ny := enterprise.NY()
	fill := time.Date(2026, 6, 16, 14, 0, 0, 0, ny)
	if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-HOME", fill)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CardsNearby(ctx, CardsNearbyOpts{LiveStops: false}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("cards nearby without --live must not call OneStep, hits=%d", hits)
	}
}

func TestCardsNearbyReportCapCountsFailedPosts(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			atomic.AddInt32(&posts, 1)
			body, _ := io.ReadAll(r.Body)
			_ = body
			http.Error(w, "nope", 500)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "gps.json"), OneStep: c}
	ctx := context.Background()
	ny := enterprise.NY()
	for i, addr := range []string{"1 MAIN ST, A", "2 MAIN ST, B", "3 MAIN ST, C", "4 MAIN ST, D", "5 MAIN ST, E"} {
		fill := time.Date(2026, 6, 10+i, 14, 0, 0, 0, ny)
		if err := st.UpsertCardTx(ctx, model.CardTx{
			CardID: "CARD-RPT", At: fill.UTC(), StationName: "SHELL", StationAddress: addr,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.CardsNearby(ctx, CardsNearbyOpts{LiveReport: true, ReportCap: 2, LiveStops: false}); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&posts); got != 2 {
		t.Fatalf("report-cap must count failed generate posts, got %d", got)
	}
}

func TestCardsNearbyTwoCertainDoesNotPersist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cache := filepath.Join(t.TempDir(), "gps-stops.json")
	a := &App{Store: st, GPSStopsPath: cache}
	ctx := context.Background()
	ny := enterprise.NY()
	carA, carB := "26LSZW", "27VA15"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: carA}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: carB}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-A", DeviceID: "DA", LinkedCarEFleetsID: &carA, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-B", DeviceID: "DB", LinkedCarEFleetsID: &carB, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	var visits []model.StopVisit
	seedGeocodedSheetz(t, ctx, st, &visits)
	for i := 0; i < 3; i++ {
		fillA := time.Date(2026, 6, 1+i*2, 10, 0, 0, 0, ny)
		fillB := time.Date(2026, 6, 2+i*2, 10, 0, 0, 0, ny)
		if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-TWO", fillA)); err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-TWO", fillB)); err != nil {
			t.Fatal(err)
		}
		visits = append(visits,
			model.StopVisit{FactoryID: "FACT-A", DeviceID: "DA", HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng, From: fillA.Add(-time.Minute), To: fillA.Add(time.Minute)},
			model.StopVisit{FactoryID: "FACT-B", DeviceID: "DB", HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng, From: fillB.Add(-time.Minute), To: fillB.Add(time.Minute)},
		)
	}
	coverFactory(&visits, "FACT-GEO", "DEV-GEO")
	coverFactory(&visits, "FACT-A", "DA")
	coverFactory(&visits, "FACT-B", "DB")
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	res, err := a.CardsNearby(ctx, CardsNearbyOpts{Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Certain < 2 {
		t.Fatalf("expected two certain boxes, got %+v", res)
	}
	eras, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(eras) != 0 {
		t.Fatalf("two certain factory_id must not persist a join: %+v", eras)
	}
}

func TestCardsNearbyEraSpanSkipsNonExclusiveDay(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cache := filepath.Join(t.TempDir(), "gps-stops.json")
	a := &App{Store: st, GPSStopsPath: cache}
	ctx := context.Background()
	ny := enterprise.NY()
	car := "26LSZW"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-CAR", DeviceID: "DEV-CAR", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-OTHER", DeviceID: "DEV-OTHER", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	var visits []model.StopVisit
	seedGeocodedSheetz(t, ctx, st, &visits)
	var lastFill time.Time
	for i := 0; i < 3; i++ {
		fill := time.Date(2026, 6, 10+i*3, 14, 20, 0, 0, ny)
		if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-HOME", fill)); err != nil {
			t.Fatal(err)
		}
		visits = append(visits, model.StopVisit{
			FactoryID: "FACT-CAR", DeviceID: "DEV-CAR",
			HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng,
			From: fill.Add(-5 * time.Minute), To: fill.Add(15 * time.Minute),
		})
	}
	other := time.Date(2026, 6, 28, 14, 0, 0, 0, ny)
	lastFill = other
	if err := st.UpsertCardTx(ctx, nearbySheetzTx("CARD-HOME", other)); err != nil {
		t.Fatal(err)
	}
	visits = append(visits, model.StopVisit{
		FactoryID: "FACT-OTHER", DeviceID: "DEV-OTHER",
		HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng,
		From: other.Add(-time.Minute), To: other.Add(time.Minute),
	})
	coverFactory(&visits, "FACT-GEO", "DEV-GEO")
	coverFactory(&visits, "FACT-CAR", "DEV-CAR")
	coverFactory(&visits, "FACT-OTHER", "DEV-OTHER")
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	res, err := a.CardsNearby(ctx, CardsNearbyOpts{Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Certain != 1 {
		t.Fatalf("certain %+v", res)
	}
	eras, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(eras) != 1 {
		t.Fatalf("eras %+v", eras)
	}
	if !eras[0].To.Before(lastFill) {
		t.Fatalf("persisted era must not include the other box's exclusive day: %+v last=%s", eras[0], lastFill)
	}
}
