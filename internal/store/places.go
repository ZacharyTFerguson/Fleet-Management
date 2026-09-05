package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"oilchange/internal/places"
)

// ListPlaces returns the local Canon catalog cache. Empty if never copied.
func (s *Store) ListPlaces(ctx context.Context) ([]places.Place, error) {
	rows, err := s.query(ctx, `SELECT general_code, type_code, brand_code, toptier, toptier_grade,
		COALESCE(label,''), COALESCE(name,''), COALESCE(address,''), COALESCE(city,''), COALESCE(state,''),
		COALESCE(merchant_location,''), COALESCE(merchant_id,''), COALESCE(site_key,''), active
		FROM places ORDER BY general_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []places.Place
	for rows.Next() {
		var p places.Place
		var active int
		if err := rows.Scan(&p.GeneralCode, &p.TypeCode, &p.BrandCode, &p.TopTier, &p.TopTierGrade,
			&p.Label, &p.Name, &p.Address, &p.City, &p.State, &p.MerchantLocation, &p.MerchantID,
			&p.SiteKey, &active); err != nil {
			return nil, err
		}
		p.Active = active != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountPlaces is the cached catalog size.
func (s *Store) CountPlaces(ctx context.Context) (int, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM places`).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// ReplacePlaces replaces the local cache with existing catalog rows.
// Refuses an empty batch so a failed read cannot wipe codes. Does not mint codes.
func (s *Store) ReplacePlaces(ctx context.Context, rows []places.Place) error {
	if len(rows) == 0 {
		return fmt.Errorf("places: refuse empty catalog replace")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM places`); err != nil {
		return err
	}
	q := s.pg(`INSERT INTO places (general_code, type_code, brand_code, toptier, toptier_grade,
		label, name, address, city, state, merchant_location, merchant_id, site_key, active)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	n := 0
	for _, p := range rows {
		code := strings.TrimSpace(p.GeneralCode)
		if code == "" {
			continue
		}
		active := 0
		if p.Active {
			active = 1
		}
		if _, err := tx.ExecContext(ctx, q,
			code, p.TypeCode, p.BrandCode, fallback(p.TopTier, "A"), fallback(p.TopTierGrade, "A"),
			p.Label, p.Name, p.Address, p.City, p.State, p.MerchantLocation, p.MerchantID, p.SiteKey, active); err != nil {
			return fmt.Errorf("places insert %s: %w", code, err)
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("places: no general_code rows to cache")
	}
	return tx.Commit()
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return strings.TrimSpace(s)
}
