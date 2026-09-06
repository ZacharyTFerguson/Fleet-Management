package app

import (
	"context"
	"path/filepath"
	"testing"

	"oilchange/internal/config"
	"oilchange/internal/store"
)

// TestSyncEnterpriseFullDumpIngest documents the operator path:
//   oilchange sync-enterprise --vehicles fleetsummary.csv --fuel-details details.csv --shop-ro maintenance.csv
// Maintenance may arrive before Fleet Summary; a later roster import must reconcile last oil.
func TestSyncEnterpriseFullDumpIngest(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "enterprise-full.sqlite")
	st, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Cfg: config.Config{SQLitePath: dbPath}, Store: st}

	// Maintenance-first drop (common operator habit).
	if err := a.SyncEnterprise(ctx, "", "", testdata("enterprise", "maintenance.csv"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CarByEFleets(ctx, "27TESTA"); err == nil {
		t.Fatal("maintenance-only ingest must not create roster cars")
	}

	if err := a.SyncEnterprise(ctx,
		testdata("enterprise", "fleetsummary.csv"),
		testdata("enterprise", "details.csv"),
		"",
		"",
	); err != nil {
		t.Fatal(err)
	}

	cars, err := st.ListCars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cars) != 2 {
		t.Fatalf("roster cars %d want 2 demo units", len(cars))
	}
	car, err := st.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastOilMiles == nil || *car.LastOilMiles != 100500 {
		t.Fatalf("maintenance-first must reconcile last oil on roster import: %+v", car.LastOilMiles)
	}
	if car.LastReadingMiles != nil {
		t.Fatalf("sync-enterprise must not invent Last Reading: %+v", car.LastReadingMiles)
	}

	fills, err := st.ListFills(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 2 {
		t.Fatalf("DETAILS fills for 27TESTA %d want 2", len(fills))
	}

	txs, err := st.ListCardTxs(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 3 {
		t.Fatalf("card_transactions %d want 3 (2 for TESTA + 1 TESTB)", len(txs))
	}
	newest := txs[0]
	if newest.RecordedEFleetsID != "27TESTA" || newest.At.Day() != 2 {
		t.Fatalf("ListCardTxs must be newest-first: %+v", txs)
	}
}
