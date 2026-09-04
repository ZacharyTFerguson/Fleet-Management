package app

import (
	"context"
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

func TestCardsWatchFetchesOnlyWatchedBoxes(t *testing.T) {
	prev := watchDriveStopMinInterval
	watchDriveStopMinInterval = 20 * time.Millisecond
	defer func() { watchDriveStopMinInterval = prev }()

	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var hits int32
	var sawUnwatched int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device"):
			_, _ = w.Write([]byte(`{"result_list":[
				{"factory_id":"FACT-VA","device_id":"DEV-VA","active":true},
				{"factory_id":"FACT-OTHER","device_id":"DEV-OTHER","active":true},
				{"factory_id":"FACT-GEO","device_id":"DEV-GEO","active":true}
			]}`))
		case strings.Contains(r.URL.Path, "/route/drive-stop"):
			atomic.AddInt32(&hits, 1)
			did := r.URL.Query().Get("device_id")
			if strings.Contains(did, ",") {
				t.Errorf("must not invent multi-device_id: %q", did)
			}
			if did == "DEV-OTHER" {
				atomic.AddInt32(&sawUnwatched, 1)
			}
			_, _ = w.Write([]byte(`{"drive_stop_list":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	cache := filepath.Join(t.TempDir(), "gps-stops.json")
	a := &App{Cfg: config.Config{SQLitePath: p}, Store: st, GPSStopsPath: cache, OneStep: c}
	ctx := context.Background()
	ny := enterprise.NY()
	va := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: va, Nickname: "292NCX", Region: "VA"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &va, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	other := "26LSZW"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: other, Nickname: "Bing-2"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-OTHER", DeviceID: "DEV-OTHER", LinkedCarEFleetsID: &other, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	var visits []model.StopVisit
	seedGeocodedSheetz(t, ctx, st, &visits)
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	fill := time.Date(2026, 8, 20, 14, 0, 0, 0, ny)
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: fill.UTC(), RecordedEFleetsID: va,
		StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := a.CardsWatch(ctx, CardsWatchOpts{LiveStops: true, Pace: 20 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) < 1 {
		t.Fatalf("expected watched drive-stop, hits=%d", hits)
	}
	if atomic.LoadInt32(&sawUnwatched) != 0 {
		t.Fatalf("fetched unwatched FACT-OTHER")
	}
}

func TestCardsWatchPersistsWhenWatchedSetComplete(t *testing.T) {
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
	car := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car, Nickname: "292NCX", Region: "VA"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &car, Active: true,
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
		fill := time.Date(2026, 8, 4+i*3, 14, 20, 0, 0, ny)
		if err := st.UpsertCardTx(ctx, model.CardTx{
			CardID: "CARD-VA", At: fill.UTC(), RecordedEFleetsID: car,
			StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
		}); err != nil {
			t.Fatal(err)
		}
		visits = append(visits, model.StopVisit{
			FactoryID: "FACT-VA", DeviceID: "DEV-VA",
			HasPos: true, Lat: nearbyPumpLat, Lng: nearbyPumpLng,
			From: fill.Add(-5 * time.Minute), To: fill.Add(15 * time.Minute),
		})
	}
	from, to := cards.UnionFillDayWindow([]model.CardTx{{
		At: time.Date(2026, 8, 4, 14, 20, 0, 0, ny),
	}, {
		At: time.Date(2026, 8, 10, 14, 20, 0, 0, ny),
	}})
	visits = append(visits, model.StopVisit{FactoryID: "FACT-VA", DeviceID: "DEV-VA", From: from, To: to})
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	res, err := a.CardsWatch(ctx, CardsWatchOpts{Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Certain != 1 {
		t.Fatalf("certain %d complete=%v %+v", res.Certain, res.CoverageComplete, res)
	}
	eras, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(eras) != 1 || eras[0].EFleetsID != car || eras[0].CardID != "CARD-VA" {
		t.Fatalf("eras %+v", eras)
	}
	got, err := st.CarByEFleets(ctx, car)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastReadingMiles != nil {
		t.Fatalf("Last Reading must stay empty")
	}
}

func TestCardsWatchDoesNotHitOneStepWithoutLive(t *testing.T) {
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
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "missing.json"), OneStep: c}
	ctx := context.Background()
	car := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	ny := enterprise.NY()
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: time.Date(2026, 8, 20, 14, 0, 0, 0, ny).UTC(),
		RecordedEFleetsID: car, StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CardsWatch(ctx, CardsWatchOpts{LiveStops: false}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("watch without --live must not call OneStep, hits=%d", hits)
	}
}

func TestCardsWatchHonorsRetryAfter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device"):
			_, _ = w.Write([]byte(`{"result_list":[{"factory_id":"FACT-VA","device_id":"DEV-VA","active":true}]}`))
		case strings.Contains(r.URL.Path, "/route/drive-stop"):
			n := atomic.AddInt32(&hits, 1)
			if n == 1 {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "slow", http.StatusTooManyRequests)
				return
			}
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
	va := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: va, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &va, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: time.Date(2026, 8, 20, 14, 0, 0, 0, ny).UTC(),
		RecordedEFleetsID: va, StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := a.CardsWatch(ctx, CardsWatchOpts{LiveStops: true, Pace: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if atomic.LoadInt32(&hits) < 2 {
		t.Fatalf("expected retry after 429, hits=%d", hits)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("Retry-After 1s must delay, elapsed=%s", elapsed)
	}
}

func TestCardsWatchDoesNotFetchGPSFirstFleet(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		if strings.Contains(r.URL.Path, "/device") {
			_, _ = w.Write([]byte(`{"result_list":[{"factory_id":"FACT-VA","device_id":"DEV-VA","active":true}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "/route/drive-stop") {
			_, _ = w.Write([]byte(`{"drive_stop_list":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "no-cache.json"), OneStep: c}
	ctx := context.Background()
	va := "292NCX"
	other := "26LSZW"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: va, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: other, Nickname: "Bing-2"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &va, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-NY", DeviceID: "DEV-NY", LinkedCarEFleetsID: &other, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	ny := enterprise.NY()
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: time.Date(2026, 8, 20, 14, 0, 0, 0, ny).UTC(),
		RecordedEFleetsID: va, StationName: "SHEETZ",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CardsWatch(ctx, CardsWatchOpts{LiveStops: true, Pace: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	var drive int
	for _, p := range paths {
		if strings.Contains(p, "drive-stop") {
			drive++
			if strings.Contains(p, "DEV-NY") {
				t.Fatalf("GPS-first must not fleet-fetch: %s", p)
			}
		}
	}
	if drive != 1 {
		t.Fatalf("expected 1 watched drive-stop, got %d %v", drive, paths)
	}
}

func TestCardsRebuildPreservesNearbyCertainEra(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "gps.json")}
	ctx := context.Background()
	car := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-NCX", DeviceID: "DEV-NCX", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceEras(ctx, []model.CardEra{{
		CardID: "NEAR-CARD", EFleetsID: car, HolderType: cards.HolderCar, HolderKey: car,
		From:      time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC),
		To:        time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC),
		EvidenceN: 3,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "NEAR-CARD", At: time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC),
		RecordedEFleetsID: car, StationName: "SHEETZ",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CardsRebuild(ctx, ""); err != nil {
		t.Fatal(err)
	}
	eras, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range eras {
		if e.CardID == "NEAR-CARD" && e.EFleetsID == car {
			found = true
		}
	}
	if !found {
		t.Fatalf("nearby certain era wiped: %+v", eras)
	}
	ladder, err := a.CardsLadder(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if ladder.Coverage.CardEraN != 1 || ladder.Coverage.KnownN != 1 {
		t.Fatalf("preserve must count in coverage after rebuild: %+v", ladder.Coverage)
	}
}

func TestCardsWatchDoesNotRetryHTTP500(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device"):
			_, _ = w.Write([]byte(`{"result_list":[{"factory_id":"FACT-VA","device_id":"DEV-VA","active":true}]}`))
		case strings.Contains(r.URL.Path, "/route/drive-stop"):
			atomic.AddInt32(&hits, 1)
			w.Header().Set("Retry-After", "1")
			http.Error(w, "nope", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "gps.json"), OneStep: c}
	ctx := context.Background()
	ny := enterprise.NY()
	va := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: va, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &va, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: time.Date(2026, 8, 20, 14, 0, 0, 0, ny).UTC(),
		RecordedEFleetsID: va, StationName: "SHEETZ",
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := a.CardsWatch(ctx, CardsWatchOpts{LiveStops: true, Pace: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("500 must not Retry-After retry, hits=%d", hits)
	}
	if time.Since(start) >= 900*time.Millisecond {
		t.Fatalf("must not wait Retry-After on 500")
	}
}

func TestCardsWatchCapsNewestFillsForFetchWindow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var fromQ, toQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device"):
			_, _ = w.Write([]byte(`{"result_list":[{"factory_id":"FACT-VA","device_id":"DEV-VA","active":true}]}`))
		case strings.Contains(r.URL.Path, "/route/drive-stop"):
			fromQ = r.URL.Query().Get("dt_tracker_from")
			toQ = r.URL.Query().Get("dt_tracker_to")
			_, _ = w.Write([]byte(`{"drive_stop_list":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "gps.json"), OneStep: c}
	ctx := context.Background()
	ny := enterprise.NY()
	va := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: va, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &va, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	// 12 punches; --fills 2 should only cover the newest two fill-day windows (Aug 20–21), not June.
	for i := 0; i < 12; i++ {
		day := 10 + i
		if err := st.UpsertCardTx(ctx, model.CardTx{
			CardID: "CARD-VA", At: time.Date(2026, 6, day, 14, 0, 0, 0, ny).UTC(),
			RecordedEFleetsID: va, StationName: "SHEETZ",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: time.Date(2026, 8, 21, 14, 0, 0, 0, ny).UTC(),
		RecordedEFleetsID: va, StationName: "SHEETZ",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: time.Date(2026, 8, 20, 14, 0, 0, 0, ny).UTC(),
		RecordedEFleetsID: va, StationName: "SHEETZ",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CardsWatch(ctx, CardsWatchOpts{LiveStops: true, Fills: 2, Pace: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if fromQ == "" {
		t.Fatal("no drive-stop")
	}
	fromT, err := time.Parse(time.RFC3339, fromQ)
	if err != nil {
		t.Fatal(err)
	}
	// Newest two are Aug 21 and Aug 20 → window starts Aug 19 Eastern, not June.
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !fromT.After(june) {
		t.Fatalf("fetch window must be newest fills, from=%s to=%s", fromQ, toQ)
	}
}

func TestCardsWatchPairsUnpairedBoxByVINAfterLive(t *testing.T) {
	prev := watchDriveStopMinInterval
	watchDriveStopMinInterval = time.Millisecond
	defer func() { watchDriveStopMinInterval = prev }()

	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var askedVIN int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device"):
			did := r.URL.Query().Get("device_id")
			if did == "DEV-LOOSE" && r.URL.Query().Get("limit") == "" {
				atomic.AddInt32(&askedVIN, 1)
				_, _ = w.Write([]byte(`[{
					"factory_id":"FACT-LOOSE","device_id":"DEV-LOOSE","display_name":"WrongCar",
					"latest_device_point":{"device_state":{"vin":"1HGCM82633A004352"},"params":{"vin":"IGNORED"}}
				}]`))
				return
			}
			_, _ = w.Write([]byte(`{"result_list":[
				{"factory_id":"FACT-VA","device_id":"DEV-VA","active":true},
				{"factory_id":"FACT-LOOSE","device_id":"DEV-LOOSE","display_name":"WrongCar","active":true}
			]}`))
		case strings.Contains(r.URL.Path, "/route/drive-stop"):
			_, _ = w.Write([]byte(`{"drive_stop_list":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Cfg: config.Config{OneStepToken: "tok"}, Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "gps.json"), OneStep: c}
	ctx := context.Background()
	ny := enterprise.NY()
	va := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: va, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", VIN: "1HGCM82633A004352", Nickname: "WrongCar"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &va, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-LOOSE", DeviceID: "DEV-LOOSE", DisplayName: "WrongCar", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "CARD-VA", At: time.Date(2026, 8, 20, 14, 0, 0, 0, ny).UTC(),
		RecordedEFleetsID: va, StationName: "SHEETZ",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CardsWatch(ctx, CardsWatchOpts{LiveStops: true, Pace: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&askedVIN) < 1 {
		t.Fatal("watch must GET /device?device_id= for unpaired VIN")
	}
	got, err := st.ListDevicesForCar(ctx, "27TESTA")
	if err != nil || len(got) != 1 || got[0].FactoryID != "FACT-LOOSE" {
		t.Fatalf("VIN must pair unpaired box to Enterprise car: %+v %v", got, err)
	}
	keep, err := st.ListDevicesForCar(ctx, va)
	if err != nil || len(keep) != 1 || keep[0].FactoryID != "FACT-VA" {
		t.Fatalf("must not steal VA map: %+v %v", keep, err)
	}
}

func TestCardsLadderCoverageIncludesPreservedWatchEra(t *testing.T) {
	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	car := "292NCX"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: car, Nickname: "292NCX"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT-VA", DeviceID: "DEV-VA", LinkedCarEFleetsID: &car, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceEras(ctx, []model.CardEra{{
		CardID: "WATCH-CARD", EFleetsID: car, HolderType: cards.HolderCar, HolderKey: car,
		From:      time.Date(2026, 8, 4, 14, 0, 0, 0, time.UTC),
		To:        time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC),
		EvidenceN: 3,
	}}); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st, GPSStopsPath: filepath.Join(t.TempDir(), "gps.json")}
	res, err := a.CardsLadder(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Coverage.CardEraN != 1 || res.Coverage.KnownN != 1 {
		t.Fatalf("preserved watch era must count as known: %+v", res.Coverage)
	}
}
