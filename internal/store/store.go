package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

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
	if err := applyMigrations(context.Background(), db, driver); err != nil {
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
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() & 0xff {
		case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED, sqlite3.SQLITE_CONSTRAINT:
			return true
		}
		return false
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == "23505"
	}
	return false
}

// retryableBusy is a cross-process lock collision (a second Store or CLI on the
// same SQLite file, or a serialization failure on pgx). The statement itself is
// fine; the transaction just has to be replayed.
func retryableBusy(err error) bool {
	if err == nil {
		return false
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() & 0xff {
		case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
			return true
		}
		return false
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		// serialization_failure, deadlock_detected
		return pe.Code == "40001" || pe.Code == "40P01"
	}
	return false
}

// ErrUnknownCar is returned when a HOLD or Last Reading write targets an
// efleets_id that has no cars row. Silently affecting zero rows would leave a
// car that compute believes it handled with neither hold_reason nor a reading.
var ErrUnknownCar = errors.New("store: unknown efleets_id")

// writeTx runs fn in one write transaction under the in-process mutex and
// replays it a bounded number of times on cross-process lock collisions.
// Every multi-statement mutation (HOLD, Last Reading, oil change, device
// import) goes through here so a second Store on the same file can never
// observe half of one.
func (s *Store) writeTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	for attempt := 0; attempt < 16; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 5 * time.Millisecond):
			}
		}
		err = s.runTx(ctx, fn)
		if err == nil || !retryableBusy(err) {
			return err
		}
	}
	return err
}

func (s *Store) runTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// requireCarTx fails the transaction when the UPDATE on cars matched nothing.
func requireCarTx(res sql.Result, efleetsID string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrUnknownCar, efleetsID)
	}
	return nil
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
		f.EFleetsID, f.CardCompanyVehicleNumber, odo, f.UnusualY, f.ProviderTransactionTime.UTC().Format(time.RFC3339),
		f.ProviderCompanyVehicleNumber, f.MerchantName, f.MerchantAddress, nz(f.Source, "fuel_details"))
	return err
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

// UpsertDevice pairs by factory_id only. Display_name is stored as a label, never used as a join key.
//
// Link semantics (COALESCE keep-link): a row whose LinkedCarEFleetsID is nil —
// the live OneStep API inventory never carries an eFleets ID — refreshes
// device_id / display_name / dead but keeps the existing car link. A row with
// a non-nil link (device map CSV) relinks the box to that car. There is
// deliberately no unlink path through this method; a device can only stop
// counting toward a car by being marked dead.
func (s *Store) UpsertDevice(ctx context.Context, d model.OneStepDevice) error {
	return s.writeTx(ctx, func(tx *sql.Tx) error { return s.upsertDeviceTx(ctx, tx, d) })
}

// UpsertDevices imports a whole registry snapshot in one transaction. A map
// row that fails (for example an efleets_id that has no cars row under the FK)
// aborts the entire import, so a second Store or a `devices` reader never sees
// half of a device map applied. The error names the offending factory_id.
func (s *Store) UpsertDevices(ctx context.Context, devs []model.OneStepDevice) error {
	if len(devs) == 0 {
		return nil
	}
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		for _, d := range devs {
			if err := s.upsertDeviceTx(ctx, tx, d); err != nil {
				return fmt.Errorf("device factory_id %s: %w", d.FactoryID, err)
			}
		}
		return nil
	})
}

func (s *Store) upsertDeviceTx(ctx context.Context, tx *sql.Tx, d model.OneStepDevice) error {
	if d.FactoryID == "" {
		return errors.New("empty factory_id")
	}
	var link any
	if d.LinkedCarEFleetsID != nil && *d.LinkedCarEFleetsID != "" {
		link = *d.LinkedCarEFleetsID
	}
	_, err := tx.ExecContext(ctx, s.pg(`INSERT INTO onestep_devices (factory_id, device_id, display_name, linked_car_efleets_id, dead) VALUES (?,?,?,?,?)
		ON CONFLICT (factory_id) DO UPDATE SET
			device_id=excluded.device_id,
			display_name=excluded.display_name,
			linked_car_efleets_id=COALESCE(excluded.linked_car_efleets_id, onestep_devices.linked_car_efleets_id),
			dead=excluded.dead`),
		d.FactoryID, d.DeviceID, d.DisplayName, link, d.Dead)
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
		var dead bool
		if err := rows.Scan(&d.FactoryID, &d.DeviceID, &d.DisplayName, &link, &dead); err != nil {
			return nil, err
		}
		d.LinkedCarEFleetsID = scanStrPtr(link)
		d.Dead = dead
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
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// WriteLastReading is the only SQL that stores Last Reading miles. Callers must not write on HOLD.
//
// The three last_reading_* columns, hold_reason=NULL, and closing every open
// hold event happen in one transaction: a reader never sees miles without
// at/source, and never sees a cleared hold_reason beside an open event.
// miles must be positive; "no reading" is NULL, never 0.
func (s *Store) WriteLastReading(ctx context.Context, efleetsID string, miles int, at time.Time, source string) error {
	if miles <= 0 {
		return fmt.Errorf("store: refusing last_reading_miles %d for %s; no reading is NULL, not 0", miles, efleetsID)
	}
	if source != model.SourceFuelDetails && source != model.SourceShopRO {
		return fmt.Errorf("store: last_reading_source %q for %s is not fuel_details or shop_ro", source, efleetsID)
	}
	if at.IsZero() {
		return fmt.Errorf("store: last_reading_at is required for %s", efleetsID)
	}
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		res, err := tx.ExecContext(ctx, s.pg(`UPDATE cars SET last_reading_miles=?, last_reading_at=?, last_reading_source=?, hold_reason=NULL, updated_at=? WHERE efleets_id=?`),
			miles, at.UTC().Format(time.RFC3339), source, now, efleetsID)
		if err != nil {
			return err
		}
		if err := requireCarTx(res, efleetsID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, s.pg(`UPDATE hold_events SET open=FALSE WHERE efleets_id=? AND open=TRUE`), efleetsID)
		return err
	})
}

// SetHold skips Last Reading. It touches hold_reason only; last_reading_miles,
// last_reading_at and last_reading_source are left exactly as the last
// WriteLastReading committed them (the exporter blanks them while a HOLD is
// open so a stale number is never shown as current odo).
//
// SetHold is idempotent per (efleets_id, reason, detail): the car ends the
// transaction with exactly one open hold_events row, which matches
// cars.hold_reason. A repeated NO_DEVICE compute therefore does not stack a
// new open event every tick, and a changed reason closes the previous one
// instead of leaving two "current" holds.
func (s *Store) SetHold(ctx context.Context, efleetsID, reason, detail string) error {
	if reason == "" {
		return fmt.Errorf("store: empty hold reason for %s", efleetsID)
	}
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		res, err := tx.ExecContext(ctx, s.pg(`UPDATE cars SET hold_reason=?, updated_at=? WHERE efleets_id=?`), reason, now, efleetsID)
		if err != nil {
			return err
		}
		if err := requireCarTx(res, efleetsID); err != nil {
			return err
		}
		// Close every open event except the oldest one that already says the
		// same thing. COALESCE(...,-1) keeps the predicate true when there is
		// no such event (id <> NULL would match nothing).
		if _, err := tx.ExecContext(ctx, s.pg(`UPDATE hold_events SET open=FALSE
			WHERE efleets_id=? AND open=TRUE
			  AND id <> COALESCE((SELECT MIN(id) FROM hold_events
			                      WHERE efleets_id=? AND open=TRUE AND reason=? AND COALESCE(detail,'')=?), -1)`),
			efleetsID, efleetsID, reason, detail); err != nil {
			return err
		}
		var open int
		if err := tx.QueryRowContext(ctx, s.pg(`SELECT COUNT(*) FROM hold_events WHERE efleets_id=? AND open=TRUE`), efleetsID).Scan(&open); err != nil {
			return err
		}
		if open > 0 {
			return nil
		}
		_, err = tx.ExecContext(ctx, s.pg(`INSERT INTO hold_events (efleets_id, reason, detail, at, open) VALUES (?,?,?,?,?)`),
			efleetsID, reason, detail, now, true)
		return err
	})
}

// ClearHolds is the operator escape hatch: it closes open events and clears
// hold_reason in one transaction so the car never has a hold_reason with no
// open event (or vice versa). It does not write a Last Reading; the car goes
// back to "never computed" until the next compute decides.
func (s *Store) ClearHolds(ctx context.Context, efleetsID string) error {
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := tx.ExecContext(ctx, s.pg(`UPDATE cars SET hold_reason=NULL, updated_at=? WHERE efleets_id=?`), now, efleetsID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.pg(`UPDATE hold_events SET open=FALSE WHERE efleets_id=? AND open=TRUE`), efleetsID)
		return err
	})
}

// OpenHolds is the holds command.
func (s *Store) OpenHolds(ctx context.Context) ([]model.HoldEvent, error) {
	rows, err := s.query(ctx, `SELECT efleets_id, reason, detail, at, open FROM hold_events WHERE open=TRUE ORDER BY at`)
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

// InsertOilChange records last oil. It does not change Last Reading.
// The history row and the cars.last_oil_* denormalisation commit together so
// a crash between them cannot leave an oil_changes row the car does not reflect.
func (s *Store) InsertOilChange(ctx context.Context, o model.OilChange) error {
	return s.writeTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.pg(`INSERT INTO oil_changes (efleets_id, miles, date, location, source) VALUES (?,?,?,?,?)`),
			o.EFleetsID, o.Miles, o.Date.Format("2006-01-02"), o.Location, o.Source); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := tx.ExecContext(ctx, s.pg(`UPDATE cars SET last_oil_miles=?, last_oil_date=?, updated_at=? WHERE efleets_id=?`),
			o.Miles, o.Date.Format(time.RFC3339), now, o.EFleetsID)
		return err
	})
}

// HasOilChange avoids double-seeding the same shop RO oil service.
func (s *Store) HasOilChange(ctx context.Context, efleetsID string, miles int, day time.Time) (bool, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM oil_changes WHERE efleets_id=? AND miles=? AND date=?`,
		efleetsID, miles, day.Format("2006-01-02")).Scan(&n)
	return n > 0, err
}
