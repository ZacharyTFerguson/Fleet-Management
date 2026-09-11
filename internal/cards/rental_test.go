package cards

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestRentalLabelsAreNotVirginiaOrVA19(t *testing.T) {
	if IsVirginiaVehicle("Unknown", "VA RENTAL 2", nil) {
		t.Fatal("VA RENTAL 2 leading VA must not seed a Virginia watch")
	}
	if IsVirginiaVehicle("Unknown", "Rental 1", nil) {
		t.Fatal("Rental 1 is not VA19")
	}
	if !IsVirginiaVehicle("27VA19", "VA19", nil) {
		t.Fatal("27VA19 / VA19 stay Virginia")
	}
	if !IsVirginiaVehicle("27SGXP", "VA19", []model.Car{{EFleetsID: "27SGXP", Nickname: "VA19", Region: "VA"}}) {
		t.Fatal("live VA19 nickname still Virginia")
	}
}

func TestRentalCardsDoNotDumpErasOntoVA19(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	// VA19 control: CARD-19, one exclusive-sit fill (testdata details_wrongcard).
	v19, t19 := exclusiveSits("27VA19", "CARD-19", 1, day, 38.85, -77.05)
	t19[0].RecordedCVN = "VA19"

	// Rental 1 / Rental 2 swipe at the same pumps while VA19's box is stopped.
	// Three exclusive sits would lock a car if rental were treated as a fleet unit.
	vR1, tR1 := exclusiveSits("27VA19", "CARD-RENT-1", 3, day, 38.85, -77.05)
	for i := range tR1 {
		tR1[i].RecordedEFleetsID = "Unknown"
		tR1[i].RecordedCVN = "Rental 1"
		tR1[i].DriverFirst, tR1[i].DriverLast = "ZACH", "FERGUSON"
	}
	vR2, tR2 := exclusiveSits("27VA19", "CARD-RENT-2", 3, day.Add(24*time.Hour), 38.85, -77.05)
	for i := range tR2 {
		tR2[i].RecordedEFleetsID = "Unknown"
		tR2[i].RecordedCVN = "VA RENTAL 2"
		tR2[i].DriverFirst, tR2[i].DriverLast = "MARK", "TEWFICK"
	}

	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	link19 := "27VA19"
	devs := []model.OneStepDevice{
		{FactoryID: "3271045657", DeviceID: "D19", DisplayName: "VA-19: Lindsey Glazier G,V DFR", LinkedCarEFleetsID: &link19, Active: true},
		{FactoryID: "351358810724200", DeviceID: "DRENT1", DisplayName: "Rental 1", Active: true},
		{FactoryID: "350457794656486", DeviceID: "DRENT2", DisplayName: "NYC-6: Julian (PDI Rental #2)", Active: true},
	}
	// A wrongly linked rental box must not count as VA19's factory_id pairing.
	wrong := "27VA19"
	devs = append(devs, model.OneStepDevice{
		FactoryID: "351358810740784", DeviceID: "DRENT4", DisplayName: "Pdi Rental 4",
		LinkedCarEFleetsID: &wrong, Active: true,
	})

	visits := append(append(v19, vR1...), vR2...)
	txs := append(append(t19, tR1...), tR2...)
	got := ClimbStationLadder(visits, txs, fleet, devs, DefaultStopSlack, DefaultLadderRungs)

	if len(got.Cars) != 1 || got.Cars[0].CardID != "CARD-19" || got.Cars[0].HolderKey != "27VA19" {
		t.Fatalf("only CARD-19 is the VA19 car card, got %+v", got.Cars)
	}
	if len(got.Offices) != 2 {
		t.Fatalf("Rental 1 and Rental 2 must be office buckets, got %+v", got.Offices)
	}
	byOffice := map[string]string{}
	for _, c := range got.Offices {
		byOffice[c.CardID] = c.HolderKey
	}
	if byOffice["CARD-RENT-1"] != "Rental 1" {
		t.Fatalf("Rental 1 holder %+v", got.Offices)
	}
	if byOffice["CARD-RENT-2"] != "VA RENTAL 2" {
		t.Fatalf("Rental 2 holder %+v", got.Offices)
	}

	va19Eras := 0
	for _, e := range got.Eras {
		if eraHolderType(e) == HolderCar && e.EFleetsID == "27VA19" {
			va19Eras++
			if e.CardID != "CARD-19" {
				t.Fatalf("rental/MIX dump onto VA19: %+v", e)
			}
			if e.SwitchedAt != nil || e.NextPairAt != nil {
				t.Fatalf("current VA19 era must not switch: %+v", e)
			}
			if e.PairStarted.IsZero() {
				t.Fatalf("VA19 control era needs pair_started: %+v", e)
			}
		}
		if (e.CardID == "CARD-RENT-1" || e.CardID == "CARD-RENT-2") && eraHolderType(e) == HolderCar {
			t.Fatalf("rental card must not persist a car era: %+v", e)
		}
	}
	if va19Eras != 1 {
		t.Fatalf("want exactly one VA19 car era (CARD-19), got %+v", got.Eras)
	}

	if got.Coverage.CardEraN != 1 || got.Coverage.KnownN != 1 {
		t.Fatalf("VA19 known from CARD-19 only: %+v", got.Coverage)
	}
	if got.Coverage.DeviceLinkedN != 1 {
		t.Fatalf("rental display_name must not count as VA19 device link: %+v", got.Coverage)
	}
	ids := FactoryIDsForLinkedCar(devs, "27VA19")
	if len(ids) != 1 || ids[0] != "3271045657" {
		t.Fatalf("watch seed is VA19's box only, got %v", ids)
	}
}

func TestRentalGPSDoesNotConfuseCardMix99Split(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	v15, t15 := exclusiveSits("27VA15", "CARD-MIX-99", 3, day, 37.54, -77.43)
	v19, t19 := exclusiveSits("27VA19", "CARD-MIX-99", 3, day.Add(10*24*time.Hour), 38.85, -77.05)
	_, tR := exclusiveSits("27VA19", "CARD-RENT-1", 3, day, 38.85, -77.05)
	for i := range tR {
		tR[i].RecordedEFleetsID = "Unknown"
		tR[i].RecordedCVN = "Rental 1"
	}
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	got := ClimbStationLadder(append(v15, v19...), append(append(t15, t19...), tR...), fleet, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Cars) != 1 || !got.Cars[0].Split || got.Cars[0].CardID != "CARD-MIX-99" {
		t.Fatalf("MIX split must survive rental labels: %+v", got.Cars)
	}
	var mix19 *model.CardEra
	for i := range got.Eras {
		e := &got.Eras[i]
		if e.CardID == "CARD-MIX-99" && e.EFleetsID == "27VA19" && eraHolderType(*e) == HolderCar {
			mix19 = e
		}
		if e.CardID == "CARD-RENT-1" && eraHolderType(*e) == HolderCar {
			t.Fatalf("rental must not share MIX's VA19 era: %+v", e)
		}
	}
	if mix19 == nil {
		t.Fatalf("MIX still gets a GPS VA19 era: %+v", got.Eras)
	}
	if mix19.SwitchedAt != nil {
		t.Fatalf("current MIX VA19 era must not switch: %+v", mix19)
	}
}

func TestScorePairingsRentalDoesNotVoteVA19(t *testing.T) {
	txs := []model.CardTx{
		{
			CardID: "CARD-RENT-2", At: ny(2026, 8, 20, 10),
			StationName: "WAWA", StationAddress: "1 MAIN, TOWN, VA",
			RecordedEFleetsID: "Unknown", RecordedCVN: "VA RENTAL 2",
			CalledEFleetsID: "27VA19",
		},
		{
			CardID: "CARD-19", At: ny(2026, 8, 23, 9),
			StationName: "MARATHON", StationAddress: "2 MAIN ST, TOWN, VA",
			RecordedEFleetsID: "27VA19", RecordedCVN: "VA19",
		},
	}
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	for _, p := range ps {
		if p.CardID == "CARD-RENT-2" && p.EntityType == "car" {
			t.Fatalf("rental must not vote a car even if GPS called VA19: %+v", ps)
		}
	}
	found := false
	for _, p := range ps {
		if p.CardID == "CARD-RENT-2" && p.EntityType == "office" && p.EntityKey == "VA RENTAL 2" && p.Best {
			found = true
		}
	}
	if !found {
		t.Fatalf("want rental office BEST, got %+v", ps)
	}
}

func TestHuntNearbySkipsRentalCardsAndBoxes(t *testing.T) {
	day := ny(2026, 6, 16, 14)
	lat, lng := 37.54, -77.43
	car := "27VA19"
	txs := []model.CardTx{{
		CardID: "CARD-RENT-1", At: day,
		StationName: "WAWA", StationAddress: "1 MAIN, TOWN, VA",
		RecordedEFleetsID: "Unknown", RecordedCVN: "Rental 1",
	}}
	visits := []model.StopVisit{{
		FactoryID: "351358810724200", DeviceID: "DRENT1", HasPos: true, Lat: lat, Lng: lng,
		From: day.Add(-10 * time.Minute), To: day.Add(10 * time.Minute),
	}, {
		FactoryID: "3271045657", DeviceID: "D19", EFleetsID: car, HasPos: true, Lat: lat, Lng: lng,
		From: day.Add(-10 * time.Minute), To: day.Add(10 * time.Minute),
	}}
	stations := []GeocodedStation{{
		Name: "WAWA", Address: "1 MAIN, TOWN, VA", Lat: lat, Lng: lng, Hits: 1,
	}}
	devs := []model.OneStepDevice{
		{FactoryID: "351358810724200", DeviceID: "DRENT1", DisplayName: "Rental 1", Active: true},
		{FactoryID: "3271045657", DeviceID: "D19", DisplayName: "VA-19: Lindsey", LinkedCarEFleetsID: &car, Active: true},
	}
	res := HuntNearby(visits, txs, stations, devs, DefaultStopSlack)
	if res.Watch != 0 || res.Certain != 0 || len(res.Cards) != 0 {
		t.Fatalf("rental cards must not hunt a VA19 join: %+v", res)
	}
}
