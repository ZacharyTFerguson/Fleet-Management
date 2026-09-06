package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/oil"
)

func TestLedgerDismissSurvivesUpsert(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "led.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	exp := 10100
	abs := 20
	row := oil.LedgerRow{
		EFleetsID: "27VA15", CardID: "C1", PunchAt: at, Merchant: "SHELL",
		Recorded: 10120, MaintOdo: 10000, MaintAt: at.Add(-24 * time.Hour),
		Expected: &exp, Overage: 20, AbsDiff: &abs, Trend: oil.TrendFlat,
		Status: oil.LedgerTrusted, InTrend: true,
	}
	if err := s.UpsertLedgerRow(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLedgerStatus(ctx, "27VA15", at.Format(time.RFC3339), 10120, oil.LedgerDismissed, "operator dismissed"); err != nil {
		t.Fatal(err)
	}
	row.Status = oil.LedgerTrusted
	row.InTrend = true
	if err := s.UpsertLedgerRow(ctx, row); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListLedger(ctx, "27VA15")
	if err != nil || len(got) != 1 {
		t.Fatalf("%d %v", len(got), err)
	}
	if got[0].Status != oil.LedgerDismissed || got[0].InTrend {
		t.Fatalf("dismiss must stick: %+v", got[0])
	}
	if err := s.SaveDriveStopWindow(ctx, "FACT1", at.Add(-time.Hour), at, 12.5); err != nil {
		t.Fatal(err)
	}
	m, ok, err := s.GetDriveStopWindow(ctx, "FACT1", at.Add(-time.Hour), at)
	if err != nil || !ok || m != 12.5 {
		t.Fatalf("window %v %v %v", m, ok, err)
	}
}

func TestListLedgerNewestFirst(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "led-order.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	older := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	for _, row := range []oil.LedgerRow{
		{EFleetsID: "27VA15", PunchAt: older, Recorded: 10000, Status: oil.LedgerHold},
		{EFleetsID: "27VA15", PunchAt: newer, Recorded: 10100, Status: oil.LedgerHold},
	} {
		if err := s.UpsertLedgerRow(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListLedger(ctx, "27VA15")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].PunchAt.Equal(newer) {
		t.Fatalf("newest-first %+v", rows)
	}
}
