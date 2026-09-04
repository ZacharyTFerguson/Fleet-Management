package cards

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestMatchByStopTimesPicksCardWhileCarStopped(t *testing.T) {
	car := "26LSZW"
	visits := []model.StopVisit{{
		EFleetsID: car,
		From:      ny(2026, 8, 30, 10),
		To:        ny(2026, 8, 30, 10).Add(15 * time.Minute),
	}}
	txs := []model.CardTx{
		{CardID: "CARD-BING", At: ny(2026, 8, 30, 10).Add(8 * time.Minute), StationName: "VALVOLINE", RecordedEFleetsID: "OTHER"},
		{CardID: "CARD-NOISE", At: ny(2026, 8, 29, 10), StationName: "SHELL", RecordedEFleetsID: car},
	}
	got := MatchByStopTimes(visits, txs, DefaultStopSlack)
	if len(got) != 1 || !got[0].Best || got[0].CardID != "CARD-BING" || got[0].EFleetsID != car {
		t.Fatalf("got %+v", got)
	}
	if got[0].EvidenceN != 1 {
		t.Fatalf("n %d", got[0].EvidenceN)
	}
	if len(got[0].EnterpriseCars) != 1 || got[0].EnterpriseCars[0] != "OTHER" {
		t.Fatalf("enterprise column is evidence only: %+v", got[0].EnterpriseCars)
	}
}

func TestMatchByStopTimesIgnoresOvernightSit(t *testing.T) {
	car := "26LSZW"
	at := ny(2026, 8, 30, 22)
	visits := []model.StopVisit{{
		EFleetsID: car,
		From:      at,
		To:        at.Add(10 * time.Hour),
	}}
	txs := []model.CardTx{{CardID: "CARD-BING", At: at.Add(time.Hour), StationName: "HOME"}}
	if got := MatchByStopTimes(visits, txs, DefaultStopSlack); len(got) != 0 {
		t.Fatalf("overnight sit is not a pump marker: %+v", got)
	}
}

func TestMatchByStopTimesSkipsTwoCarsStopped(t *testing.T) {
	at := ny(2026, 8, 30, 10)
	visits := []model.StopVisit{
		{EFleetsID: "CAR-A", From: at, To: at.Add(10 * time.Minute)},
		{EFleetsID: "CAR-B", From: at, To: at.Add(10 * time.Minute)},
	}
	txs := []model.CardTx{{CardID: "CARD-X", At: at.Add(2 * time.Minute), StationName: "WAWA"}}
	if got := MatchByStopTimes(visits, txs, DefaultStopSlack); len(got) != 0 {
		t.Fatalf("ambiguous stop time must not vote: %+v", got)
	}
}

func TestParseDriveStopVisitsStopsOnly(t *testing.T) {
	// parse lives in onestep; matching must ignore drive segments via visit list.
	visits := []model.StopVisit{
		{EFleetsID: "CAR1", From: ny(2026, 8, 30, 10), To: ny(2026, 8, 30, 10).Add(time.Minute)},
	}
	txs := []model.CardTx{{CardID: "C", At: ny(2026, 8, 30, 12), StationName: "EXXON"}}
	if got := MatchByStopTimes(visits, txs, time.Minute); len(got) != 0 {
		t.Fatalf("swipe far from stop: %+v", got)
	}
}
