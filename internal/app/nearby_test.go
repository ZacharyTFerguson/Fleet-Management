package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/config"
	"oilchange/internal/enterprise"
	"oilchange/internal/model"
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
		From: time.Date(2026, 6, 9, 0, 0, 0, 0, ny),
		To:   time.Date(2026, 6, 20, 0, 0, 0, 0, ny),
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
	// FACT-GEO covered; FACT-LOOSE is not — incomplete cache must not invent exclusive.
	coverFactory(&visits, "FACT-GEO", "DEV-GEO")
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
