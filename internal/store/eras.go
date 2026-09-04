package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"oilchange/internal/model"
)

// ReplaceEras persists GPS / station-ladder card history (car, person, office).
// Full-table rewrite under the store mutex so rebuild and backup cannot tear sqlite.
// Never writes Last Reading.
func (s *Store) ReplaceEras(ctx context.Context, rows []model.CardEra) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.exec(ctx, `DELETE FROM card_eras`); err != nil {
		return err
	}
	for _, e := range rows {
		card := strings.TrimSpace(e.CardID)
		if card == "" {
			continue
		}
		ht := strings.TrimSpace(e.HolderType)
		if ht == "" {
			ht = "car"
		}
		hk := strings.TrimSpace(e.HolderKey)
		if hk == "" {
			hk = strings.TrimSpace(e.EFleetsID)
		}
		if hk == "" {
			continue
		}
		from, to := e.From, e.To
		if from.IsZero() {
			from = to
		}
		if to.IsZero() {
			to = from
		}
		if from.IsZero() {
			continue
		}
		st, err := json.Marshal(e.Stations)
		if err != nil {
			return err
		}
		split := 0
		if e.Split {
			split = 1
		}
		if _, err := s.exec(ctx, `INSERT INTO card_eras (
			card_id, holder_type, holder_key, efleets_id, nickname, from_at, to_at, evidence_n, stations, split, rung
		) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			card, ht, hk, nullIfEmpty(e.EFleetsID), e.Nickname,
			from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
			e.EvidenceN, string(st), split, e.Rung); err != nil {
			return err
		}
	}
	return nil
}

// ListEras returns persisted card location history, oldest first.
func (s *Store) ListEras(ctx context.Context) ([]model.CardEra, error) {
	rows, err := s.query(ctx, `SELECT card_id, holder_type, holder_key, COALESCE(efleets_id,''), COALESCE(nickname,''),
		from_at, to_at, evidence_n, COALESCE(stations,''), split, rung FROM card_eras
		ORDER BY card_id, from_at, holder_type, holder_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CardEra
	for rows.Next() {
		var e model.CardEra
		var from, to, st string
		var split int
		if err := rows.Scan(&e.CardID, &e.HolderType, &e.HolderKey, &e.EFleetsID, &e.Nickname,
			&from, &to, &e.EvidenceN, &st, &split, &e.Rung); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			e.From = t
		}
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			e.To = t
		}
		e.Split = split != 0
		if st != "" && st != "null" {
			_ = json.Unmarshal([]byte(st), &e.Stations)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
