package cards

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"oilchange/internal/model"
)

func exclusiveSits(car, card string, n int, day0 time.Time, lat0, lng0 float64) ([]model.StopVisit, []model.CardTx) {
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

func TestStationLadderLocksCardAtThreeExclusivePumps(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	visits, txs := exclusiveSits("27VA15", "CARD-15", 3, day, 37.54, -77.43)
	fleet := []model.Car{{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}}
	link := "27VA15"
	devs := []model.OneStepDevice{{FactoryID: "F15", DeviceID: "D15", DisplayName: "label-only", LinkedCarEFleetsID: &link, Active: true}}
	got := ClimbStationLadder(visits, txs, fleet, devs, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Rungs) != 3 || got.Rungs[0].Stations != 3 {
		t.Fatalf("rungs %+v", got.Rungs)
	}
	if len(got.Rungs[0].Cars) != 1 || got.Rungs[0].Cars[0].CardID != "CARD-15" || got.Rungs[0].Cars[0].HolderKey != "27VA15" {
		t.Fatalf("rung 3 cars %+v", got.Rungs[0].Cars)
	}
	if len(got.Rungs[1].Cars) != 0 {
		t.Fatalf("3 exclusive stations must not lock at 5: %+v", got.Rungs[1].Cars)
	}
	if got.Coverage.RosterN != 1 || got.Coverage.KnownN != 1 || got.Coverage.KnownPct != 100 {
		t.Fatalf("coverage %+v", got.Coverage)
	}
	if got.Coverage.DeviceLinkedN != 1 || got.Coverage.CardEraN != 1 {
		t.Fatalf("both device and card era required: %+v", got.Coverage)
	}
}

func TestStationLadderExpandsToFiveThenTenStations(t *testing.T) {
	day := ny(2026, 7, 1, 10)
	v3, t3 := exclusiveSits("27VA15", "CARD-3", 3, day, 37.50, -77.40)
	v5, t5 := exclusiveSits("27VA19", "CARD-5", 5, day.Add(40*24*time.Hour), 38.85, -77.05)
	v10, t10 := exclusiveSits("27VA21", "CARD-10", 10, day.Add(90*24*time.Hour), 37.20, -76.50)
	visits := append(append(v3, v5...), v10...)
	txs := append(append(t3, t5...), t10...)
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
		{EFleetsID: "27VA21", Nickname: "VA21", Region: "VA"},
	}
	got := ClimbStationLadder(visits, txs, fleet, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Rungs[0].Cars) != 3 {
		t.Fatalf("rung 3 should lock all three cards, got %+v", got.Rungs[0].Cars)
	}
	if len(got.Rungs[1].Cars) != 2 {
		t.Fatalf("rung 5 should lock CARD-5 and CARD-10, got %+v", got.Rungs[1].Cars)
	}
	if len(got.Rungs[2].Cars) != 1 || got.Rungs[2].Cars[0].CardID != "CARD-10" {
		t.Fatalf("rung 10 should lock only CARD-10, got %+v", got.Rungs[2].Cars)
	}
	by := map[string]int{}
	for _, c := range got.Cars {
		by[c.CardID] = c.Rung
	}
	if by["CARD-3"] != 3 || by["CARD-5"] != 5 || by["CARD-10"] != 10 {
		t.Fatalf("rungs by card %+v", by)
	}
}

func TestStationLadderSplitsCardSittingInTwoCars(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	v15, t15 := exclusiveSits("27VA15", "CARD-MIX-99", 3, day, 37.54, -77.43)
	v19, t19 := exclusiveSits("27VA19", "CARD-MIX-99", 3, day.Add(10*24*time.Hour), 38.85, -77.05)
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	got := ClimbStationLadder(append(v15, v19...), append(t15, t19...), fleet, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Cars) != 1 || !got.Cars[0].Split || got.Cars[0].CardID != "CARD-MIX-99" {
		t.Fatalf("want one SPLIT card, got %+v", got.Cars)
	}
	if len(got.Cars[0].Cars) != 2 {
		t.Fatalf("split cars %+v", got.Cars[0].Cars)
	}
	carEras := 0
	for _, e := range got.Eras {
		if e.CardID == "CARD-MIX-99" && eraHolderType(e) == HolderCar {
			carEras++
			if !e.Split {
				t.Fatalf("era must be SPLIT: %+v", e)
			}
		}
	}
	if carEras < 2 {
		t.Fatalf("want VA15 and VA19 eras, got %+v", got.Eras)
	}
}

func TestStationLadderDriverKeptCardStaysOnPerson(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	// One exclusive pump per car — below rung 3 — same driver holds the card.
	v15, t15 := exclusiveSits("27VA15", "CARD-DRV", 1, day, 37.54, -77.43)
	v19, t19 := exclusiveSits("27VA19", "CARD-DRV", 1, day.Add(48*time.Hour), 38.85, -77.05)
	t15[0].DriverFirst, t15[0].DriverLast = "PAT", "TECH"
	t19[0].DriverFirst, t19[0].DriverLast = "PAT", "TECH"
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	got := ClimbStationLadder(append(v15, v19...), append(t15, t19...), fleet, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Cars) != 0 {
		t.Fatalf("driver-kept card must not lock as a car: %+v", got.Cars)
	}
	if len(got.People) != 1 || got.People[0].HolderKey != "PAT TECH" {
		t.Fatalf("people %+v", got.People)
	}
	for _, e := range got.Eras {
		if e.CardID == "CARD-DRV" && eraHolderType(e) == HolderCar {
			t.Fatalf("driver-kept must not persist a car era: %+v", e)
		}
	}
}

func TestStationLadderLogisticsPersonnelNeverCreatesCarLink(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	visits, txs := exclusiveSits("27VA15", "CARD-TYLER", 3, day, 37.54, -77.43)
	for i := range txs {
		txs[i].DriverFirst, txs[i].DriverLast = "Tyler", "Spare"
	}
	fleet := []model.Car{{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}}
	link := "27VA15"
	// A box labeled for Tyler must not count as the car's factory_id pairing.
	devs := []model.OneStepDevice{{
		FactoryID: "TYLERBOX", DeviceID: "DX", DisplayName: "Tyler spare",
		LinkedCarEFleetsID: &link, Active: true,
	}}
	got := ClimbStationLadder(visits, txs, fleet, devs, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Cars) != 0 {
		t.Fatalf("logistics card must not lock a car: %+v", got.Cars)
	}
	if len(got.People) != 1 || got.People[0].CardID != "CARD-TYLER" {
		t.Fatalf("people %+v", got.People)
	}
	if got.Coverage.DeviceLinkedN != 0 {
		t.Fatalf("logistics display_name must not create a device↔car link: %+v", got.Coverage)
	}
	if got.Coverage.CardEraN != 0 || got.Coverage.KnownN != 0 {
		t.Fatalf("personnel card must not make the car known: %+v", got.Coverage)
	}
	for _, e := range got.Eras {
		if eraHolderType(e) == HolderCar {
			t.Fatalf("no car era from logistics card: %+v", e)
		}
	}
}

func TestStationLadderOfficeCardIsNotACar(t *testing.T) {
	at := ny(2026, 8, 12, 10)
	txs := []model.CardTx{{
		CardID: "CARD-OFF", At: at,
		StationName: "WAWA", StationAddress: "1 MAIN, TOWN, VA",
		RecordedEFleetsID: "PDI OFFICE", RecordedCVN: "HQ",
	}}
	got := ClimbStationLadder(nil, txs, []model.Car{{EFleetsID: "27VA15", Nickname: "VA15"}}, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Offices) != 1 || got.Offices[0].CardID != "CARD-OFF" {
		t.Fatalf("offices %+v", got.Offices)
	}
	if len(got.Cars) != 0 {
		t.Fatalf("office card is not a car: %+v", got.Cars)
	}
}

func TestStationLadderTrackerMerchantsYieldZeroCarLocks(t *testing.T) {
	at := ny(2026, 8, 12, 10)
	visits := []model.StopVisit{{
		EFleetsID: "27SGXD", HasPos: true, Lat: 37.54, Lng: -77.43,
		From: at, To: at.Add(10 * time.Minute),
	}}
	txs := []model.CardTx{{
		CardID: "x10000", At: at.Add(2 * time.Minute),
		StationName: "TRACKER", StationAddress: "1 MAIN,TOWN,VA",
		RecordedEFleetsID: "27SGXD", DriverFirst: "FLEET", DriverLast: "DRIVER",
	}}
	fleet := []model.Car{{EFleetsID: "27SGXD", Nickname: "BING-1", Region: "BING"}}
	link := "27SGXD"
	devs := []model.OneStepDevice{{FactoryID: "F1", DeviceID: "D1", LinkedCarEFleetsID: &link, Active: true}}
	got := ClimbStationLadder(visits, txs, fleet, devs, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Cars) != 0 || got.Coverage.CardEraN != 0 {
		t.Fatalf("TRACKER is not a pump name: cars=%+v cov=%+v", got.Cars, got.Coverage)
	}
	if len(got.People) != 0 {
		t.Fatalf("FLEET DRIVER is not a person who keeps a card: %+v", got.People)
	}
	if got.Coverage.DeviceLinkedN != 1 || got.Coverage.KnownN != 0 {
		t.Fatalf("device without named card era is not known: %+v", got.Coverage)
	}
	if !strings.Contains(got.Coverage.Blocked, "TRACKER") {
		t.Fatalf("blocker %q", got.Coverage.Blocked)
	}
}

func TestLadderBlockerWhenMostMerchantsAreTracker(t *testing.T) {
	var txs []model.CardTx
	at := ny(2026, 8, 12, 10)
	for i := 0; i < 10; i++ {
		txs = append(txs, model.CardTx{
			CardID: fmt.Sprintf("x%d", i), At: at,
			StationName: "TRACKER", RecordedEFleetsID: "27SGXD",
		})
	}
	txs = append(txs, model.CardTx{
		CardID: "xSHELL", At: at, StationName: "SHELL", StationAddress: "1 MAIN, TOWN, VA", RecordedEFleetsID: "27TESTA",
	})
	msg := LadderBlocker(txs, nil)
	if !strings.Contains(msg, "TRACKER") || !strings.Contains(msg, "named=1") {
		t.Fatalf("blocker %q", msg)
	}
}

func TestRosterCoverageRequiresDeviceAndCardEra(t *testing.T) {
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15"},
		{EFleetsID: "27VA19", Nickname: "VA19"},
		{EFleetsID: "TRACKER", Nickname: "nope"},
	}
	link := "27VA15"
	devs := []model.OneStepDevice{{FactoryID: "F15", DeviceID: "D15", LinkedCarEFleetsID: &link, Active: true}}
	eras := []CardEra{
		{CardID: "CARD-15", EFleetsID: "27VA15", HolderType: HolderCar, HolderKey: "27VA15"},
	}
	cov := RosterCoverage(fleet, devs, eras, nil)
	if cov.RosterN != 2 {
		t.Fatalf("TRACKER is not a roster car, roster=%d", cov.RosterN)
	}
	if cov.KnownN != 1 || cov.DeviceLinkedN != 1 || cov.CardEraN != 1 {
		t.Fatalf("only 27VA15 is known: %+v", cov)
	}
	if len(cov.UnknownRemaining) != 1 || cov.UnknownRemaining[0] != "27VA19" {
		t.Fatalf("unknown %+v", cov.UnknownRemaining)
	}
	if cov.KnownPct < 49 || cov.KnownPct > 51 {
		t.Fatalf("pct %f", cov.KnownPct)
	}
}

func TestScorePairingsLogisticsPersonnelDoesNotVoteCar(t *testing.T) {
	txs := []model.CardTx{{
		CardID: "CARD-TYLER", At: ny(2026, 8, 20, 10),
		StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA",
		RecordedEFleetsID: "27VA15", DriverFirst: "Tyler", DriverLast: "Spare",
	}}
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	for _, p := range ps {
		if p.EntityType == "car" {
			t.Fatalf("logistics swipe must not vote a car: %+v", ps)
		}
	}
	found := false
	for _, p := range ps {
		if p.EntityType == "person" && strings.Contains(p.EntityKey, "TYLER") {
			found = true
		}
	}
	if !found {
		t.Fatalf("person vote still stored for audit: %+v", ps)
	}
}

func TestScorePairingsOfficeDoesNotVoteCar(t *testing.T) {
	txs := []model.CardTx{{
		CardID: "CARD-OFF", At: ny(2026, 8, 20, 10),
		StationName: "WAWA", StationAddress: "1 MAIN, TOWN, VA",
		RecordedEFleetsID: "PDI OFFICE",
	}}
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	for _, p := range ps {
		if p.EntityType == "car" {
			t.Fatalf("office must not be a car: %+v", ps)
		}
	}
	found := false
	for _, p := range ps {
		if p.EntityType == "office" && p.EntityKey == "PDI OFFICE" && p.Best {
			found = true
		}
	}
	if !found {
		t.Fatalf("want office BEST, got %+v", ps)
	}
}

func TestTRACKERRecordedVehicleIsNotACar(t *testing.T) {
	if !isUnknownCar("TRACKER") || !isUnknownCar("tracker") {
		t.Fatal("TRACKER is not a car")
	}
	txs := []model.CardTx{{
		CardID: "CARD-X", At: ny(2026, 8, 20, 10),
		StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA",
		RecordedEFleetsID: "TRACKER",
	}}
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	for _, p := range ps {
		if p.EntityType == "car" {
			t.Fatalf("TRACKER voted as a car: %+v", ps)
		}
	}
}
