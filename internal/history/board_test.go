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

func TestBuildBoardFillsNewestFirst(t *testing.T) {
	older := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	same := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	cars := []model.Car{{PDIID: "PDI-0003", EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}}
	txOlder := model.CardTx{CardID: "C-OLD", At: older, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	txNewer := model.CardTx{CardID: "C-NEW", At: newer, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	txA := model.CardTx{CardID: "C-A", At: same, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	txB := model.CardTx{CardID: "C-B", At: same, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	assigns := []model.TxAssignment{
		{TxKey: txOlder.Key(), AssignedEFleetsID: "27VA15", AssignedPDIID: "PDI-0003", Source: "owner"},
		{TxKey: txNewer.Key(), AssignedEFleetsID: "27VA15", AssignedPDIID: "PDI-0003", Source: "owner"},
		{TxKey: txA.Key(), AssignedEFleetsID: "27VA15", AssignedPDIID: "PDI-0003", Source: "owner"},
		{TxKey: txB.Key(), AssignedEFleetsID: "27VA15", AssignedPDIID: "PDI-0003", Source: "owner"},
	}
	got := BuildBoard(cars, []model.CardTx{txOlder, txNewer, txA, txB}, assigns, "VA", newer)
	if len(got.Cars) != 1 || len(got.Cars[0].Fills) != 4 {
		t.Fatalf("fills %+v", got.Cars)
	}
	fills := got.Cars[0].Fills
	if fills[0].CardID != "C-A" || fills[1].CardID != "C-B" || fills[2].CardID != "C-NEW" || fills[3].CardID != "C-OLD" {
		t.Fatalf("newest-first stable order %+v", fills)
	}
}

func TestBuildBoardUnassignedNewestFirst(t *testing.T) {
	older := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	cars := []model.Car{{PDIID: "PDI-0003", EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}}
	txOlder := model.CardTx{CardID: "C-OLD", At: older, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	txNewer := model.CardTx{CardID: "C-NEW", At: newer, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	got := BuildBoard(cars, []model.CardTx{txOlder, txNewer}, nil, "VA", newer)
	if len(got.Unassigned) != 2 || got.Unassigned[0].CardID != "C-NEW" || got.Unassigned[1].CardID != "C-OLD" {
		t.Fatalf("unassigned newest-first %+v", got.Unassigned)
	}
}

// TestBuildBoardOrdersNewestFirstWithStableTies locks the one display order:
// Provider Transaction Date+Time descending everywhere on the History page,
// with deterministic ties for same-second punches. Before this rule, car
// columns read oldest-first while the tray read newest-first.
func TestBuildBoardOrdersNewestFirstWithStableTies(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	cars := []model.Car{{PDIID: "PDI-0001", EFleetsID: "27SEPB", Nickname: "VA22", Region: "VA"}}
	odoHi, odoLo := 90500, 90400
	txs := []model.CardTx{
		{CardID: "x22020", At: base.Add(-48 * time.Hour), StationName: "OLD", RecordedEFleetsID: "27SEPB", Odometer: &odoLo},
		{CardID: "x22020", At: base, StationName: "MARATHON", RecordedEFleetsID: "27SEPB", Odometer: &odoHi},
		{CardID: "x22020", At: base, StationName: "EVGO", RecordedEFleetsID: "27SEPB"}, // same second, no odometer
		{CardID: "x22020", At: base.Add(48 * time.Hour), StationName: "NEW", RecordedEFleetsID: "27SEPB", Odometer: &odoHi},
	}
	var assigns []model.TxAssignment
	for _, tx := range txs {
		assigns = append(assigns, model.TxAssignment{
			TxKey: tx.Key(), AssignedEFleetsID: "27SEPB", AssignedPDIID: "PDI-0001", Source: "owner",
		})
	}
	wantStations := []string{"NEW", "MARATHON", "EVGO", "OLD"}

	check := func(order []model.CardTx) {
		t.Helper()
		got := BuildBoard(cars, order, assigns, "VA", base)
		if len(got.Cars) != 1 || len(got.Cars[0].Fills) != len(wantStations) {
			t.Fatalf("board %+v", got)
		}
		for i, want := range wantStations {
			if s := got.Cars[0].Fills[i].Station; s != want {
				t.Fatalf("column position %d = %s want %s (newest first, higher odometer wins the tie)", i, s, want)
			}
		}
	}
	check(txs)
	// Input order must not matter: reversed input yields the identical board.
	rev := make([]model.CardTx, 0, len(txs))
	for i := len(txs) - 1; i >= 0; i-- {
		rev = append(rev, txs[i])
	}
	check(rev)

	// The tray follows the same rule as the columns.
	tray := BuildBoard(cars, txs, nil, "VA", base)
	if len(tray.Unassigned) != len(wantStations) {
		t.Fatalf("tray %+v", tray.Unassigned)
	}
	for i, want := range wantStations {
		if s := tray.Unassigned[i].Station; s != want {
			t.Fatalf("tray position %d = %s want %s", i, s, want)
		}
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
