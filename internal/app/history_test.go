package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/store"
)

func TestAssignFillOneTransactionAndUndo(t *testing.T) {
	st, err := store.Open("sqlite", filepath.Join(t.TempDir(), "h.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{PDIID: "PDI-0003", EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{PDIID: "PDI-0020", EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	tx := model.CardTx{CardID: "CARD-MIX-99", At: at, StationName: "Shell", RecordedEFleetsID: "27VA15"}
	if err := st.UpsertCardTx(ctx, tx); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st}
	board, err := a.AssignFill(ctx, tx.Key(), "27VA19", "manual_drag", "VA")
	if err != nil {
		t.Fatal(err)
	}
	if board.AssignedN != 1 || board.UnassignedN != 0 {
		t.Fatalf("after drag %+v", board)
	}
	found := false
	for _, c := range board.Cars {
		if c.EFleetsID == "27VA19" {
			for _, f := range c.Fills {
				if f.TxKey == tx.Key() && f.AssignedPDIID != "" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("fill not under VA19 %+v", board.Cars)
	}
	board, err = a.AssignFill(ctx, tx.Key(), "", "undo", "VA")
	if err != nil {
		t.Fatal(err)
	}
	if board.AssignedN != 0 || board.UnassignedN != 1 {
		t.Fatalf("undo %+v", board)
	}
	ev, err := st.ListAssignmentEvents(ctx, tx.Key())
	if err != nil || len(ev) != 2 {
		t.Fatalf("audit %v %+v", err, ev)
	}
}

func TestRefreshGPSCallsKeepsOwnerHome(t *testing.T) {
	st, err := store.Open("sqlite", filepath.Join(t.TempDir(), "r.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_ = st.UpsertCar(ctx, model.Car{PDIID: "PDI-0003", EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"})
	_ = st.UpsertCar(ctx, model.Car{PDIID: "PDI-0020", EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"})
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	tx := model.CardTx{CardID: "C1", At: at, RecordedEFleetsID: "27VA15"}
	_ = st.UpsertCardTx(ctx, tx)
	a := &App{Store: st}
	if _, err := a.AssignFill(ctx, tx.Key(), "27VA19", "manual_drag", "VA"); err != nil {
		t.Fatal(err)
	}
	tx.CalledEFleetsID = "27VA15"
	if err := st.RefreshGPSCalls(ctx, []model.CardTx{tx}); err != nil {
		t.Fatal(err)
	}
	board, err := a.HistoryBoard(ctx, "VA")
	if err != nil {
		t.Fatal(err)
	}
	if board.AssignedN != 1 || board.GPSFlagN != 1 {
		t.Fatalf("owner stays, GPS flags %+v", board)
	}
}
