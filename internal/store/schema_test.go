package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/oil"
	"oilchange/internal/places"
)

func TestCanonicalSchemaHasRemoteParity(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "schema.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	for _, name := range []string{
		"cars", "cards", "fills", "shop_ros", "oil_changes", "hold_events",
		"onestep_devices", "drive_stop_miles", "card_transactions", "card_pairings",
		"card_eras", "transaction_assignments", "assignment_events",
		"places", "place_types", "place_brands", "toptier_codes", "toptier_grades",
		"gas_station_marker_jobs", "mileage_ledger", "drive_stop_windows",
		"desk_users", "vault_secrets", "desk_heartbeats",
	} {
		var n int
		if err := s.queryRow(ctx, "SELECT COUNT(*) FROM "+name).Scan(&n); err != nil {
			t.Fatalf("missing table %s: %v", name, err)
		}
	}

	var brands, types int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM place_brands WHERE code='UNKWN'`).Scan(&brands); err != nil || brands != 1 {
		t.Fatalf("place_brands seed UNKWN: %d %v", brands, err)
	}
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM place_types WHERE code=?`, places.TypeGas).Scan(&types); err != nil || types != 1 {
		t.Fatalf("place_types seed 001: %d %v", types, err)
	}

	var idx int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='fills_one_null_odo_per_second'`).Scan(&idx); err != nil || idx != 1 {
		t.Fatalf("fills_one_null_odo_per_second: %d %v", idx, err)
	}

	if _, err := s.exec(ctx, `INSERT INTO mileage_ledger (
		efleets_id, punch_at, recorded_odo, difference, overage, shortage, status, in_trend, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"27VA15", time.Now().UTC().Format(time.RFC3339), 100, 5, 5, 0, oil.LedgerHold, 0,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("mileage_ledger.difference: %v", err)
	}

	if _, err := s.exec(ctx, `INSERT INTO places (general_code, type_code, brand_code, toptier, toptier_grade, label, city, hold_reason, source)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		"A000001", places.TypeGas, places.BrandUnknown, "A", "A", "A000001_001_UNKWN_A_A", "Richmond", "", "seed"); err != nil {
		t.Fatalf("places extra columns: %v", err)
	}
}
