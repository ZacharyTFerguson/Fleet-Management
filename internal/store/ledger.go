package store

import (
	"context"
	"database/sql"
	"time"

	"oilchange/internal/oil"
)

func (s *Store) UpsertLedgerRow(ctx context.Context, r oil.LedgerRow) error {
	now := time.Now().UTC().Format(time.RFC3339)
	inTrend := 0
	if r.InTrend {
		inTrend = 1
	}
	_, err := s.exec(ctx, `INSERT INTO mileage_ledger (
		efleets_id, card_id, punch_at, merchant, recorded_odo, maint_odo, maint_at, miles_since, expected_odo,
		difference, overage, shortage, abs_diff, trend, status, hold_reason, hold_detail, in_trend, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (efleets_id, punch_at, recorded_odo) DO UPDATE SET
			card_id=excluded.card_id, merchant=excluded.merchant, maint_odo=excluded.maint_odo, maint_at=excluded.maint_at,
			miles_since=excluded.miles_since, expected_odo=excluded.expected_odo, difference=excluded.difference,
			overage=excluded.overage, shortage=excluded.shortage,
			abs_diff=excluded.abs_diff, trend=excluded.trend,
			status=CASE WHEN mileage_ledger.status IN ('dismissed','corrected') THEN mileage_ledger.status ELSE excluded.status END,
			hold_reason=CASE WHEN mileage_ledger.status IN ('dismissed','corrected') THEN mileage_ledger.hold_reason ELSE excluded.hold_reason END,
			hold_detail=CASE WHEN mileage_ledger.status IN ('dismissed','corrected') THEN mileage_ledger.hold_detail ELSE excluded.hold_detail END,
			in_trend=CASE WHEN mileage_ledger.status IN ('dismissed','corrected') THEN false ELSE excluded.in_trend END,
			updated_at=excluded.updated_at`,
		r.EFleetsID, r.CardID, r.PunchAt.UTC().Format(time.RFC3339), r.Merchant, r.Recorded,
		anyIntZeroNil(r.MaintOdo), anyTimeRFC3339(timePtrIfSet(r.MaintAt)), anyFloatPtr(r.MilesSince), anyIntPtr(r.Expected),
		anyIntPtr(r.Difference), r.Overage, r.Shortage, anyIntPtr(r.AbsDiff), r.Trend, r.Status, r.HoldReason, r.HoldDetail, inTrend, now, now)
	return err
}

func anyIntZeroNil(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func timePtrIfSet(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Store) SetLedgerStatus(ctx context.Context, efleetsID, punchAt string, recorded int, status, detail string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `UPDATE mileage_ledger SET status=?, hold_detail=?, in_trend=0, updated_at=?
		WHERE efleets_id=? AND punch_at=? AND recorded_odo=?`,
		status, detail, now, efleetsID, punchAt, recorded)
	return err
}

func (s *Store) ListLedger(ctx context.Context, efleetsID string) ([]oil.LedgerRow, error) {
	q := `SELECT efleets_id, COALESCE(card_id,''), punch_at, COALESCE(merchant,''), recorded_odo, maint_odo, maint_at, miles_since, expected_odo,
		difference, overage, shortage, abs_diff, COALESCE(trend,''), status, COALESCE(hold_reason,''), COALESCE(hold_detail,''), in_trend
		FROM mileage_ledger`
	var args []any
	if efleetsID != "" {
		q += ` WHERE efleets_id=?`
		args = append(args, efleetsID)
	}
	// recorded_odo is part of the ledger's unique key; without it two punches
	// in the same second could swap between box-score renders.
	q += ` ORDER BY efleets_id, punch_at, recorded_odo`
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []oil.LedgerRow
	for rows.Next() {
		var r oil.LedgerRow
		var punch, maint sql.NullString
		var maintOdo, expected, diff, abs sql.NullInt64
		var miles sql.NullFloat64
		var inTrend int
		if err := rows.Scan(&r.EFleetsID, &r.CardID, &punch, &r.Merchant, &r.Recorded, &maintOdo, &maint, &miles, &expected,
			&diff, &r.Overage, &r.Shortage, &abs, &r.Trend, &r.Status, &r.HoldReason, &r.HoldDetail, &inTrend); err != nil {
			return nil, err
		}
		if t := parseStoreTime(punch.String); t != nil {
			r.PunchAt = *t
		}
		if maintOdo.Valid {
			r.MaintOdo = int(maintOdo.Int64)
		}
		if t := parseStoreTime(maint.String); t != nil {
			r.MaintAt = *t
		}
		if miles.Valid {
			v := miles.Float64
			r.MilesSince = &v
		}
		if expected.Valid {
			v := int(expected.Int64)
			r.Expected = &v
		}
		if diff.Valid {
			v := int(diff.Int64)
			r.Difference = &v
		}
		if abs.Valid {
			v := int(abs.Int64)
			r.AbsDiff = &v
		}
		if r.Difference == nil {
			r.Difference = r.SignedDifference()
		}
		r.InTrend = inTrend != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SaveDriveStopWindow(ctx context.Context, factoryID string, from, to time.Time, miles float64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `INSERT INTO drive_stop_windows (factory_id, from_at, to_at, miles, fetched_at) VALUES (?,?,?,?,?)
		ON CONFLICT (factory_id, from_at, to_at) DO UPDATE SET miles=excluded.miles, fetched_at=excluded.fetched_at`,
		factoryID, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), miles, now)
	return err
}

// DriveStopWindow is a measured maint→punch trip. Never device odometer.
type DriveStopWindow struct {
	FactoryID string
	From      time.Time
	To        time.Time
	Miles     float64
}

func (s *Store) ListDriveStopWindows(ctx context.Context) ([]DriveStopWindow, error) {
	rows, err := s.query(ctx, `SELECT factory_id, from_at, to_at, miles FROM drive_stop_windows ORDER BY factory_id, from_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DriveStopWindow
	for rows.Next() {
		var w DriveStopWindow
		var from, to string
		if err := rows.Scan(&w.FactoryID, &from, &to, &w.Miles); err != nil {
			return nil, err
		}
		if t := parseStoreTime(from); t != nil {
			w.From = *t
		}
		if t := parseStoreTime(to); t != nil {
			w.To = *t
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) GetDriveStopWindow(ctx context.Context, factoryID string, from, to time.Time) (float64, bool, error) {
	var miles float64
	err := s.queryRow(ctx, `SELECT miles FROM drive_stop_windows WHERE factory_id=? AND from_at=? AND to_at=?`,
		factoryID, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)).Scan(&miles)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return miles, true, nil
}
