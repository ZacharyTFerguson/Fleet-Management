package cards

import (
	"strings"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestGPSFirstSplitsCardAcrossTwoCarsAtDifferentPumps(t *testing.T) {
	// VA19's card rides in VA15 at SHELL, then later is back in VA19 at MARATHON.
	shell := model.StopVisit{
		EFleetsID: "27VA15", HasPos: true, Lat: 37.54, Lng: -77.43,
		From: ny(2026, 8, 20, 10), To: ny(2026, 8, 20, 10).Add(12 * time.Minute),
	}
	shellLater := model.StopVisit{
		EFleetsID: "27VA15", HasPos: true, Lat: 37.54, Lng: -77.43,
		From: ny(2026, 8, 24, 10), To: ny(2026, 8, 24, 10).Add(10 * time.Minute),
	}
	marathon := model.StopVisit{
		EFleetsID: "27VA19", HasPos: true, Lat: 38.85, Lng: -77.05,
		From: ny(2026, 8, 25, 11), To: ny(2026, 8, 25, 11).Add(8 * time.Minute),
	}
	// A CT car is also sitting still at the SHELL second — time-only matching
	// would skip. Region + later geocode must still pick VA15.
	ctNoise := model.StopVisit{
		EFleetsID: "285JCG", HasPos: true, Lat: 41.76, Lng: -72.68,
		From: ny(2026, 8, 20, 10), To: ny(2026, 8, 20, 10).Add(15 * time.Minute),
	}
	ctAtShellTime := model.StopVisit{
		EFleetsID: "285JCG", HasPos: true, Lat: 41.76, Lng: -72.68,
		From: ny(2026, 8, 24, 10), To: ny(2026, 8, 24, 10).Add(15 * time.Minute),
	}
	txs := []model.CardTx{
		{CardID: "CARD-MIX-99", At: ny(2026, 8, 20, 10).Add(5 * time.Minute), StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA", RecordedEFleetsID: "27VA15"},
		{CardID: "CARD-MIX-99", At: ny(2026, 8, 24, 10).Add(4 * time.Minute), StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA", RecordedEFleetsID: "27VA15"},
		{CardID: "CARD-MIX-99", At: ny(2026, 8, 25, 11).Add(3 * time.Minute), StationName: "MARATHON", StationAddress: "2 MAIN ST, TOWN, VA", RecordedEFleetsID: "27VA19"},
	}
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
		{EFleetsID: "285JCG", Nickname: "CT1", Region: "CT"},
	}
	got := MatchGPSFirst([]model.StopVisit{shell, shellLater, marathon, ctNoise, ctAtShellTime}, txs, fleet, DefaultStopSlack)
	if len(got.Calls) != 3 {
		t.Fatalf("calls %d want 3: %+v", len(got.Calls), got.Calls)
	}
	if got.Calls[0].CalledCar != "27VA15" || got.Calls[0].CalledName != "VA15" {
		t.Fatalf("first swipe must be called VA15: %+v", got.Calls[0])
	}
	if got.Calls[2].CalledCar != "27VA19" || got.Calls[2].CalledName != "VA19" {
		t.Fatalf("third swipe must be called VA19: %+v", got.Calls[2])
	}
	if len(got.Eras) != 2 {
		t.Fatalf("eras %+v want CARD-MIX-99 split VA15 then VA19", got.Eras)
	}
	if !got.Eras[0].Split || got.Eras[0].EFleetsID != "27VA15" || got.Eras[1].EFleetsID != "27VA19" {
		t.Fatalf("split eras %+v", got.Eras)
	}
	scored := ApplyCalls(txs, got.Calls)
	if scored[0].CalledEFleetsID != "27VA15" || scored[0].RecordedEFleetsID != "27VA15" {
		t.Fatalf("ApplyCalls must not clobber Enterprise Vehicle: %+v", scored[0])
	}
}

func TestGPSFirstGeocodeDisambiguatesTwoCarsStopped(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	// First swipe: only VA15 at SHELL → geocode SHELL from that sit.
	first := ny(2026, 8, 29, 10)
	visits := []model.StopVisit{
		{EFleetsID: "27VA15", HasPos: true, Lat: 37.54, Lng: -77.43, From: first, To: first.Add(10 * time.Minute)},
		{EFleetsID: "27VA15", HasPos: true, Lat: 37.54, Lng: -77.43, From: at, To: at.Add(10 * time.Minute)},
		{EFleetsID: "27VA19", HasPos: true, Lat: 38.85, Lng: -77.05, From: at, To: at.Add(10 * time.Minute)},
	}
	txs := []model.CardTx{
		{CardID: "CARD-A", At: first.Add(2 * time.Minute), StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA", RecordedEFleetsID: "OTHER"},
		{CardID: "CARD-A", At: at.Add(2 * time.Minute), StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA", RecordedEFleetsID: "OTHER"},
	}
	got := MatchGPSFirst(visits, txs, []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}, DefaultStopSlack)
	if len(got.Calls) != 2 {
		t.Fatalf("both SHELL swipes should be VA15 after geocode, got %+v", got.Calls)
	}
	for _, c := range got.Calls {
		if c.CalledCar != "27VA15" {
			t.Fatalf("called %+v", c)
		}
		if c.EnterpriseCar != "OTHER" {
			t.Fatalf("enterprise column is evidence only: %+v", c)
		}
	}
	if len(got.Stations) == 0 {
		t.Fatal("SHELL should be geocoded from GPS")
	}
}

func TestGPSFirstRegionPicksHomeStateWhenTwoCarsSit(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	visits := []model.StopVisit{
		{EFleetsID: "27VA15", From: at, To: at.Add(10 * time.Minute)},
		{EFleetsID: "285JCG", From: at, To: at.Add(10 * time.Minute)},
	}
	txs := []model.CardTx{{
		CardID: "CARD-VA", At: at.Add(2 * time.Minute),
		StationName: "WAWA", StationAddress: "1 ST, RICHMOND, VA",
		RecordedEFleetsID: "WRONG",
	}}
	if got := MatchByStopTimes(visits, txs, DefaultStopSlack); len(got) != 0 {
		t.Fatalf("time-only matcher must still skip two sits: %+v", got)
	}
	got := MatchGPSFirst(visits, txs, []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "285JCG", Nickname: "CT1", Region: "CT"},
	}, DefaultStopSlack)
	if len(got.Calls) != 1 || got.Calls[0].CalledCar != "27VA15" || got.Calls[0].CalledName != "VA15" {
		t.Fatalf("VA pump + CT sit must call VA15: %+v", got.Calls)
	}
}

func TestGPSFirstSkipsTrackerWithoutPumpCluster(t *testing.T) {
	at := ny(2026, 8, 12, 10)
	visits := []model.StopVisit{{
		EFleetsID: "27SGXD", HasPos: true, Lat: 37.54, Lng: -77.43,
		From: at, To: at.Add(10 * time.Minute),
	}}
	txs := []model.CardTx{{
		CardID: "x10000", At: at.Add(2 * time.Minute),
		StationName: "TRACKER", StationAddress: "1 MAIN,TOWN,VA",
		RecordedEFleetsID: "27SGXD",
	}}
	got := MatchGPSFirst(visits, txs, []model.Car{{EFleetsID: "27SGXD", Nickname: "BING-1", Region: "BING"}}, DefaultStopSlack)
	if len(got.Calls) != 0 || len(got.Matches) != 0 {
		t.Fatalf("TRACKER at a one-off sit is not a pump: %+v", got)
	}
}

func TestGPSFirstNamesTrackerSwipeFromExclusivePumpSit(t *testing.T) {
	at := ny(2026, 8, 12, 10)
	lat, lng := 37.54, -77.43
	visits := append(pumpClusterSeeds(lat, lng, at), model.StopVisit{
		EFleetsID: "27SGXD", HasPos: true, Lat: lat, Lng: lng,
		From: at, To: at.Add(10 * time.Minute),
	})
	if n := len(clusterPumps(visits)); n == 0 {
		t.Fatalf("need a pump cluster from seeds, got %d", n)
	}
	txs := []model.CardTx{{
		CardID: "x10000", At: at.Add(2 * time.Minute),
		StationName: "TRACKER", StationAddress: "1 MAIN,TOWN,VA",
		RecordedEFleetsID: "WRONG-ENTERPRISE",
		DriverFirst:       "FLEET", DriverLast: "DRIVER",
	}}
	cands := uniqueCarsAt(bucketPumpVisits(visits, DefaultStopSlack), txs[0].At, DefaultStopSlack)
	if len(cands) != 1 || cands[0].EFleetsID != "27SGXD" {
		t.Fatalf("exclusive sit at swipe, got %+v", cands)
	}
	if !sitAtPump(cands[0], clusterPumps(visits)) {
		t.Fatal("exclusive sit must be on the seeded pump cluster")
	}
	got := MatchGPSFirst(visits, txs, []model.Car{{EFleetsID: "27SGXD", Nickname: "BING-1", Region: "BING"}}, DefaultStopSlack)
	if len(got.Calls) != 1 || got.Calls[0].CalledCar != "27SGXD" {
		t.Fatalf("exclusive GPS sit must name the card, not TRACKER/Enterprise: %+v", got.Calls)
	}
	if got.Calls[0].EnterpriseCar != "WRONG-ENTERPRISE" {
		t.Fatalf("Enterprise Vehicle is not identity: %+v", got.Calls[0])
	}
	if !strings.HasPrefix(got.Calls[0].Station, "gps:") {
		t.Fatalf("call station must be GPS cluster not TRACKER: %+v", got.Calls[0])
	}
	st := ""
	if len(got.Matches) == 1 && len(got.Matches[0].Stations) > 0 {
		st = got.Matches[0].Stations[0]
	}
	if !strings.HasPrefix(st, "gps:") {
		t.Fatalf("station must be GPS cluster not TRACKER: matches=%+v", got.Matches)
	}
}

func TestGPSFirstBackpropNamesEarlierTrackerSwipe(t *testing.T) {
	later := ny(2026, 8, 12, 10)
	lat, lng := 37.54, -77.43
	visits := append(pumpClusterSeeds(lat, lng, later), model.StopVisit{
		EFleetsID: "27SGXD", HasPos: true, Lat: lat, Lng: lng,
		From: later, To: later.Add(10 * time.Minute),
	})
	early := later.Add(-26 * time.Hour)
	txs := []model.CardTx{
		{
			CardID: "x10000", At: early.Add(2 * time.Minute),
			StationName: "TRACKER", StationAddress: "1 MAIN,TOWN,VA",
			RecordedEFleetsID: "WRONG-ENTERPRISE",
			DriverFirst:       "FLEET", DriverLast: "DRIVER",
		},
		{
			CardID: "x10000", At: later.Add(2 * time.Minute),
			StationName: "TRACKER", StationAddress: "1 MAIN,TOWN,VA",
			RecordedEFleetsID: "WRONG-ENTERPRISE",
			DriverFirst:       "FLEET", DriverLast: "DRIVER",
		},
	}
	got := MatchGPSFirst(visits, txs, []model.Car{{EFleetsID: "27SGXD", Nickname: "BING-1", Region: "BING"}}, DefaultStopSlack)
	if len(got.Calls) != 2 {
		t.Fatalf("later exclusive sit must backprop onto earlier TRACKER swipe: %+v", got.Calls)
	}
	var earlyCall, lateCall *RecordCall
	for i := range got.Calls {
		c := &got.Calls[i]
		switch {
		case c.At.Equal(txs[0].At):
			earlyCall = c
		case c.At.Equal(txs[1].At):
			lateCall = c
		}
	}
	if lateCall == nil || lateCall.CalledCar != "27SGXD" || lateCall.Why != "gps-stop" {
		t.Fatalf("later swipe must be gps-stop 27SGXD: %+v", lateCall)
	}
	if earlyCall == nil || earlyCall.CalledCar != "27SGXD" || earlyCall.Why != "backprop" {
		t.Fatalf("earlier TRACKER swipe must inherit GPS name: %+v", earlyCall)
	}
	if !strings.HasPrefix(earlyCall.Station, "gps:") {
		t.Fatalf("backprop station must be GPS cluster: %+v", earlyCall)
	}
}

func TestGPSFirstSkipsSimultaneousTrackerCards(t *testing.T) {
	at := ny(2026, 8, 12, 10)
	lat, lng := 37.54, -77.43
	visits := append(pumpClusterSeeds(lat, lng, at), model.StopVisit{
		EFleetsID: "27SGXD", HasPos: true, Lat: lat, Lng: lng,
		From: at, To: at.Add(10 * time.Minute),
	})
	txs := []model.CardTx{
		{CardID: "x10000", At: at.Add(2 * time.Minute), StationName: "TRACKER", RecordedEFleetsID: "WRONG-A"},
		{CardID: "x10001", At: at.Add(3 * time.Minute), StationName: "TRACKER", RecordedEFleetsID: "WRONG-B"},
	}
	got := MatchGPSFirst(visits, txs, []model.Car{{EFleetsID: "27SGXD", Nickname: "BING-1"}}, DefaultStopSlack)
	if len(got.Calls) != 0 {
		t.Fatalf("two TRACKER cards at once must not both inherit one GPS car: %+v", got.Calls)
	}
}

func TestGPSFirstTrackerSkipsLongLotSit(t *testing.T) {
	at := ny(2026, 8, 12, 10)
	lat, lng := 37.54, -77.43
	visits := append(pumpClusterSeeds(lat, lng, at), model.StopVisit{
		EFleetsID: "27SGXD", HasPos: true, Lat: lat, Lng: lng,
		From: at, To: at.Add(90 * time.Minute),
	})
	txs := []model.CardTx{{
		CardID: "x10000", At: at.Add(2 * time.Minute),
		StationName: "TRACKER", RecordedEFleetsID: "WRONG-ENTERPRISE",
	}}
	got := MatchGPSFirst(visits, txs, []model.Car{{EFleetsID: "27SGXD", Nickname: "BING-1"}}, DefaultStopSlack)
	if len(got.Calls) != 0 {
		t.Fatalf("90-minute lot sit is not a fuel: %+v", got.Calls)
	}
}

func pumpClusterSeeds(lat, lng float64, around time.Time) []model.StopVisit {
	cars := []string{"27SEDA", "27SEDB", "27SEDC"}
	var out []model.StopVisit
	for i, c := range cars {
		t := around.Add(-time.Duration(i+3) * 24 * time.Hour)
		out = append(out, model.StopVisit{
			EFleetsID: c, HasPos: true, Lat: lat, Lng: lng,
			From: t, To: t.Add(8 * time.Minute),
		})
	}
	return out
}

func TestGPSFirstPicksCarSittingAtSharedPump(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	earlier := ny(2026, 8, 20, 10)
	shellLat, shellLng := 37.54, -77.43
	homeLat, homeLng := 37.60, -77.50
	visits := []model.StopVisit{
		{EFleetsID: "27VA15", HasPos: true, Lat: shellLat, Lng: shellLng, From: at, To: at.Add(8 * time.Minute)},
		{EFleetsID: "27VA19", HasPos: true, Lat: homeLat, Lng: homeLng, From: at, To: at.Add(8 * time.Minute)},
		{EFleetsID: "27VA15", HasPos: true, Lat: shellLat, Lng: shellLng, From: earlier, To: earlier.Add(9 * time.Minute)},
		{EFleetsID: "27VA21", HasPos: true, Lat: shellLat, Lng: shellLng, From: earlier.Add(24 * time.Hour), To: earlier.Add(24*time.Hour + 7*time.Minute)},
		{EFleetsID: "27VA22", HasPos: true, Lat: shellLat, Lng: shellLng, From: earlier.Add(48 * time.Hour), To: earlier.Add(48*time.Hour + 6*time.Minute)},
	}
	txs := []model.CardTx{{
		CardID: "CARD-IN-VA15", At: at.Add(3 * time.Minute),
		StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA",
		RecordedEFleetsID: "WRONGCAR",
	}}
	got := MatchGPSFirst(visits, txs, []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
		{EFleetsID: "27VA21", Nickname: "VA21", Region: "VA"},
		{EFleetsID: "27VA22", Nickname: "VA22", Region: "VA"},
	}, DefaultStopSlack)
	if got.Pumps == 0 {
		t.Fatal("SHELL lot must cluster as a pump from three cars")
	}
	if len(got.Calls) != 1 || got.Calls[0].CalledCar != "27VA15" {
		t.Fatalf("card at SHELL must be called VA15, not the car sitting at home: %+v", got.Calls)
	}
}

func TestGPSFirstTwoBoxesAtPumpStayUnnamedWithoutFuel(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	lat, lng := 37.54, -77.43
	visits := append(pumpClusterSeeds(lat, lng, at),
		model.StopVisit{EFleetsID: "27VA15", FactoryID: "FACT-A", HasPos: true, Lat: lat, Lng: lng, From: at, To: at.Add(10 * time.Minute)},
		model.StopVisit{EFleetsID: "27VA19", FactoryID: "FACT-B", HasPos: true, Lat: lat, Lng: lng, From: at, To: at.Add(8 * time.Minute)},
	)
	txs := []model.CardTx{{
		CardID: "CARD-COLLIDE", At: at.Add(3 * time.Minute),
		StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA",
		RecordedEFleetsID: "WRONG",
	}}
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	got := MatchGPSFirst(visits, txs, fleet, DefaultStopSlack)
	if len(got.Calls) != 0 {
		t.Fatalf("two linked boxes in 350 m must stay unnamed without fuel history: %+v", got.Calls)
	}
	empty := MatchGPSFirstWithFuel(visits, txs, fleet, DefaultStopSlack, SeriesFuelLook(nil, nil))
	if len(empty.Calls) != 0 {
		t.Fatalf("empty fuel series must not invent a gauge: %+v", empty.Calls)
	}
}

func TestGPSFirstFuelJumpNamesUniqueRise(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	lat, lng := 37.54, -77.43
	visits := append(pumpClusterSeeds(lat, lng, at),
		model.StopVisit{EFleetsID: "27VA15", FactoryID: "FACT-A", HasPos: true, Lat: lat, Lng: lng, From: at, To: at.Add(10 * time.Minute)},
		model.StopVisit{EFleetsID: "27VA19", FactoryID: "FACT-B", HasPos: true, Lat: lat, Lng: lng, From: at, To: at.Add(8 * time.Minute)},
	)
	txs := []model.CardTx{{
		CardID: "CARD-COLLIDE", At: at.Add(3 * time.Minute),
		StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA",
		RecordedEFleetsID: "WRONG",
	}}
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	fill := txs[0].At
	look := SeriesFuelLook([]FuelPoint{
		{FactoryID: "FACT-A", At: fill.Add(-time.Hour), Level: 15},
		{FactoryID: "FACT-A", At: fill.Add(time.Hour), Level: 88},
		{FactoryID: "FACT-B", At: fill.Add(-time.Hour), Level: 62},
		{FactoryID: "FACT-B", At: fill.Add(time.Hour), Level: 48},
	}, map[string]float64{"FACT-A": 3, "FACT-B": 11})
	got := MatchGPSFirstWithFuel(visits, txs, fleet, DefaultStopSlack, look)
	if len(got.Calls) != 1 || got.Calls[0].CalledCar != "27VA15" {
		t.Fatalf("unique fuel rise must name VA15: %+v", got.Calls)
	}
	if got.Calls[0].EnterpriseCar != "WRONG" {
		t.Fatalf("Enterprise Vehicle is not identity: %+v", got.Calls[0])
	}
}

func TestGPSFirstFuelJumpBothRoseStaysUnnamed(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	lat, lng := 37.54, -77.43
	visits := append(pumpClusterSeeds(lat, lng, at),
		model.StopVisit{EFleetsID: "27VA15", FactoryID: "FACT-A", HasPos: true, Lat: lat, Lng: lng, From: at, To: at.Add(10 * time.Minute)},
		model.StopVisit{EFleetsID: "27VA19", FactoryID: "FACT-B", HasPos: true, Lat: lat, Lng: lng, From: at, To: at.Add(8 * time.Minute)},
	)
	txs := []model.CardTx{{
		CardID: "CARD-COLLIDE", At: at.Add(3 * time.Minute),
		StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA",
	}}
	fill := txs[0].At
	look := SeriesFuelLook([]FuelPoint{
		{FactoryID: "FACT-A", At: fill.Add(-time.Hour), Level: 20},
		{FactoryID: "FACT-A", At: fill.Add(time.Hour), Level: 80},
		{FactoryID: "FACT-B", At: fill.Add(-time.Hour), Level: 30},
		{FactoryID: "FACT-B", At: fill.Add(time.Hour), Level: 90},
	}, map[string]float64{"FACT-A": 1, "FACT-B": 1})
	got := MatchGPSFirstWithFuel(visits, txs, []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}, DefaultStopSlack, look)
	if len(got.Calls) != 0 {
		t.Fatalf("both rose must stay unnamed: %+v", got.Calls)
	}
}

func TestGPSFirstFuelJumpDoesNotLoosen350m(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	visits := []model.StopVisit{
		{EFleetsID: "27VA15", FactoryID: "FACT-A", HasPos: true, Lat: 37.54, Lng: -77.43, From: at, To: at.Add(10 * time.Minute)},
		{EFleetsID: "27VA19", FactoryID: "FACT-B", HasPos: true, Lat: 38.85, Lng: -77.05, From: at, To: at.Add(10 * time.Minute)},
	}
	txs := []model.CardTx{{
		CardID: "CARD-FAR", At: at.Add(2 * time.Minute),
		StationName: "WAWA", StationAddress: "1 ST, RICHMOND, VA",
	}}
	fill := txs[0].At
	look := SeriesFuelLook([]FuelPoint{
		{FactoryID: "FACT-A", At: fill.Add(-time.Hour), Level: 70},
		{FactoryID: "FACT-A", At: fill.Add(time.Hour), Level: 40},
		{FactoryID: "FACT-B", At: fill.Add(-time.Hour), Level: 10},
		{FactoryID: "FACT-B", At: fill.Add(time.Hour), Level: 95},
	}, map[string]float64{"FACT-A": 20, "FACT-B": 5})
	got := MatchGPSFirstWithFuel(visits, txs, []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}, DefaultStopSlack, look)
	for _, c := range got.Calls {
		if c.CalledCar == "27VA19" {
			t.Fatalf("fuel jump must not name a box outside 350 m: %+v", got.Calls)
		}
	}
}

func TestFillsWithFleetSightDropsOneBoxAugustFetch(t *testing.T) {
	var linked []model.OneStepDevice
	ids := []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8"}
	for _, id := range ids {
		car := "CAR-" + id
		linked = append(linked, model.OneStepDevice{
			FactoryID: id, Active: true, LinkedCarEFleetsID: &car,
		})
	}
	may := time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
	aug := time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC)
	var visits []model.StopVisit
	for _, id := range ids {
		visits = append(visits, model.StopVisit{
			FactoryID: id, From: may.Add(-24 * time.Hour), To: may.Add(48 * time.Hour),
		})
	}
	visits = append(visits, model.StopVisit{
		FactoryID: "F1", From: aug.Add(-24 * time.Hour), To: aug.Add(48 * time.Hour),
	})
	txs := []model.CardTx{{CardID: "MAY", At: may}, {CardID: "AUG", At: aug}}
	got := FillsWithFleetSight(txs, visits, linked)
	if len(got) != 1 || got[0].CardID != "MAY" {
		t.Fatalf("August one-box fetch must not be GPS-first exclusive: %+v", got)
	}
}
