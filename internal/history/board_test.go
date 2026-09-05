package history

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestBuildBoardFilesOneFillUnderAssignedCar(t *testing.T) {
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	cars := []model.Car{
		{PDIID: "PDI-0003", EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{PDIID: "PDI-0020", EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
		{PDIID: "PDI-0100", EFleetsID: "26CT1", Nickname: "CT1", Region: "CT"},
	}
	tx := model.CardTx{CardID: "CARD-MIX-99", At: at, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	assigns := []model.TxAssignment{{
		TxKey: tx.Key(), AssignedEFleetsID: "27VA19", AssignedPDIID: "PDI-0020", Source: "owner",
	}}
	got := BuildBoard(cars, []model.CardTx{tx}, assigns, "VA", at)
	if got.Region != "VA" || len(got.Regions) != 2 {
		t.Fatalf("regions %+v", got)
	}
	if got.AssignedN != 1 || got.UnassignedN != 0 || len(got.Unassigned) != 0 {
		t.Fatalf("counts %+v", got)
	}
	var va19 CarColumn
	for _, c := range got.Cars {
		if c.EFleetsID == "27VA19" {
			va19 = c
		}
		if c.EFleetsID == "26CT1" {
			t.Fatal("CT car must not appear on VA turnstile")
		}
	}
	if len(va19.Fills) != 1 || va19.Fills[0].TxKey != tx.Key() || va19.Fills[0].AssignedPDIID != "PDI-0020" {
		t.Fatalf("VA19 fills %+v", va19)
	}
}

func TestBuildBoardUnassignedStaysInTray(t *testing.T) {
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	cars := []model.Car{{PDIID: "PDI-0003", EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}}
	tx := model.CardTx{CardID: "C1", At: at, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	got := BuildBoard(cars, []model.CardTx{tx}, nil, "VA", at)
	if got.UnassignedN != 1 || len(got.Unassigned) != 1 || got.Unassigned[0].CardID != "C1" {
		t.Fatalf("%+v", got)
	}
	if len(got.Cars) != 1 || len(got.Cars[0].Fills) != 0 {
		t.Fatalf("unassigned must not sit on a car %+v", got.Cars)
	}
}
