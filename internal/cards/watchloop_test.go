package cards

import (
	"testing"
	"time"

	"oilchange/internal/enterprise"
	"oilchange/internal/model"
)

func TestWatchFillBatchNewestTen(t *testing.T) {
	ny := enterprise.NY()
	var txs []model.CardTx
	for i := 0; i < 12; i++ {
		txs = append(txs, model.CardTx{
			CardID: "C1",
			At:     time.Date(2026, 8, 1+i, 10, 0, 0, 0, ny).UTC(),
		})
	}
	got := WatchFillBatch(txs, 10)
	if len(got) != 10 {
		t.Fatalf("len %d", len(got))
	}
	if !got[0].At.Equal(time.Date(2026, 8, 12, 10, 0, 0, 0, ny).UTC()) {
		t.Fatalf("newest first: %s", got[0].At)
	}
	if !got[9].At.Equal(time.Date(2026, 8, 3, 10, 0, 0, 0, ny).UTC()) {
		t.Fatalf("10th newest: %s", got[9].At)
	}
}

func TestIsVirginiaVehicleNCXAndVA15(t *testing.T) {
	cars := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "26LSZW", Nickname: "Bing-2", Region: "NY"},
		{EFleetsID: "292NCX", Nickname: "292NCX"},
	}
	if !IsVirginiaVehicle("292NCX", "", cars) {
		t.Fatal("292NCX")
	}
	if !IsVirginiaVehicle("27VA15", "VA15", cars) {
		t.Fatal("27VA15")
	}
	if IsVirginiaVehicle("26LSZW", "", cars) {
		t.Fatal("Bing-2 is not VA")
	}
	if !IsVirginiaVehicle("27VA19", "", nil) {
		t.Fatal("27VA19 by id shape")
	}
}

func TestSeedWatchedFactoryIDsVirginiaThenHits(t *testing.T) {
	car := "292NCX"
	devs := []model.OneStepDevice{{
		FactoryID: "7000335987", DeviceID: "DEV-VA", Active: true, LinkedCarEFleetsID: &car,
	}}
	fills := []model.CardTx{{
		CardID: "xxxxxxxxxxxxx57770", At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
		RecordedEFleetsID: car,
	}}
	got := SeedWatchedFactoryIDs(fills, NearbyResult{}, devs, []model.Car{{EFleetsID: car, Nickname: "292NCX"}})
	if len(got) != 1 || got[0] != "7000335987" {
		t.Fatalf("VA seed: %v", got)
	}

	prior := NearbyResult{Cards: []NearbyCard{{
		CardID: "xxxxxxxxxxxxx57770",
		Watch:  []NearbyDevice{{FactoryID: "HIT-BOX", ExclusiveFills: 1}},
	}}}
	got = SeedWatchedFactoryIDs(fills, prior, devs, []model.Car{{EFleetsID: car, Nickname: "292NCX"}})
	if len(got) != 2 || got[0] != "7000335987" && got[1] != "7000335987" {
		t.Fatalf("hits+VA: %v", got)
	}
}

func TestSeedWatchedFactoryIDsHypothesisWhenEmpty(t *testing.T) {
	car := "26LSZW"
	devs := []model.OneStepDevice{{
		FactoryID: "FACT-NY", DeviceID: "DEV-NY", Active: true, LinkedCarEFleetsID: &car,
	}}
	fills := []model.CardTx{{
		CardID: "CARD-NY", At: time.Now().UTC(), RecordedEFleetsID: car,
	}}
	got := SeedWatchedFactoryIDs(fills, NearbyResult{}, devs, []model.Car{{EFleetsID: car, Nickname: "Bing-2", Region: "NY"}})
	if len(got) != 1 || got[0] != "FACT-NY" {
		t.Fatalf("hypothesis: %v", got)
	}
}

func TestSeedWatchedFactoryIDsSkipsTRACKERAndUnpaired(t *testing.T) {
	fills := []model.CardTx{{
		CardID: "CARD-T", At: time.Now().UTC(), RecordedEFleetsID: "TRACKER",
	}}
	devs := []model.OneStepDevice{{FactoryID: "LOOSE", DeviceID: "D", Active: true}}
	got := SeedWatchedFactoryIDs(fills, NearbyResult{}, devs, nil)
	if len(got) != 0 {
		t.Fatalf("must not invent unpaired: %v", got)
	}
}

func TestWatchCardOrderLikelyThenVirginia(t *testing.T) {
	ny := enterprise.NY()
	txs := []model.CardTx{
		{CardID: "CARD-NEW", At: time.Date(2026, 8, 30, 10, 0, 0, 0, ny), RecordedEFleetsID: "26LSZW"},
		{CardID: "CARD-VA", At: time.Date(2026, 8, 20, 10, 0, 0, 0, ny), RecordedEFleetsID: "292NCX"},
		{CardID: "CARD-LIKELY", At: time.Date(2026, 8, 10, 10, 0, 0, 0, ny), RecordedEFleetsID: "TRACKER"},
	}
	prior := NearbyResult{Cards: []NearbyCard{{
		CardID: "CARD-LIKELY",
		Likely: []NearbyDevice{{FactoryID: "F", ExclusiveFills: 2}},
	}}}
	got := WatchCardOrder(txs, prior, []model.Car{{EFleetsID: "292NCX", Nickname: "292NCX"}})
	if len(got) != 3 || got[0] != "CARD-LIKELY" || got[1] != "CARD-VA" || got[2] != "CARD-NEW" {
		t.Fatalf("%v", got)
	}
}

func TestWatchedCoverageCompleteRequiresEveryBox(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	visits := []model.StopVisit{{
		FactoryID: "A", From: from, To: to,
	}}
	if WatchedCoverageComplete(visits, []string{"A", "B"}, from, to) {
		t.Fatal("B missing")
	}
	if WatchedCoverageComplete(visits, nil, from, to) {
		t.Fatal("empty watch list is incomplete")
	}
	visits = append(visits, model.StopVisit{FactoryID: "B", From: from, To: to})
	if !WatchedCoverageComplete(visits, []string{"A", "B"}, from, to) {
		t.Fatal("both spanned")
	}
	short := []model.StopVisit{{
		FactoryID: "A", From: from.Add(time.Hour), To: from.Add(2 * time.Hour),
	}}
	if DeviceCoveredInWindow(short, "A", from, to) {
		t.Fatal("short stop is not a fetch")
	}
}

func TestSeedPriorityFactoryIDsNewestVirginiaOnly(t *testing.T) {
	vaNew, vaOld := "27VA15", "292NCX"
	devs := []model.OneStepDevice{
		{FactoryID: "FACT-NEW", DeviceID: "D-NEW", Active: true, LinkedCarEFleetsID: &vaNew},
		{FactoryID: "FACT-OLD", DeviceID: "D-OLD", Active: true, LinkedCarEFleetsID: &vaOld},
	}
	cars := []model.Car{
		{EFleetsID: vaNew, Nickname: "VA15", Region: "VA"},
		{EFleetsID: vaOld, Nickname: "292NCX"},
	}
	fills := []model.CardTx{
		{CardID: "C1", At: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC), RecordedEFleetsID: vaNew},
		{CardID: "C1", At: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC), RecordedEFleetsID: vaOld},
	}
	got := SeedPriorityFactoryIDs(fills, devs, cars)
	if len(got) != 1 || got[0] != "FACT-NEW" {
		t.Fatalf("newest VA only: %v", got)
	}
	watched := SeedWatchedFactoryIDs(fills, NearbyResult{}, devs, cars)
	if len(watched) != 1 || watched[0] != "FACT-NEW" {
		t.Fatalf("watched newest VA only: %v", watched)
	}
}

func TestPreserveUnladderedCarErasKeepsNearbyCertain(t *testing.T) {
	ladder := []model.CardEra{{
		CardID: "GPS-CARD", EFleetsID: "26LSZW", HolderType: HolderCar,
	}}
	existing := []model.CardEra{
		{CardID: "GPS-CARD", EFleetsID: "OLD", HolderType: HolderCar},
		{CardID: "NEAR-CARD", EFleetsID: "292NCX", HolderType: HolderCar, EvidenceN: 3},
		{CardID: "PERSON", HolderType: HolderPerson, HolderKey: "JANE"},
	}
	got := PreserveUnladderedCarEras(ladder, existing)
	if len(got) != 2 {
		t.Fatalf("len %d %+v", len(got), got)
	}
	if got[1].CardID != "NEAR-CARD" || got[1].EFleetsID != "292NCX" {
		t.Fatalf("%+v", got)
	}
}
