package app

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/store"
)

func TestCardsHistoryFindsSplitCardErasWithNamedMerchants(t *testing.T) {
	p := filepath.Join(t.TempDir(), "history.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	day := ny(2026, 8, 1, 10)
	v15, t15 := historyExclusiveSits("27VA15", "CARD-MIX-99", 3, day, 37.54, -77.43)
	v19, t19 := historyExclusiveSits("27VA19", "CARD-MIX-99", 3, day.Add(10*24*time.Hour), 38.85, -77.05)
	cachePath := filepath.Join(t.TempDir(), "gps-stops.json")
	if err := cards.SaveStopVisits(cachePath, append(v15, v19...)); err != nil {
		t.Fatal(err)
	}

	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"}); err != nil {
		t.Fatal(err)
	}
	link15, link19 := "27VA15", "27VA19"
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "F15", DeviceID: "D15", DisplayName: "label-only",
		LinkedCarEFleetsID: &link15, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "F19", DeviceID: "D19", DisplayName: "label-only",
		LinkedCarEFleetsID: &link19, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	for _, tx := range append(t15, t19...) {
		if err := st.UpsertCardTx(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}

	a := &App{
		Cfg:          config.Config{SQLitePath: p},
		Store:        st,
		GPSStopsPath: cachePath,
	}
	res, err := a.CardsHistory(ctx, CardsHistoryOpts{
		DevicesOutPath: filepath.Join(t.TempDir(), "devices.csv"),
		NoGPS:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.TxN < 6 {
		t.Fatalf("txs %d want CARD-MIX-99 history", res.TxN)
	}
	if len(res.Ladder.Cars) != 1 || !res.Ladder.Cars[0].Split {
		t.Fatalf("want one SPLIT car card, got %+v", res.Ladder.Cars)
	}
	carEras := 0
	for _, e := range res.Ladder.Eras {
		if e.CardID == "CARD-MIX-99" && eraHolder(e) == cards.HolderCar {
			carEras++
		}
	}
	if carEras < 2 {
		t.Fatalf("want VA15+VA19 car eras, got %+v", res.Ladder.Eras)
	}
	persisted, err := st.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) == 0 {
		t.Fatal("card_eras must be persisted")
	}
}

func TestCardsHistoryTrackerMerchantsLockViaGPSPump(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tracker.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	at := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	lat, lng := 37.54, -77.43
	cachePath := filepath.Join(t.TempDir(), "gps-stops.json")
	visits := append(cardsPumpSeeds(lat, lng, at), model.StopVisit{
		EFleetsID: "27SGXD", HasPos: true, Lat: lat, Lng: lng,
		From: at, To: at.Add(10 * time.Minute),
	})
	if err := cards.SaveStopVisits(cachePath, visits); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27SGXD", Nickname: "BING-1", Region: "BING"}); err != nil {
		t.Fatal(err)
	}
	link := "27SGXD"
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "F1", DeviceID: "D1", LinkedCarEFleetsID: &link, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCardTx(ctx, model.CardTx{
		CardID: "x10000", At: at.Add(2 * time.Minute),
		StationName: "TRACKER", StationAddress: "1 MAIN,TOWN,VA",
		RecordedEFleetsID: "WRONG", DriverFirst: "FLEET", DriverLast: "DRIVER",
	}); err != nil {
		t.Fatal(err)
	}

	a := &App{Store: st, GPSStopsPath: cachePath}
	res, err := a.CardsHistory(ctx, CardsHistoryOpts{
		DevicesOutPath: filepath.Join(t.TempDir(), "devices.csv"),
		NoGPS:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ladder.Coverage.CardEraN != 1 || res.Ladder.Coverage.KnownN != 1 {
		t.Fatalf("GPS pump sit must name the card: cars=%+v cov=%+v", res.Ladder.Cars, res.Ladder.Coverage)
	}
}

func cardsPumpSeeds(lat, lng float64, around time.Time) []model.StopVisit {
	cars := []string{"27SEDA", "27SEDB", "27SEDC"}
	var out []model.StopVisit
	for i, c := range cars {
		t0 := around.Add(-time.Duration(i+3) * 24 * time.Hour)
		out = append(out, model.StopVisit{
			EFleetsID: c, HasPos: true, Lat: lat, Lng: lng,
			From: t0, To: t0.Add(8 * time.Minute),
		})
	}
	return out
}

func TestCardsHistoryDoesNotWriteLastReading(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lr.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	at := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27VA15", Nickname: "VA15"}); err != nil {
		t.Fatal(err)
	}
	if err := st.WriteLastReading(ctx, "27VA15", 100010, at, model.SourceFuelDetails); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st}
	if _, err := a.CardsHistory(ctx, CardsHistoryOpts{
		VehiclesPath:    testdata("enterprise", "fleetsummary_wrongcard.csv"),
		FuelDetailsPath: testdata("enterprise", "details_wrongcard.csv"),
		DevicesOutPath:  filepath.Join(t.TempDir(), "devices.csv"),
		NoGPS:           true,
	}); err != nil {
		t.Fatal(err)
	}
	car, err := st.CarByEFleets(ctx, "27VA15")
	if err != nil || car.LastReadingMiles == nil || *car.LastReadingMiles != 100010 {
		t.Fatalf("history must not touch Last Reading: %+v %v", car, err)
	}
}

func historyExclusiveSits(car, card string, n int, day0 time.Time, lat0, lng0 float64) ([]model.StopVisit, []model.CardTx) {
	var visits []model.StopVisit
	var txs []model.CardTx
	for i := 0; i < n; i++ {
		at := day0.Add(time.Duration(i*24) * time.Hour)
		name := fmt.Sprintf("PUMP%02d", i+1)
		addr := fmt.Sprintf("%d MAIN ST, TOWN, VA", i+1)
		visits = append(visits, model.StopVisit{
			EFleetsID: car, HasPos: true,
			Lat: lat0 + float64(i)*0.02, Lng: lng0 - float64(i)*0.02,
			From: at, To: at.Add(10 * time.Minute),
		})
		txs = append(txs, model.CardTx{
			CardID: card, At: at.Add(2 * time.Minute),
			StationName: name, StationAddress: addr, RecordedEFleetsID: car,
		})
	}
	return visits, txs
}

func ny(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, time.FixedZone("America/New_York", -4*3600))
}

func eraHolder(e model.CardEra) string {
	if e.HolderType == "" {
		return cards.HolderCar
	}
	return e.HolderType
}
