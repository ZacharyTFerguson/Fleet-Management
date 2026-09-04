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
	lat, lng := 40.61716, -79.722
	var visits []model.StopVisit
	for i := 0; i < 3; i++ {
		fill := time.Date(2026, 6, 10+i*3, 14, 20, 0, 0, ny)
		tx := model.CardTx{
			CardID: "CARD-HOME", At: fill.UTC(),
			StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
			RecordedEFleetsID: "WRONG",
		}
		if err := st.UpsertCardTx(ctx, tx); err != nil {
			t.Fatal(err)
		}
		visits = append(visits, model.StopVisit{
			FactoryID: "FACT-CAR", DeviceID: "DEV-CAR", EFleetsID: car,
			HasPos: true, Lat: lat, Lng: lng,
			From: fill.Add(-5 * time.Minute), To: fill.Add(15 * time.Minute),
		})
	}
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		t.Fatal(err)
	}
	res, err := a.CardsNearby(ctx, CardsNearbyOpts{Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Certain != 1 {
		t.Fatalf("certain %d %+v", res.Certain, res)
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
