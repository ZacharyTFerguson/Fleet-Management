package cards

import (
	"strings"
	"testing"
	"time"

	"oilchange/internal/enterprise"
	"oilchange/internal/model"
)

func TestFillDayWindowIsDayBeforeThroughDayAfterEastern(t *testing.T) {
	ny := enterprise.NY()
	fill := time.Date(2026, 6, 16, 10, 14, 0, 0, ny)
	from, to := FillDayWindow(fill)
	wantFrom := time.Date(2026, 6, 15, 0, 0, 0, 0, ny).UTC()
	wantTo := time.Date(2026, 6, 18, 0, 0, 0, 0, ny).Add(-time.Millisecond).UTC()
	if !from.Equal(wantFrom) {
		t.Fatalf("from %s want %s", from, wantFrom)
	}
	if !to.Equal(wantTo) {
		t.Fatalf("to %s want %s", to, wantTo)
	}
	// Bank posting a day later must not be used; window is anchored on swipe time.
	bank := fill.Add(24 * time.Hour)
	bFrom, _ := FillDayWindow(bank)
	if bFrom.Equal(from) {
		t.Fatal("bank posting day must not produce the same window as swipe day")
	}
}

func TestHuntNearbyOneHitIsWatchNotJoin(t *testing.T) {
	ny := enterprise.NY()
	fill := time.Date(2026, 6, 16, 14, 20, 0, 0, ny)
	lat, lng := 40.61716, -79.722
	txs := []model.CardTx{{
		CardID: "CARD-UNK", At: fill.UTC(),
		StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
		RecordedEFleetsID: "TRACKER",
	}}
	visits := []model.StopVisit{{
		FactoryID: "FACT-WATCH", DeviceID: "DEV-WATCH", HasPos: true, Lat: lat, Lng: lng,
		From: fill.Add(-10 * time.Minute), To: fill.Add(10 * time.Minute),
	}}
	stations := []GeocodedStation{{
		Key:  "sheetz|203 craigdell rd, lower burrell, pa",
		Name: "SHEETZ", Address: "203 CRAIGDELL RD, LOWER BURRELL, PA",
		Lat: lat, Lng: lng, Hits: 1,
	}}
	res := HuntNearby(visits, txs, stations, nil, DefaultStopSlack)
	if len(res.Cards) != 1 || res.Watch != 1 || res.Certain != 0 {
		t.Fatalf("one hit must be watch, not certain: %+v", res)
	}
	if res.Cards[0].Watch[0].FactoryID != "FACT-WATCH" {
		t.Fatalf("%+v", res.Cards[0])
	}
}

func TestHuntNearbyThreeExclusiveFillsIsCertainLinkedCar(t *testing.T) {
	ny := enterprise.NY()
	lat, lng := 40.61716, -79.722
	car := "26LSZW"
	dev := model.OneStepDevice{FactoryID: "FACT-CAR", DeviceID: "DEV-CAR", LinkedCarEFleetsID: &car}
	var txs []model.CardTx
	var visits []model.StopVisit
	for i := 0; i < 3; i++ {
		fill := time.Date(2026, 6, 10+i*3, 14, 20, 0, 0, ny)
		txs = append(txs, model.CardTx{
			CardID: "CARD-HOME", At: fill.UTC(),
			StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
			RecordedEFleetsID: "WRONG",
		})
		visits = append(visits, model.StopVisit{
			FactoryID: "FACT-CAR", DeviceID: "DEV-CAR", EFleetsID: car,
			HasPos: true, Lat: lat, Lng: lng,
			From: fill.Add(-5 * time.Minute), To: fill.Add(15 * time.Minute),
		})
	}
	stations := []GeocodedStation{{
		Name: "SHEETZ", Address: "203 CRAIGDELL RD, LOWER BURRELL, PA",
		Lat: lat, Lng: lng, Hits: 3,
	}}
	res := HuntNearby(visits, txs, stations, []model.OneStepDevice{dev}, DefaultStopSlack)
	if res.Certain != 1 || len(res.Cards) != 1 || len(res.Cards[0].Certain) != 1 {
		t.Fatalf("certain %+v", res)
	}
	got := res.Cards[0].Certain[0]
	if got.FactoryID != "FACT-CAR" || got.LinkedCar != car || got.ExclusiveFills != 3 {
		t.Fatalf("%+v", got)
	}
	linked := CertainLinkedCars(res)
	if len(linked) != 1 || linked[0].LinkedCar != car {
		t.Fatalf("linked %+v", linked)
	}
}

func TestHuntNearbyTwoBoxesAtFillStayWatch(t *testing.T) {
	ny := enterprise.NY()
	fill := time.Date(2026, 6, 16, 14, 20, 0, 0, ny)
	lat, lng := 40.61716, -79.722
	txs := []model.CardTx{{
		CardID: "CARD-SPLIT", At: fill.UTC(),
		StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
	}}
	visits := []model.StopVisit{
		{FactoryID: "A", DeviceID: "DA", HasPos: true, Lat: lat, Lng: lng, From: fill.Add(-time.Minute), To: fill.Add(time.Minute)},
		{FactoryID: "B", DeviceID: "DB", HasPos: true, Lat: lat + 0.001, Lng: lng, From: fill.Add(-time.Minute), To: fill.Add(time.Minute)},
	}
	stations := []GeocodedStation{{Name: "SHEETZ", Address: "203 CRAIGDELL RD, LOWER BURRELL, PA", Lat: lat, Lng: lng}}
	res := HuntNearby(visits, txs, stations, nil, DefaultStopSlack)
	if res.Certain != 0 || res.Likely != 0 || res.Watch < 2 {
		t.Fatalf("collision must stay watch: %+v", res)
	}
}

func TestHuntNearbyIgnoresBeyondOneMileAndDisplayName(t *testing.T) {
	ny := enterprise.NY()
	fill := time.Date(2026, 6, 16, 14, 20, 0, 0, ny)
	lat, lng := 40.61716, -79.722
	txs := []model.CardTx{{
		CardID: "CARD-FAR", At: fill.UTC(),
		StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
	}}
	// ~5 miles east
	visits := []model.StopVisit{{
		FactoryID: "FAR", DeviceID: "DFAR", HasPos: true, Lat: lat, Lng: lng + 0.1,
		From: fill.Add(-time.Minute), To: fill.Add(time.Minute),
	}}
	stations := []GeocodedStation{{Name: "SHEETZ", Address: "203 CRAIGDELL RD, LOWER BURRELL, PA", Lat: lat, Lng: lng}}
	res := HuntNearby(visits, txs, stations, nil, DefaultStopSlack)
	if res.Watch != 0 {
		t.Fatalf("beyond 1 mile: %+v", res)
	}
}

func TestHuntNearbySkipsLogisticsPersonnelBox(t *testing.T) {
	ny := enterprise.NY()
	fill := time.Date(2026, 6, 16, 14, 20, 0, 0, ny)
	lat, lng := 40.61716, -79.722
	txs := []model.CardTx{{
		CardID: "CARD-RICH", At: fill.UTC(),
		StationName: "SHEETZ", StationAddress: "203 CRAIGDELL RD, LOWER BURRELL, PA",
		DriverFirst: "Rich",
	}}
	visits := []model.StopVisit{{
		FactoryID: "RICHBOX", HasPos: true, Lat: lat, Lng: lng,
		From: fill.Add(-time.Minute), To: fill.Add(time.Minute),
	}}
	stations := []GeocodedStation{{Name: "SHEETZ", Address: "203 CRAIGDELL RD, LOWER BURRELL, PA", Lat: lat, Lng: lng}}
	res := HuntNearby(visits, txs, stations, nil, DefaultStopSlack)
	if len(res.Cards) != 0 && res.Watch+res.Certain+res.Likely != 0 {
		t.Fatalf("logistics swipe must not hunt a join: %+v", res)
	}
}

func TestFormatNearbyDoesNotTreatEntityAsJoin(t *testing.T) {
	s := FormatNearby(NearbyResult{Cards: []NearbyCard{{
		CardID: "C1", Fills: 1,
		Watch: []NearbyDevice{{FactoryID: "F1", DeviceID: "D1", Rank: NearbyWatch}},
	}}})
	if !strings.Contains(s, "factory_id=F1") {
		t.Fatal(s)
	}
	if strings.Contains(s, "display_name") {
		t.Fatal(s)
	}
}
