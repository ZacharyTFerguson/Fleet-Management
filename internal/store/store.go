package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"oilchange/internal/model"
)

// Store is the fleet-oil database. SQLite is a test double only.
type Store struct {
	db      *sql.DB
	dialect string
	// mu serializes in-process access. Concurrent CLI ticks (sync --interval),
	// compute, and sync-enterprise can share one SQLite file; database/sql's
	// pool plus modernc otherwise races nextPDI (COUNT then INSERT) and
	// last_reading vs HOLD writes. Cross-process locking is SQLite BEGIN
	// IMMEDIATE (_txlock=immediate) plus busy timeout.
	mu sync.Mutex
}

// Open migrates and returns a store. dialect is sqlite or pgx.
func Open(driver, dsn string) (*Store, error) {
	if driver == "sqlite" {
		if dsn == "" {
			return nil, fmt.Errorf("empty sqlite path")
		}
		dsn = "file:" + dsn + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		// One connection so SQLite writers are not interleaved across the pool.
		db.SetMaxOpenConns(1)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := applyMigrations(db, driver); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, dialect: driver}, nil
}

// Close releases the pool.
func (s *Store) Close() error { return s.db.Close() }

// pg rewrites ? placeholders so the same statements run on pgx (fleet-oil) and sqlite (tests).
func (s *Store) pg(q string) string {
	if s.dialect != "pgx" {
		return q
	}
	n := 0
	var b strings.Builder
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

// exec runs a statement after placeholder rewrite so tests (sqlite) and fleet-oil (pgx) share SQL.
func (s *Store) rawExec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.pg(q), args...)
}

func (s *Store) rawQuery(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.pg(q), args...)
}

func (s *Store) rawQueryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.pg(q), args...)
}

// exec runs a statement after placeholder rewrite so tests (sqlite) and fleet-oil (pgx) share SQL.
func (s *Store) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.rawExec(ctx, q, args...)
}

// query is the read twin of exec (placeholder rewrite).
func (s *Store) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.rawQuery(ctx, q, args...)
}

// queryRow is the single-row twin of query.
func (s *Store) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.rawQueryRow(ctx, q, args...)
}

// scanIntPtr keeps SQL NULL as Go nil so "no last reading" is not stored as 0 miles.
func scanIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// scanTimePtr keeps missing timestamps nil so compute can tell "never written" from epoch.
func scanTimePtr(n sql.NullString) *time.Time {
	if !n.Valid || n.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, n.String)
	if err != nil {
		t, err = time.Parse("2006-01-02", n.String)
		if err != nil {
			return nil
		}
	}
	return &t
}

// scanStrPtr keeps NULL hold_reason as nil (no HOLD) rather than empty string.
func scanStrPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	s := n.String
	return &s
}

// UpsertCar inserts or updates identity fields without clobbering last_reading on a re-sync.
// COUNT+INSERT for a new PDI runs in one IMMEDIATE/serializable transaction so two
// Store instances on the same SQLite file cannot mint the same PDI-NNNN.
func (s *Store) UpsertCar(ctx context.Context, c model.Car) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	for attempt := 0; attempt < 16; attempt++ {
		err = s.upsertCarTx(ctx, c)
		if err == nil {
			return nil
		}
		if !retryableAlloc(err) {
			return err
		}
	}
	return err
}

func (s *Store) upsertCarTx(ctx context.Context, c model.Car) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	existing, err := s.carByEFleetsTx(ctx, tx, c.EFleetsID)
	if err == nil && existing != nil {
		_, err = tx.ExecContext(ctx, s.pg(`UPDATE cars SET nickname=?, plate=?, vin=?, region=?, updated_at=? WHERE efleets_id=?`),
			c.Nickname, c.Plate, c.VIN, c.Region, time.Now().UTC().Format(time.RFC3339), c.EFleetsID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if c.PDIID != "" {
		inUse, err := s.pdiInUseTx(ctx, tx, c.PDIID)
		if err != nil {
			return err
		}
		if inUse {
			// Fleet Summary derives row-order PDI values. A newly inserted car
			// can therefore arrive with an ID already owned by a different car.
			c.PDIID = ""
		}
	}
	if c.PDIID == "" {
		id, err := s.nextPDITx(ctx, tx)
		if err != nil {
			return err
		}
		c.PDIID = id
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx, s.pg(`INSERT INTO cars (pdi_id, efleets_id, nickname, plate, vin, region, interval_miles, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`), c.PDIID, c.EFleetsID, c.Nickname, c.Plate, c.VIN, c.Region, c.IntervalMiles, now, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) carByEFleetsTx(ctx context.Context, tx *sql.Tx, id string) (*model.Car, error) {
	row := tx.QueryRowContext(ctx, s.pg(`SELECT pdi_id, efleets_id, nickname, plate, vin, region,
		last_oil_miles, last_oil_date, last_reading_miles, last_reading_at, last_reading_source, hold_reason, interval_miles
		FROM cars WHERE efleets_id=?`), id)
	return scanCar(row)
}

// nextPDITx allocates opaque PDI-NNNN inside an open write txn (caller holds the lock).
func (s *Store) nextPDITx(ctx context.Context, tx *sql.Tx) (string, error) {
	var n int
	err := tx.QueryRowContext(ctx, s.pg(`SELECT COUNT(*) FROM cars`)).Scan(&n)
	if err != nil {
		return "", err
	}
	for {
		n++
		id := fmt.Sprintf("PDI-%04d", n)
		inUse, err := s.pdiInUseTx(ctx, tx, id)
		if err != nil {
			return "", err
		}
		if !inUse {
			return id, nil
		}
	}
}

func (s *Store) pdiInUseTx(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, s.pg(`SELECT 1 FROM cars WHERE pdi_id=?`), id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func retryableAlloc(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "primary key") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy")
}

// CarByEFleets loads one car. EFleetsID is the join key.
func (s *Store) CarByEFleets(ctx context.Context, id string) (*model.Car, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.carByEFleetsLocked(ctx, id)
}

func (s *Store) carByEFleetsLocked(ctx context.Context, id string) (*model.Car, error) {
	row := s.rawQueryRow(ctx, `SELECT pdi_id, efleets_id, nickname, plate, vin, region,
		last_oil_miles, last_oil_date, last_reading_miles, last_reading_at, last_reading_source, hold_reason, interval_miles
		FROM cars WHERE efleets_id=?`, id)
	return scanCar(row)
}

// scanCar maps nullable last_reading_* so HOLD rows do not look like a zero odometer.
func scanCar(row *sql.Row) (*model.Car, error) {
	var c model.Car
	var oilM, readM, interval sql.NullInt64
	var oilD, readA, src, hold sql.NullString
	err := row.Scan(&c.PDIID, &c.EFleetsID, &c.Nickname, &c.Plate, &c.VIN, &c.Region,
		&oilM, &oilD, &readM, &readA, &src, &hold, &interval)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	c.LastOilMiles = scanIntPtr(oilM)
	c.LastOilDate = scanTimePtr(oilD)
	c.LastReadingMiles = scanIntPtr(readM)
	c.LastReadingAt = scanTimePtr(readA)
	c.LastReadingSource = scanStrPtr(src)
	c.HoldReason = scanStrPtr(hold)
	if interval.Valid {
		c.IntervalMiles = int(interval.Int64)
	}
	return &c, nil
}

// ListCars is the report/compute universe.
func (s *Store) ListCars(ctx context.Context) ([]model.Car, error) {
	rows, err := s.query(ctx, `SELECT pdi_id, efleets_id, nickname, plate, vin, region,
		last_oil_miles, last_oil_date, last_reading_miles, last_reading_at, last_reading_source, hold_reason, interval_miles
		FROM cars ORDER BY efleets_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Car
	for rows.Next() {
		var c model.Car
		var oilM, readM, interval sql.NullInt64
		var oilD, readA, src, hold sql.NullString
		if err := rows.Scan(&c.PDIID, &c.EFleetsID, &c.Nickname, &c.Plate, &c.VIN, &c.Region,
			&oilM, &oilD, &readM, &readA, &src, &hold, &interval); err != nil {
			return nil, err
		}
		c.LastOilMiles = scanIntPtr(oilM)
		c.LastOilDate = scanTimePtr(oilD)
		c.LastReadingMiles = scanIntPtr(readM)
		c.LastReadingAt = scanTimePtr(readA)
		c.LastReadingSource = scanStrPtr(src)
		c.HoldReason = scanStrPtr(hold)
		if interval.Valid {
			c.IntervalMiles = int(interval.Int64)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertFill is idempotent on eFleets ID + fill second + odo.
func (s *Store) UpsertFill(ctx context.Context, f model.Fill) error {
	var odo any
	if f.Odometer != nil {
		odo = *f.Odometer
	}
	_, err := s.exec(ctx, `INSERT INTO fills (efleets_id, card_company_vehicle_number, odometer, unusual_y, provider_transaction_time, provider_company_vehicle_number, merchant_name, merchant_address, source)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT (efleets_id, provider_transaction_time, odometer) DO NOTHING`,
		f.EFleetsID, f.CardCompanyVehicleNumber, odo, boolToInt(f.UnusualY), f.ProviderTransactionTime.UTC().Format(time.RFC3339),
		f.ProviderCompanyVehicleNumber, f.MerchantName, f.MerchantAddress, nz(f.Source, "fuel_details"))
	return err
}

// boolToInt is sqlite's stand-in for BOOLEAN; postgres still gets 0/1 which it coerces.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nz fills source='fuel_details' when the CSV omitted it so the CHECK constraint stays honest.
func nz(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// ListFills returns punches for one car, oldest first, so the fill picker can walk the chain.
func (s *Store) ListFills(ctx context.Context, efleetsID string) ([]model.Fill, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, card_company_vehicle_number, odometer, unusual_y, provider_transaction_time, provider_company_vehicle_number, merchant_name, merchant_address, source
		FROM fills WHERE efleets_id=? ORDER BY provider_transaction_time`, efleetsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Fill
	for rows.Next() {
		var f model.Fill
		var odo sql.NullInt64
		var unusual int
		var ts string
		if err := rows.Scan(&f.EFleetsID, &f.CardCompanyVehicleNumber, &odo, &unusual, &ts, &f.ProviderCompanyVehicleNumber, &f.MerchantName, &f.MerchantAddress, &f.Source); err != nil {
			return nil, err
		}
		f.Odometer = scanIntPtr(odo)
		f.UnusualY = unusual != 0
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			f.ProviderTransactionTime = t
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// UpsertShopRO stores one RO. Line-item collapse happens in the parser.
func (s *Store) UpsertShopRO(ctx context.Context, r model.ShopRO) error {
	_, err := s.exec(ctx, `INSERT INTO shop_ros (efleets_id, odometer, at, location_name, ro_id, service_desc) VALUES (?,?,?,?,?,?)
		ON CONFLICT (efleets_id, at, odometer) DO NOTHING`,
		r.EFleetsID, r.Odometer, r.At.UTC().Format(time.RFC3339), r.LocationName, r.ROID, r.ServiceDesc)
	return err
}

// ListShopROs is shop history for Last Reading and last-oil seed.
func (s *Store) ListShopROs(ctx context.Context, efleetsID string) ([]model.ShopRO, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, odometer, at, location_name, COALESCE(ro_id,''), COALESCE(service_desc,'') FROM shop_ros WHERE efleets_id=? ORDER BY at`, efleetsID)
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

// UpsertStation keeps merchant rows for later matching, not for Last Reading math.
func (s *Store) UpsertStation(ctx context.Context, g model.GasStation) error {
	_, err := s.exec(ctx, `INSERT INTO gas_stations (id, name, address, merchant_id) VALUES (?,?,?,?)
		ON CONFLICT (id) DO UPDATE SET name=excluded.name, address=excluded.address`, g.ID, g.Name, g.Address, g.MerchantID)
	return err
}

// UpsertMaintLoc keeps shop names from ROs.
func (s *Store) UpsertMaintLoc(ctx context.Context, m model.MaintenanceLocation) error {
	_, err := s.exec(ctx, `INSERT INTO maintenance_locations (id, name, address, notes) VALUES (?,?,?,?)
		ON CONFLICT (id) DO UPDATE SET name=excluded.name`, m.ID, m.Name, m.Address, m.Notes)
	return err
}

// UpsertCard stores a fuel card. CARD_MIX callers pass a nil link.
func (s *Store) UpsertCard(ctx context.Context, c model.Card) error {
	var link any
	if c.LinkedCarEFleetsID != nil {
		link = *c.LinkedCarEFleetsID
	}
	_, err := s.exec(ctx, `INSERT INTO cards (id, company_vehicle_number, linked_car_efleets_id, notes) VALUES (?,?,?,?)
		ON CONFLICT (id) DO UPDATE SET company_vehicle_number=excluded.company_vehicle_number, linked_car_efleets_id=excluded.linked_car_efleets_id, notes=excluded.notes`,
		c.ID, c.CompanyVehicleNumber, link, c.Notes)
	return err
}

// ClearCardLink is CARD_MIX: the card is not this car.
func (s *Store) ClearCardLink(ctx context.Context, cardID string) error {
	_, err := s.exec(ctx, `UPDATE cards SET linked_car_efleets_id=NULL WHERE id=?`, cardID)
	return err
}

// UpsertDevice pairs by factory_id. Display_name is stored as a label only.
func (s *Store) UpsertDevice(ctx context.Context, d model.OneStepDevice) error {
	var link any
	if d.LinkedCarEFleetsID != nil {
		link = *d.LinkedCarEFleetsID
	}
	_, err := s.exec(ctx, `INSERT INTO onestep_devices (factory_id, device_id, display_name, linked_car_efleets_id, dead) VALUES (?,?,?,?,?)
		ON CONFLICT (factory_id) DO UPDATE SET device_id=excluded.device_id, display_name=excluded.display_name, linked_car_efleets_id=excluded.linked_car_efleets_id, dead=excluded.dead`,
		d.FactoryID, d.DeviceID, d.DisplayName, link, boolToInt(d.Dead))
	return err
}

// ListDevicesForCar returns boxes linked by factory_id, including dead ones so compute can ignore them.
func (s *Store) ListDevicesForCar(ctx context.Context, efleetsID string) ([]model.OneStepDevice, error) {
	rows, err := s.query(ctx, `SELECT factory_id, device_id, display_name, linked_car_efleets_id, dead FROM onestep_devices WHERE linked_car_efleets_id=?`, efleetsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.OneStepDevice
	for rows.Next() {
		var d model.OneStepDevice
		var link sql.NullString
		var dead int
		if err := rows.Scan(&d.FactoryID, &d.DeviceID, &d.DisplayName, &link, &dead); err != nil {
			return nil, err
		}
		d.LinkedCarEFleetsID = scanStrPtr(link)
		d.Dead = dead != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveMilesSince stores GPS trip sum after a known second.
func (s *Store) SaveMilesSince(ctx context.Context, m model.DriveStopMiles) error {
	_, err := s.exec(ctx, `INSERT INTO drive_stop_miles (factory_id, since_at, miles, fetched_at) VALUES (?,?,?,?)
		ON CONFLICT (factory_id, since_at) DO UPDATE SET miles=excluded.miles, fetched_at=excluded.fetched_at`,
		m.FactoryID, m.Since.UTC().Format(time.RFC3339), m.Miles, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ListMilesSince returns stored trip sums for compute. Compute does not call OneStep itself.
func (s *Store) ListMilesSince(ctx context.Context, factoryIDs []string) ([]model.DriveStopMiles, error) {
	var out []model.DriveStopMiles
	for _, id := range factoryIDs {
		rows, err := s.query(ctx, `SELECT factory_id, since_at, miles FROM drive_stop_miles WHERE factory_id=?`, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var m model.DriveStopMiles
			var ts string
			if err := rows.Scan(&m.FactoryID, &ts, &m.Miles); err != nil {
				rows.Close()
				return nil, err
			}
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				m.Since = t
			}
			out = append(out, m)
		}
		rows.Close()
	}
	return out, nil
}

// WriteLastReading is the only SQL that stores Last Reading miles. Callers must not write on HOLD.
func (s *Store) WriteLastReading(ctx context.Context, efleetsID string, miles int, at time.Time, source string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `UPDATE cars SET last_reading_miles=?, last_reading_at=?, last_reading_source=?, hold_reason=NULL, updated_at=? WHERE efleets_id=?`,
		miles, at.UTC().Format(time.RFC3339), source, now, efleetsID)
	return err
}

// SetHold skips Last Reading. Prior last_reading_* stay put so operators do not trust a flagged number.
func (s *Store) SetHold(ctx context.Context, efleetsID, reason, detail string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.exec(ctx, `UPDATE cars SET hold_reason=?, updated_at=? WHERE efleets_id=?`, reason, now, efleetsID); err != nil {
		return err
	}
	_, err := s.exec(ctx, `INSERT INTO hold_events (efleets_id, reason, detail, at, open) VALUES (?,?,?,?,1)`,
		efleetsID, reason, detail, now)
	return err
}

// ClearHolds closes open events after a successful write.
func (s *Store) ClearHolds(ctx context.Context, efleetsID string) error {
	_, err := s.exec(ctx, `UPDATE hold_events SET open=0 WHERE efleets_id=? AND open=1`, efleetsID)
	return err
}

// OpenHolds is the holds command.
func (s *Store) OpenHolds(ctx context.Context) ([]model.HoldEvent, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, reason, detail, at, open FROM hold_events WHERE open=1 ORDER BY at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.HoldEvent
	for rows.Next() {
		var h model.HoldEvent
		var ts string
		var open int
		if err := rows.Scan(&h.EFleetsID, &h.Reason, &h.Detail, &ts, &open); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			h.At = t
		}
		h.Open = open != 0
		out = append(out, h)
	}
	return out, rows.Err()
}

// InsertOilChange records last oil. It does not change Last Reading.
func (s *Store) InsertOilChange(ctx context.Context, o model.OilChange) error {
	if _, err := s.exec(ctx, `INSERT INTO oil_changes (efleets_id, miles, date, location, source) VALUES (?,?,?,?,?)`,
		o.EFleetsID, o.Miles, o.Date.Format("2006-01-02"), o.Location, o.Source); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `UPDATE cars SET last_oil_miles=?, last_oil_date=?, updated_at=? WHERE efleets_id=?`,
		o.Miles, o.Date.Format(time.RFC3339), now, o.EFleetsID)
	return err
}

// HasOilChange avoids double-seeding the same shop RO oil service.
func (s *Store) HasOilChange(ctx context.Context, efleetsID string, miles int, day time.Time) (bool, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM oil_changes WHERE efleets_id=? AND miles=? AND date=?`,
		efleetsID, miles, day.Format("2006-01-02")).Scan(&n)
	return n > 0, err
}
