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
	return parseStoreTime(n.String)
}

func parseStoreTime(s string) *time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
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
func (s *Store) UpsertDevice(ctx context.Context, d model.OneStepDevice) error {
	var link any
	if d.LinkedCarEFleetsID != nil {
		link = *d.LinkedCarEFleetsID
	}
	var pdi any
	if d.LinkedCarPDIID != nil {
		pdi = *d.LinkedCarPDIID
	}
	active := !d.Dead
	if !active {
		d.Dead = true
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var retired any
	if d.Dead {
		if d.RetiredAt != nil {
			retired = d.RetiredAt.UTC().Format(time.RFC3339)
		} else {
			retired = now
		}
	}
	_, err := s.exec(ctx, `INSERT INTO onestep_devices (
			factory_id, device_id, display_name, linked_car_efleets_id, linked_car_pdi_id,
			dead, active, retired_at, last_synced_at, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (factory_id) DO UPDATE SET
			device_id=excluded.device_id,
			display_name=excluded.display_name,
			linked_car_efleets_id=COALESCE(excluded.linked_car_efleets_id, onestep_devices.linked_car_efleets_id),
			linked_car_pdi_id=COALESCE(excluded.linked_car_pdi_id, onestep_devices.linked_car_pdi_id),
			dead=excluded.dead,
			active=excluded.active,
			retired_at=excluded.retired_at,
			last_synced_at=excluded.last_synced_at,
			updated_at=excluded.updated_at`,
		d.FactoryID, d.DeviceID, d.DisplayName, link, pdi,
		d.Dead, active, retired, now, now, now)
	return err
}

// GetDevice returns one box by factory_id (the only join key).
func (s *Store) GetDevice(ctx context.Context, factoryID string) (*model.OneStepDevice, error) {
	row := s.queryRow(ctx, `SELECT factory_id, device_id, COALESCE(display_name,''), linked_car_efleets_id, linked_car_pdi_id,
		dead, active, retired_at, last_synced_at, created_at, updated_at
		FROM onestep_devices WHERE factory_id=?`, factoryID)
	d, err := scanDevice(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDevices returns every OneStep box (including retired).
func (s *Store) ListDevices(ctx context.Context) ([]model.OneStepDevice, error) {
	rows, err := s.query(ctx, `SELECT factory_id, device_id, COALESCE(display_name,''), linked_car_efleets_id, linked_car_pdi_id,
		dead, active, retired_at, last_synced_at, created_at, updated_at
		FROM onestep_devices ORDER BY factory_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.OneStepDevice
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDevicesForCar returns boxes linked by efleets_id, including dead ones so compute can ignore them.
func (s *Store) ListDevicesForCar(ctx context.Context, efleetsID string) ([]model.OneStepDevice, error) {
	rows, err := s.query(ctx, `SELECT factory_id, device_id, COALESCE(display_name,''), linked_car_efleets_id, linked_car_pdi_id,
		dead, active, retired_at, last_synced_at, created_at, updated_at
		FROM onestep_devices WHERE linked_car_efleets_id=?`, efleetsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.OneStepDevice
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type deviceScanner interface {
	Scan(dest ...any) error
}

func scanDevice(row deviceScanner) (model.OneStepDevice, error) {
	var d model.OneStepDevice
	var link, pdi, retired, synced, created, updated sql.NullString
	var dead, active bool
	if err := row.Scan(&d.FactoryID, &d.DeviceID, &d.DisplayName, &link, &pdi, &dead, &active, &retired, &synced, &created, &updated); err != nil {
		return d, err
	}
	d.LinkedCarEFleetsID = scanStrPtr(link)
	d.LinkedCarPDIID = scanStrPtr(pdi)
	d.Dead = dead
	d.Active = active
	d.RetiredAt = scanTimePtr(retired)
	d.LastSyncedAt = scanTimePtr(synced)
	if created.Valid {
		if t := parseStoreTime(created.String); t != nil {
			d.CreatedAt = *t
		}
	}
	if updated.Valid {
		if t := parseStoreTime(updated.String); t != nil {
			d.UpdatedAt = *t
		}
	}
	return d, nil
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
func (s *Store) WriteLastReading(ctx context.Context, efleetsID string, miles int, at time.Time, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, s.pg(`UPDATE cars SET last_reading_miles=?, last_reading_at=?, last_reading_source=?, hold_reason=NULL, updated_at=? WHERE efleets_id=?`),
		miles, at.UTC().Format(time.RFC3339), source, now, efleetsID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.pg(`UPDATE hold_events SET open=FALSE WHERE efleets_id=? AND open=TRUE`), efleetsID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetHold skips Last Reading. Prior last_reading_* stay put so operators do not trust a flagged number.
func (s *Store) SetHold(ctx context.Context, efleetsID, reason, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, s.pg(`UPDATE cars SET hold_reason=?, updated_at=? WHERE efleets_id=?`), reason, now, efleetsID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.pg(`UPDATE hold_events SET open=FALSE WHERE efleets_id=? AND open=TRUE`), efleetsID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.pg(`INSERT INTO hold_events (efleets_id, reason, detail, at, open) VALUES (?,?,?,?,?)`),
		efleetsID, reason, detail, now, true); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearHolds closes open events after a successful write.
func (s *Store) ClearHolds(ctx context.Context, efleetsID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.exec(ctx, `UPDATE hold_events SET open=FALSE WHERE efleets_id=? AND open=TRUE`, efleetsID)
	return err
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
// cars.last_oil_* only moves forward (later date, or same date with higher miles).
func (s *Store) InsertOilChange(ctx context.Context, o model.OilChange) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.exec(ctx, `INSERT INTO oil_changes (efleets_id, miles, date, location, source) VALUES (?,?,?,?,?)`,
		o.EFleetsID, o.Miles, o.Date.Format("2006-01-02"), o.Location, o.Source); err != nil {
		return err
	}
	c, err := s.carByEFleetsLocked(ctx, o.EFleetsID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if c.LastOilDate != nil {
		if c.LastOilDate.After(o.Date) {
			return nil
		}
		if c.LastOilDate.Equal(o.Date) && c.LastOilMiles != nil && *c.LastOilMiles >= o.Miles {
			return nil
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.exec(ctx, `UPDATE cars SET last_oil_miles=?, last_oil_date=?, updated_at=? WHERE efleets_id=?`,
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

// UpsertCardTx appends one swipe. UNIQUE on card+time+car+odo so rebuild is idempotent.
func (s *Store) UpsertCardTx(ctx context.Context, t model.CardTx) error {
	var odo any
	if t.Odometer != nil {
		odo = *t.Odometer
	} else {
		odo = 0
	}
	var gal, amt any
	if t.Gallons != nil {
		gal = *t.Gallons
	}
	if t.Amount != nil {
		amt = *t.Amount
	}
	_, err := s.exec(ctx, `INSERT INTO card_transactions
		(card_id, at, station_name, station_address, gallons, amount, recorded_efleets_id, recorded_cvn, plate, driver_first, driver_last, source_row, odometer)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (card_id, at, recorded_efleets_id, odometer) DO UPDATE SET
		  station_name=excluded.station_name, gallons=excluded.gallons, amount=excluded.amount,
		  recorded_cvn=excluded.recorded_cvn, plate=excluded.plate, driver_first=excluded.driver_first, driver_last=excluded.driver_last`,
		t.CardID, t.At.UTC().Format(time.RFC3339), t.StationName, t.StationAddress, gal, amt,
		t.RecordedEFleetsID, t.RecordedCVN, t.Plate, t.DriverFirst, t.DriverLast, t.SourceRow, odo)
	return err
}

// ListCardTxs returns swipes, optionally filtered by card_id (empty = all).
func (s *Store) ListCardTxs(ctx context.Context, cardID string) ([]model.CardTx, error) {
	q := `SELECT card_id, at, COALESCE(station_name,''), COALESCE(station_address,''), gallons, amount,
		COALESCE(recorded_efleets_id,''), COALESCE(recorded_cvn,''), COALESCE(plate,''),
		COALESCE(driver_first,''), COALESCE(driver_last,''), COALESCE(source_row,''), odometer
		FROM card_transactions`
	var args []any
	if cardID != "" {
		q += ` WHERE card_id=?`
		args = append(args, cardID)
	}
	q += ` ORDER BY at`
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CardTx
	for rows.Next() {
		var t model.CardTx
		var ts string
		var gal, amt sql.NullFloat64
		var odo sql.NullInt64
		if err := rows.Scan(&t.CardID, &ts, &t.StationName, &t.StationAddress, &gal, &amt,
			&t.RecordedEFleetsID, &t.RecordedCVN, &t.Plate, &t.DriverFirst, &t.DriverLast, &t.SourceRow, &odo); err != nil {
			return nil, err
		}
		if at, err := time.Parse(time.RFC3339, ts); err == nil {
			t.At = at
		}
		if gal.Valid {
			t.Gallons = &gal.Float64
		}
		if amt.Valid {
			t.Amount = &amt.Float64
		}
		if odo.Valid && odo.Int64 != 0 {
			t.Odometer = scanIntPtr(odo)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ReplacePairings writes scored links. Each card in the batch is deleted then re-inserted.
func (s *Store) ReplacePairings(ctx context.Context, rows []model.CardPairing) error {
	seen := map[string]struct{}{}
	for _, p := range rows {
		if _, ok := seen[p.CardID]; !ok {
			if _, err := s.exec(ctx, `DELETE FROM card_pairings WHERE card_id=?`, p.CardID); err != nil {
				return err
			}
			seen[p.CardID] = struct{}{}
		}
		best := 0
		if p.Best {
			best = 1
		}
		if _, err := s.exec(ctx, `INSERT INTO card_pairings (card_id, entity_type, entity_key, evidence_n, score, best)
			VALUES (?,?,?,?,?,?)`, p.CardID, p.EntityType, p.EntityKey, p.EvidenceN, p.Score, best); err != nil {
			return err
		}
	}
	return nil
}

// ListPairings returns stored scores. Empty cardID lists every card.
func (s *Store) ListPairings(ctx context.Context, cardID string) ([]model.CardPairing, error) {
	q := `SELECT card_id, entity_type, entity_key, evidence_n, score, best FROM card_pairings`
	var args []any
	if cardID != "" {
		q += ` WHERE card_id=?`
		args = append(args, cardID)
	}
	q += ` ORDER BY card_id, entity_type, score DESC`
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CardPairing
	for rows.Next() {
		var p model.CardPairing
		var best int
		if err := rows.Scan(&p.CardID, &p.EntityType, &p.EntityKey, &p.EvidenceN, &p.Score, &best); err != nil {
			return nil, err
		}
		p.Best = best != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListCards returns every fuel card row (Enterprise last-write-wins link plus notes).
func (s *Store) ListCards(ctx context.Context) ([]model.Card, error) {
	rows, err := s.query(ctx, `SELECT id, company_vehicle_number, linked_car_efleets_id, COALESCE(notes,'') FROM cards ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Card
	for rows.Next() {
		var c model.Card
		var link sql.NullString
		if err := rows.Scan(&c.ID, &c.CompanyVehicleNumber, &link, &c.Notes); err != nil {
			return nil, err
		}
		c.LinkedCarEFleetsID = scanStrPtr(link)
		out = append(out, c)
	}
	return out, rows.Err()
}
