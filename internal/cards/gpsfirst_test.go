package cards

import (
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
