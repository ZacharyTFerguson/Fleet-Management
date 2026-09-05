package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"oilchange/internal/model"
)

// GetAssignment returns the current home of one fill, or a zero value.
func (s *Store) GetAssignment(ctx context.Context, txKey string) (model.TxAssignment, error) {
	txKey = strings.TrimSpace(txKey)
	var a model.TxAssignment
	if txKey == "" {
		return a, nil
	}
	row := s.queryRow(ctx, `SELECT tx_key, COALESCE(assigned_efleets_id,''), COALESCE(assigned_pdi_id,''),
		COALESCE(source,''), COALESCE(gps_called_efleets_id,''), gps_disagrees, updated_at
		FROM transaction_assignments WHERE tx_key=?`, txKey)
	var updated string
	var flag int
	err := row.Scan(&a.TxKey, &a.AssignedEFleetsID, &a.AssignedPDIID, &a.Source,
		&a.GPSCalledEFleetsID, &flag, &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.TxAssignment{}, nil
		}
		return a, err
	}
	a.GPSDisagrees = flag != 0
	if t, perr := time.Parse(time.RFC3339, updated); perr == nil {
		a.UpdatedAt = t
	}
	return a, nil
}

// ListAssignments returns every current fill home.
func (s *Store) ListAssignments(ctx context.Context) ([]model.TxAssignment, error) {
	rows, err := s.query(ctx, `SELECT tx_key, COALESCE(assigned_efleets_id,''), COALESCE(assigned_pdi_id,''),
		COALESCE(source,''), COALESCE(gps_called_efleets_id,''), gps_disagrees, updated_at
		FROM transaction_assignments ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.TxAssignment
	for rows.Next() {
		var a model.TxAssignment
		var updated string
		var flag int
		if err := rows.Scan(&a.TxKey, &a.AssignedEFleetsID, &a.AssignedPDIID, &a.Source,
			&a.GPSCalledEFleetsID, &flag, &updated); err != nil {
			return nil, err
		}
		a.GPSDisagrees = flag != 0
		if t, perr := time.Parse(time.RFC3339, updated); perr == nil {
			a.UpdatedAt = t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AssignTx moves one fill. Empty toEFleets unassigns. Writes an audit event.
// Does not write Last Reading. Does not change card_transactions.
func (s *Store) AssignTx(ctx context.Context, txKey, toEFleets, toPDI, actor, reason string) (model.TxAssignment, error) {
	txKey = strings.TrimSpace(txKey)
	toEFleets = strings.TrimSpace(toEFleets)
	toPDI = strings.TrimSpace(toPDI)
	if actor == "" {
		actor = "owner"
	}
	if reason == "" {
		reason = "manual_drag"
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, err := s.getAssignmentLocked(ctx, txKey)
	if err != nil {
		return cur, err
	}
	fromE, fromP := cur.AssignedEFleetsID, cur.AssignedPDIID
	flag := 0
	if cur.GPSCalledEFleetsID != "" && toEFleets != "" && cur.GPSCalledEFleetsID != toEFleets {
		flag = 1
	}
	src := ""
	if toEFleets != "" || toPDI != "" {
		src = "owner"
	}
	if _, err := s.exec(ctx, `INSERT INTO transaction_assignments
		(tx_key, assigned_efleets_id, assigned_pdi_id, source, gps_called_efleets_id, gps_disagrees, updated_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT (tx_key) DO UPDATE SET
		  assigned_efleets_id=excluded.assigned_efleets_id,
		  assigned_pdi_id=excluded.assigned_pdi_id,
		  source=excluded.source,
		  gps_disagrees=excluded.gps_disagrees,
		  updated_at=excluded.updated_at`,
		txKey, nullIfEmpty(toEFleets), nullIfEmpty(toPDI), src,
		nullIfEmpty(cur.GPSCalledEFleetsID), flag, now.Format(time.RFC3339)); err != nil {
		return cur, err
	}
	if _, err := s.exec(ctx, `INSERT INTO assignment_events
		(tx_key, from_efleets_id, to_efleets_id, from_pdi_id, to_pdi_id, actor, reason, at)
		VALUES (?,?,?,?,?,?,?,?)`,
		txKey, nullIfEmpty(fromE), nullIfEmpty(toEFleets), nullIfEmpty(fromP), nullIfEmpty(toPDI),
		actor, reason, now.Format(time.RFC3339)); err != nil {
		return cur, err
	}
	cur.TxKey = txKey
	cur.AssignedEFleetsID = toEFleets
	cur.AssignedPDIID = toPDI
	cur.Source = src
	cur.GPSDisagrees = flag != 0
	cur.UpdatedAt = now
	return cur, nil
}

// RefreshGPSCalls stores GPS-named cars after rematch. Owner Assigned* is kept.
// When GPS names a different car than the owner, GPSDisagrees becomes true.
func (s *Store) RefreshGPSCalls(ctx context.Context, txs []model.CardTx) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range txs {
		key := t.Key()
		if key == "||0" || strings.TrimSpace(t.CardID) == "" {
			continue
		}
		called := strings.TrimSpace(t.CalledEFleetsID)
		cur, err := s.getAssignmentLocked(ctx, key)
		if err != nil {
			return err
		}
		assigned := cur.AssignedEFleetsID
		flag := 0
		if assigned != "" && called != "" && assigned != called {
			flag = 1
		}
		src := cur.Source
		if _, err := s.exec(ctx, `INSERT INTO transaction_assignments
			(tx_key, assigned_efleets_id, assigned_pdi_id, source, gps_called_efleets_id, gps_disagrees, updated_at)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT (tx_key) DO UPDATE SET
			  gps_called_efleets_id=excluded.gps_called_efleets_id,
			  gps_disagrees=excluded.gps_disagrees`,
			key, nullIfEmpty(assigned), nullIfEmpty(cur.AssignedPDIID), src,
			nullIfEmpty(called), flag, now); err != nil {
			return err
		}
	}
	return nil
}

// ListAssignmentEvents returns the audit log for one fill (or all if key empty).
func (s *Store) ListAssignmentEvents(ctx context.Context, txKey string) ([]model.AssignmentEvent, error) {
	q := `SELECT id, tx_key, COALESCE(from_efleets_id,''), COALESCE(to_efleets_id,''),
		COALESCE(from_pdi_id,''), COALESCE(to_pdi_id,''), actor, reason, at
		FROM assignment_events`
	var args []any
	if strings.TrimSpace(txKey) != "" {
		q += ` WHERE tx_key=?`
		args = append(args, strings.TrimSpace(txKey))
	}
	q += ` ORDER BY at, id`
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AssignmentEvent
	for rows.Next() {
		var e model.AssignmentEvent
		var at string
		if err := rows.Scan(&e.ID, &e.TxKey, &e.FromEFleetsID, &e.ToEFleetsID,
			&e.FromPDIID, &e.ToPDIID, &e.Actor, &e.Reason, &at); err != nil {
			return nil, err
		}
		if t, perr := time.Parse(time.RFC3339, at); perr == nil {
			e.At = t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReplaceAssignment copies one current home without writing an audit event.
func (s *Store) ReplaceAssignment(ctx context.Context, a model.TxAssignment) error {
	if strings.TrimSpace(a.TxKey) == "" {
		return nil
	}
	flag := 0
	if a.GPSDisagrees {
		flag = 1
	}
	at := a.UpdatedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := s.exec(ctx, `INSERT INTO transaction_assignments
		(tx_key, assigned_efleets_id, assigned_pdi_id, source, gps_called_efleets_id, gps_disagrees, updated_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT (tx_key) DO UPDATE SET
		  assigned_efleets_id=excluded.assigned_efleets_id,
		  assigned_pdi_id=excluded.assigned_pdi_id,
		  source=excluded.source,
		  gps_called_efleets_id=excluded.gps_called_efleets_id,
		  gps_disagrees=excluded.gps_disagrees,
		  updated_at=excluded.updated_at`,
		a.TxKey, nullIfEmpty(a.AssignedEFleetsID), nullIfEmpty(a.AssignedPDIID), a.Source,
		nullIfEmpty(a.GPSCalledEFleetsID), flag, at.UTC().Format(time.RFC3339))
	return err
}

// ReplaceAssignmentEvents rewrites the audit log (backup only).
func (s *Store) ReplaceAssignmentEvents(ctx context.Context, rows []model.AssignmentEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.exec(ctx, `DELETE FROM assignment_events`); err != nil {
		return err
	}
	for _, e := range rows {
		if strings.TrimSpace(e.TxKey) == "" {
			continue
		}
		at := e.At
		if at.IsZero() {
			at = time.Now().UTC()
		}
		if _, err := s.exec(ctx, `INSERT INTO assignment_events
			(tx_key, from_efleets_id, to_efleets_id, from_pdi_id, to_pdi_id, actor, reason, at)
			VALUES (?,?,?,?,?,?,?,?)`,
			e.TxKey, nullIfEmpty(e.FromEFleetsID), nullIfEmpty(e.ToEFleetsID),
			nullIfEmpty(e.FromPDIID), nullIfEmpty(e.ToPDIID),
			firstNonEmptyStore(e.Actor, "owner"), firstNonEmptyStore(e.Reason, "manual_drag"),
			at.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

func firstNonEmptyStore(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func (s *Store) getAssignmentLocked(ctx context.Context, txKey string) (model.TxAssignment, error) {
	var a model.TxAssignment
	row := s.queryRow(ctx, `SELECT tx_key, COALESCE(assigned_efleets_id,''), COALESCE(assigned_pdi_id,''),
		COALESCE(source,''), COALESCE(gps_called_efleets_id,''), gps_disagrees, updated_at
		FROM transaction_assignments WHERE tx_key=?`, txKey)
	var updated string
	var flag int
	err := row.Scan(&a.TxKey, &a.AssignedEFleetsID, &a.AssignedPDIID, &a.Source,
		&a.GPSCalledEFleetsID, &flag, &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.TxAssignment{TxKey: txKey}, nil
		}
		return a, err
	}
	a.GPSDisagrees = flag != 0
	if t, perr := time.Parse(time.RFC3339, updated); perr == nil {
		a.UpdatedAt = t
	}
	return a, nil
}
