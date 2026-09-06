package app

import (
	"context"
	"path/filepath"
	"testing"

	"oilchange/internal/config"
	"oilchange/internal/store"
)

// TestLiveSep6IngestLandsInSQLite walks the operator dump path with fixtures
// derived from the real Sep 2026 export headers: Fleet Summary + Fuel DETAILS
// + Maintenance Detail -> sqlite. A full 90d/12mo dump uses the same flags:
//
//	oilchange sync-enterprise --vehicles fleetsummary.csv \
//	    --fuel-details details.csv --shop-ro maintenance.csv
//
// Overlapping dumps are the norm (a 90d pull repeats most of last month's
// punches), so the second half of this test locks re-ingest idempotency —
// including punches with a blank Provider Odometer (EV charging), which the
// UNIQUE(efleets_id, provider_transaction_time, odometer) key alone cannot
// dedupe because NULLs never conflict.
func TestLiveSep6IngestLandsInSQLite(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "live-sep6.sqlite")
	st, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Cfg: config.Config{SQLitePath: dbPath}, Store: st}

	sync := func() {
		t.Helper()
		if err := a.SyncEnterprise(ctx,
			testdata("enterprise", "live_sep6", "fleetsummary.csv"),
			testdata("enterprise", "live_sep6", "details.csv"),
			testdata("enterprise", "live_sep6", "maintenance.csv"),
			"",
		); err != nil {
			t.Fatal(err)
		}
	}
	sync()

	cars, err := st.ListCars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cars) != 3 {
		t.Fatalf("roster cars=%d want 3 (27ZZZZ is not on Fleet Summary): %+v", len(cars), cars)
	}
	for _, c := range cars {
		if c.LastReadingMiles != nil {
			t.Fatalf("Enterprise ingest must never invent Last Reading: %s %+v", c.EFleetsID, c.LastReadingMiles)
		}
	}

	assertFills := func() {
		t.Helper()
		fillsA, err := st.ListFills(ctx, "27SEPA")
		if err != nil {
			t.Fatal(err)
		}
		if len(fillsA) != 2 {
			t.Fatalf("27SEPA fills=%d want 2", len(fillsA))
		}
		fillsB, err := st.ListFills(ctx, "27SEPB")
		if err != nil {
			t.Fatal(err)
		}
		if len(fillsB) != 3 {
			t.Fatalf("27SEPB fills=%d want 3 (one EV punch with no odometer)", len(fillsB))
		}
		noOdo := 0
		for _, f := range fillsB {
			if f.Odometer == nil {
				noOdo++
			}
		}
		if noOdo != 1 {
			t.Fatalf("27SEPB punches without odometer=%d want exactly 1", noOdo)
		}
		fillsZ, err := st.ListFills(ctx, "27ZZZZ")
		if err != nil {
			t.Fatal(err)
		}
		if len(fillsZ) != 0 {
			t.Fatalf("off-roster 27ZZZZ punches must be skipped, got %d", len(fillsZ))
		}
		txs, err := st.ListCardTxs(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(txs) != 5 {
			t.Fatalf("card_transactions=%d want 5 (roster punches only)", len(txs))
		}
	}
	assertFills()

	rosA, err := st.ListShopROs(ctx, "27SEPA")
	if err != nil {
		t.Fatal(err)
	}
	if len(rosA) != 1 {
		t.Fatalf("27SEPA shop ROs=%d want 1 (line items collapse to one RO)", len(rosA))
	}
	carA, err := st.CarByEFleets(ctx, "27SEPA")
	if err != nil {
		t.Fatal(err)
	}
	if carA.LastOilMiles == nil || *carA.LastOilMiles != 119800 {
		t.Fatalf("27SEPA last oil must seed from the lube RO: %+v", carA.LastOilMiles)
	}
	carB, err := st.CarByEFleets(ctx, "27SEPB")
	if err != nil {
		t.Fatal(err)
	}
	if carB.LastOilMiles != nil {
		t.Fatalf("brake RO must not seed last oil: %+v", *carB.LastOilMiles)
	}
	carC, err := st.CarByEFleets(ctx, "26SEPC")
	if err != nil {
		t.Fatal(err)
	}
	if carC.LastOilMiles == nil || *carC.LastOilMiles != 65000 {
		t.Fatalf("26SEPC last oil must seed from the oil & filter RO: %+v", carC.LastOilMiles)
	}

	// Overlapping dumps: same files twice more must not add a single row.
	sync()
	sync()
	assertFills()

	rosA, err = st.ListShopROs(ctx, "27SEPA")
	if err != nil {
		t.Fatal(err)
	}
	if len(rosA) != 1 {
		t.Fatalf("re-ingest duplicated shop ROs: %d", len(rosA))
	}
	carA, err = st.CarByEFleets(ctx, "27SEPA")
	if err != nil {
		t.Fatal(err)
	}
	if carA.LastOilMiles == nil || *carA.LastOilMiles != 119800 {
		t.Fatalf("re-ingest moved last oil: %+v", carA.LastOilMiles)
	}
}
