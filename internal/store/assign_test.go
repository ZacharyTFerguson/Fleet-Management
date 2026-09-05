package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestAssignTxOneFillAndAudit(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "a.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27VA15", Nickname: "VA15", PDIID: "PDI-0003"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27VA19", Nickname: "VA19", PDIID: "PDI-0020"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	odo := 1000
	tx := model.CardTx{CardID: "CARD-MIX-99", At: at, RecordedEFleetsID: "27VA15", Odometer: &odo}
	if err := s.UpsertCardTx(ctx, tx); err != nil {
		t.Fatal(err)
	}
	got, err := s.AssignTx(ctx, tx.Key(), "27VA19", "PDI-0020", "owner", "manual_drag")
	if err != nil {
		t.Fatal(err)
	}
	if got.AssignedEFleetsID != "27VA19" || got.AssignedPDIID != "PDI-0020" || got.Source != "owner" {
		t.Fatalf("%+v", got)
	}
	ev, err := s.ListAssignmentEvents(ctx, tx.Key())
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 1 || ev[0].FromEFleetsID != "" || ev[0].ToEFleetsID != "27VA19" || ev[0].ToPDIID != "PDI-0020" {
		t.Fatalf("events %+v", ev)
	}
	if _, err := s.AssignTx(ctx, tx.Key(), "", "", "owner", "undo"); err != nil {
		t.Fatal(err)
	}
	cur, err := s.GetAssignment(ctx, tx.Key())
	if err != nil {
		t.Fatal(err)
	}
	if cur.AssignedEFleetsID != "" || cur.Source != "" {
		t.Fatalf("undo %+v", cur)
	}
	ev, _ = s.ListAssignmentEvents(ctx, tx.Key())
	if len(ev) != 2 || ev[1].Reason != "undo" {
		t.Fatalf("audit must keep undo %+v", ev)
	}
}

func TestRefreshGPSCallsDoesNotMoveOwner(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "g.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	tx := model.CardTx{CardID: "C1", At: at, RecordedEFleetsID: "27VA15"}
	if _, err := s.AssignTx(ctx, tx.Key(), "27VA19", "PDI-0020", "owner", "manual_drag"); err != nil {
		t.Fatal(err)
	}
	tx.CalledEFleetsID = "27VA15"
	if err := s.RefreshGPSCalls(ctx, []model.CardTx{tx}); err != nil {
		t.Fatal(err)
	}
	cur, err := s.GetAssignment(ctx, tx.Key())
	if err != nil {
		t.Fatal(err)
	}
	if cur.AssignedEFleetsID != "27VA19" {
		t.Fatalf("owner must win %+v", cur)
	}
	if cur.GPSCalledEFleetsID != "27VA15" || !cur.GPSDisagrees {
		t.Fatalf("want GPS flag %+v", cur)
	}
}
