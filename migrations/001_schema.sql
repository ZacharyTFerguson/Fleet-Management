-- ready for Supabase project fleet-oil
-- Integer PKs except cars.pdi_id (opaque text) and onestep_devices.factory_id.

CREATE TABLE cars (
  pdi_id TEXT PRIMARY KEY,
  efleets_id TEXT NOT NULL UNIQUE,
  nickname TEXT, plate TEXT, vin TEXT, region TEXT,
  last_oil_miles INTEGER, last_oil_date TIMESTAMPTZ,
  last_reading_miles INTEGER, last_reading_at TIMESTAMPTZ,
  last_reading_source TEXT CHECK (last_reading_source IN ('fuel_details','shop_ro') OR last_reading_source IS NULL),
  hold_reason TEXT,
  interval_miles INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE cards (
  id TEXT PRIMARY KEY,
  company_vehicle_number TEXT NOT NULL,
  linked_car_efleets_id TEXT REFERENCES cars(efleets_id),
  notes TEXT
);

CREATE TABLE gas_stations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL, address TEXT, merchant_id TEXT, lat DOUBLE PRECISION, lng DOUBLE PRECISION
);

CREATE TABLE maintenance_locations (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, address TEXT, notes TEXT
);

CREATE TABLE fills (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  card_company_vehicle_number TEXT,
  odometer INTEGER,
  unusual_y BOOLEAN NOT NULL DEFAULT false,
  provider_transaction_time TIMESTAMPTZ NOT NULL,
  provider_company_vehicle_number TEXT,
  merchant_name TEXT, merchant_address TEXT,
  source TEXT NOT NULL DEFAULT 'fuel_details',
  UNIQUE (efleets_id, provider_transaction_time, odometer)
);

CREATE TABLE shop_ros (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  odometer INTEGER NOT NULL,
  at TIMESTAMPTZ NOT NULL,
  location_name TEXT,
  ro_id TEXT,
  service_desc TEXT,
  UNIQUE (efleets_id, at, odometer)
);

CREATE TABLE onestep_devices (
  factory_id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  display_name TEXT,
  linked_car_efleets_id TEXT REFERENCES cars(efleets_id),
  dead BOOLEAN NOT NULL DEFAULT false
);

-- GPS trip sum after a known second. Never stores device odometer. Ready for fleet-oil.
CREATE TABLE drive_stop_miles (
  factory_id TEXT NOT NULL,
  since_at TIMESTAMPTZ NOT NULL,
  miles DOUBLE PRECISION NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (factory_id, since_at)
);

CREATE TABLE hold_events (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  detail TEXT,
  at TIMESTAMPTZ NOT NULL DEFAULT now(),
  open BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE oil_changes (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  miles INTEGER NOT NULL,
  date DATE NOT NULL,
  location TEXT,
  source TEXT NOT NULL
);
