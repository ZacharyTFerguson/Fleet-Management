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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.pg(`DELETE FROM card_eras`)); err != nil {
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
		pairStarted := e.PairStarted
		if pairStarted.IsZero() {
			pairStarted = from
		}
		if _, err := tx.ExecContext(ctx, s.pg(`INSERT INTO card_eras (
			card_id, holder_type, holder_key, efleets_id, nickname, from_at, to_at,
			pair_started_at, switched_at, next_pair_at,
			evidence_n, stations, split, rung
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
			card, ht, hk, nullIfEmpty(e.EFleetsID), e.Nickname,
			from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
			timeOrNull(pairStarted), timePtrOrNull(e.SwitchedAt), timePtrOrNull(e.NextPairAt),
			e.EvidenceN, string(st), split, e.Rung); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListEras returns persisted card location history, oldest first.
func (s *Store) ListEras(ctx context.Context) ([]model.CardEra, error) {
	rows, err := s.query(ctx, `SELECT card_id, holder_type, holder_key, COALESCE(efleets_id,''), COALESCE(nickname,''),
		from_at, to_at, COALESCE(pair_started_at,''), COALESCE(switched_at,''), COALESCE(next_pair_at,''),
		evidence_n, COALESCE(stations,''), split, rung FROM card_eras
		ORDER BY card_id, from_at, holder_type, holder_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CardEra
	for rows.Next() {
		var e model.CardEra
		var from, to, pairStarted, switched, nextPair, st string
		var split int
		if err := rows.Scan(&e.CardID, &e.HolderType, &e.HolderKey, &e.EFleetsID, &e.Nickname,
			&from, &to, &pairStarted, &switched, &nextPair,
			&e.EvidenceN, &st, &split, &e.Rung); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			e.From = t
		}
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			e.To = t
		}
		if t, err := time.Parse(time.RFC3339, pairStarted); err == nil {
			e.PairStarted = t
		}
		e.SwitchedAt = parseTimePtr(switched)
		e.NextPairAt = parseTimePtr(nextPair)
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

func timeOrNull(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func timePtrOrNull(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func parseTimePtr(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil || t.IsZero() {
		return nil
	}
	return &t
}
