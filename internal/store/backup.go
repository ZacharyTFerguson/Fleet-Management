package store

import (
	"context"
	"database/sql"
	"time"

	"oilchange/internal/model"
)

// MergeCarRemote copies non-null remote last oil / Last Reading / HOLD / interval
// onto an existing cars row. Remote NULL does not wipe a local value or HOLD.
// It does not invent miles and does not call WriteLastReading.
func (s *Store) MergeCarRemote(ctx context.Context, c model.Car) error {
	now := time.Now().UTC().Format(time.RFC3339)
	iv := c.IntervalMiles
	_, err := s.exec(ctx, `UPDATE cars SET
		last_oil_miles=COALESCE(?, last_oil_miles),
		last_oil_date=COALESCE(?, last_oil_date),
		last_reading_miles=COALESCE(?, last_reading_miles),
		last_reading_at=COALESCE(?, last_reading_at),
		last_reading_source=COALESCE(?, last_reading_source),
		hold_reason=COALESCE(?, hold_reason),
		interval_miles=CASE WHEN ? > 0 THEN ? ELSE interval_miles END,
		updated_at=?
		WHERE efleets_id=?`,
		anyIntPtr(c.LastOilMiles), anyTimeRFC3339(c.LastOilDate),
		anyIntPtr(c.LastReadingMiles), anyTimeRFC3339(c.LastReadingAt), anyStrPtr(c.LastReadingSource),
		anyStrPtr(c.HoldReason), iv, iv, now, c.EFleetsID)
	return err
}

// ApplyCarSnapshot copies last oil, Last Reading, HOLD, and interval onto an
// existing cars row. It does not invent miles and does not clear HOLD.
func (s *Store) ApplyCarSnapshot(ctx context.Context, c model.Car) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `UPDATE cars SET
		nickname=?, plate=?, vin=?, region=?,
		last_oil_miles=?, last_oil_date=?,
		last_reading_miles=?, last_reading_at=?, last_reading_source=?,
		hold_reason=?, interval_miles=?, updated_at=?
		WHERE efleets_id=?`,
		c.Nickname, c.Plate, c.VIN, c.Region,
		anyIntPtr(c.LastOilMiles), anyTimeRFC3339(c.LastOilDate),
		anyIntPtr(c.LastReadingMiles), anyTimeRFC3339(c.LastReadingAt), anyStrPtr(c.LastReadingSource),
		anyStrPtr(c.HoldReason), c.IntervalMiles, now, c.EFleetsID)
	return err
}

func anyIntPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func anyStrPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func anyTimeRFC3339(p *time.Time) any {
	if p == nil {
		return nil
	}
	return p.UTC().Format(time.RFC3339)
}

// ListAllFills returns every fill, oldest first.
func (s *Store) ListAllFills(ctx context.Context) ([]model.Fill, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, card_company_vehicle_number, odometer, unusual_y, provider_transaction_time, provider_company_vehicle_number, merchant_name, merchant_address, source
		FROM fills ORDER BY provider_transaction_time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Fill
	for rows.Next() {
		var f model.Fill
		var odo sql.NullInt64
		var unusual bool
		var ts string
		if err := rows.Scan(&f.EFleetsID, &f.CardCompanyVehicleNumber, &odo, &unusual, &ts, &f.ProviderCompanyVehicleNumber, &f.MerchantName, &f.MerchantAddress, &f.Source); err != nil {
			return nil, err
		}
		f.Odometer = scanIntPtr(odo)
		f.UnusualY = unusual
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			f.ProviderTransactionTime = t
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListAllShopROs returns every shop RO, oldest first.
func (s *Store) ListAllShopROs(ctx context.Context) ([]model.ShopRO, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, odometer, at, location_name, COALESCE(ro_id,''), COALESCE(service_desc,'') FROM shop_ros ORDER BY at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ShopRO
	for rows.Next() {
		var r model.ShopRO
		var ts string
		if err := rows.Scan(&r.EFleetsID, &r.Odometer, &ts, &r.LocationName, &r.ROID, &r.ServiceDesc); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			r.At = t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListStations returns every gas station row.
func (s *Store) ListStations(ctx context.Context) ([]model.GasStation, error) {
	rows, err := s.query(ctx, `SELECT id, name, COALESCE(address,''), COALESCE(merchant_id,''), lat, lng FROM gas_stations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.GasStation
	for rows.Next() {
		var g model.GasStation
		var lat, lng sql.NullFloat64
		if err := rows.Scan(&g.ID, &g.Name, &g.Address, &g.MerchantID, &lat, &lng); err != nil {
			return nil, err
		}
		if lat.Valid {
			v := lat.Float64
			g.Lat = &v
		}
		if lng.Valid {
			v := lng.Float64
			g.Lng = &v
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListMaintLocs returns every maintenance location row.
func (s *Store) ListMaintLocs(ctx context.Context) ([]model.MaintenanceLocation, error) {
	rows, err := s.query(ctx, `SELECT id, name, COALESCE(address,''), COALESCE(notes,'') FROM maintenance_locations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MaintenanceLocation
	for rows.Next() {
		var m model.MaintenanceLocation
		if err := rows.Scan(&m.ID, &m.Name, &m.Address, &m.Notes); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListOilChanges returns every oil-change row.
func (s *Store) ListOilChanges(ctx context.Context) ([]model.OilChange, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, miles, date, COALESCE(location,''), source FROM oil_changes ORDER BY date, miles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.OilChange
	for rows.Next() {
		var o model.OilChange
		var day string
		if err := rows.Scan(&o.EFleetsID, &o.Miles, &day, &o.Location, &o.Source); err != nil {
			return nil, err
		}
		if t := parseStoreTime(day); t != nil {
			o.Date = *t
		} else if t, err := time.Parse("2006-01-02", day); err == nil {
			o.Date = t
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// InsertOilChangeRow writes oil_changes only. It does not touch cars.last_oil_*.
func (s *Store) InsertOilChangeRow(ctx context.Context, o model.OilChange) error {
	_, err := s.exec(ctx, `INSERT INTO oil_changes (efleets_id, miles, date, location, source) VALUES (?,?,?,?,?)`,
		o.EFleetsID, o.Miles, o.Date.Format("2006-01-02"), o.Location, o.Source)
	return err
}

// ListHoldEvents returns every HOLD event, open and closed.
func (s *Store) ListHoldEvents(ctx context.Context) ([]model.HoldEvent, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, reason, detail, at, open FROM hold_events ORDER BY at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.HoldEvent
	for rows.Next() {
		var h model.HoldEvent
		var ts string
		var open bool
		if err := rows.Scan(&h.EFleetsID, &h.Reason, &h.Detail, &ts, &open); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			h.At = t
		}
		h.Open = open
		out = append(out, h)
	}
	return out, rows.Err()
}

// HasHoldEvent is true when a matching event already exists (backup idempotency).
func (s *Store) HasHoldEvent(ctx context.Context, h model.HoldEvent) (bool, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM hold_events WHERE efleets_id=? AND reason=? AND at=? AND open=?`,
		h.EFleetsID, h.Reason, h.At.UTC().Format(time.RFC3339), h.Open).Scan(&n)
	return n > 0, err
}

// InsertHoldEvent copies a HOLD row as-is. It does not change cars.hold_reason.
func (s *Store) InsertHoldEvent(ctx context.Context, h model.HoldEvent) error {
	_, err := s.exec(ctx, `INSERT INTO hold_events (efleets_id, reason, detail, at, open) VALUES (?,?,?,?,?)`,
		h.EFleetsID, h.Reason, h.Detail, h.At.UTC().Format(time.RFC3339), h.Open)
	return err
}

// ListAllMilesSince returns every stored drive-stop miles row.
func (s *Store) ListAllMilesSince(ctx context.Context) ([]model.DriveStopMiles, error) {
	rows, err := s.query(ctx, `SELECT factory_id, since_at, miles FROM drive_stop_miles ORDER BY factory_id, since_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DriveStopMiles
	for rows.Next() {
		var m model.DriveStopMiles
		var ts string
		if err := rows.Scan(&m.FactoryID, &ts, &m.Miles); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			m.Since = t
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
