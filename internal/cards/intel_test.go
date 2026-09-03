package cards

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

func ny(y int, mo time.Month, d, h int) time.Time {
	return time.Date(y, mo, d, h, 0, 0, 0, time.UTC)
}

// syntheticWrongCard is the locked testdata story: CARD-MIX-99 lives on 27VA15
// at SHELL, then one swipe is recorded on 27VA19 the same day 27VA15 is also at SHELL.
func syntheticWrongCard() []model.CardTx {
	shell := "SHELL"
	addr := "1 MAIN ST, TOWN, VA"
	gal := 10.0
	return []model.CardTx{
		{CardID: "CARD-MIX-99", At: ny(2026, 8, 20, 10), StationName: shell, StationAddress: addr, RecordedEFleetsID: "27VA15", RecordedCVN: "VA15", DriverFirst: "PAT", DriverLast: "TECH", Gallons: &gal},
		{CardID: "CARD-MIX-99", At: ny(2026, 8, 22, 10), StationName: shell, StationAddress: addr, RecordedEFleetsID: "27VA15", RecordedCVN: "VA15", DriverFirst: "PAT", DriverLast: "TECH", Gallons: &gal},
		{CardID: "CARD-MIX-99", At: ny(2026, 8, 24, 10), StationName: shell, StationAddress: addr, RecordedEFleetsID: "27VA15", RecordedCVN: "VA15", DriverFirst: "PAT", DriverLast: "TECH", Gallons: &gal},
		// Weird + latest: Enterprise wrote the swipe on 27VA19.
		{CardID: "CARD-MIX-99", At: ny(2026, 8, 25, 11), StationName: shell, StationAddress: addr, RecordedEFleetsID: "27VA19", RecordedCVN: "VA19", DriverFirst: "PAT", DriverLast: "TECH", Gallons: &gal},
		// Same station/day: the usual car is also at SHELL (true driver).
		{CardID: "CARD-15", At: ny(2026, 8, 25, 10), StationName: shell, StationAddress: addr, RecordedEFleetsID: "27VA15", RecordedCVN: "VA15", DriverFirst: "PAT", DriverLast: "TECH", Gallons: &gal},
		{CardID: "CARD-19", At: ny(2026, 8, 23, 9), StationName: "MARATHON", StationAddress: "2 MAIN ST, TOWN, VA", RecordedEFleetsID: "27VA19", RecordedCVN: "VA19", DriverFirst: "OTHER", DriverLast: "TECH"},
	}
}

func TestBestPairingPrefersMajorityCar(t *testing.T) {
	txs := syntheticWrongCard()
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	var best string
	for _, p := range ps {
		if p.CardID == "CARD-MIX-99" && p.EntityType == "car" && p.Best {
			best = p.EntityKey
		}
	}
	if best != "27VA15" {
		t.Fatalf("best car %q want 27VA15 (majority at SHELL)", best)
	}
}

func TestSuspectWhenEnterpriseCarDiffers(t *testing.T) {
	txs := syntheticWrongCard()
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	s := FindSuspects(txs, ps)
	if len(s) != 1 || s[0].CardID != "CARD-MIX-99" {
		t.Fatalf("suspects %+v", s)
	}
	if s[0].BestCar != "27VA15" || s[0].EnterpriseCar != "27VA19" {
		t.Fatalf("want latest 27VA19 vs best 27VA15, got %+v", s[0])
	}
}

func TestTraceFindsOtherCarAtSameStationNearbyDay(t *testing.T) {
	txs := syntheticWrongCard()
	hits := TraceStationDays(txs, "CARD-MIX-99", 2)
	found := false
	for _, h := range hits {
		if h.OtherEFleetsID == "27VA15" && h.OtherCardID == "CARD-15" && h.DaysApart <= 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 27VA15 at SHELL near the weird swipe, hits=%+v", hits)
	}
}

func TestTxFromFillRequiresCardID(t *testing.T) {
	if _, ok := TxFromFill(model.Fill{EFleetsID: "X"}); ok {
		t.Fatal("empty card must not vote")
	}
}
