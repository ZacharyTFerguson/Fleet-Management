-- Shared-project schema for ZacharyTFerguson's Project (hdtwfdjdvdzdxfdriyzn).
-- Prefixed fleet_* so reception (cd_reception_records) and Users stay untouched.
-- Never apply to XRAY (chjqcznyxvtjbamttqdj).
-- Applied remotely via Supabase MCP apply_migration (fleet_oil_schema / fleet_oil_rls).

CREATE TABLE IF NOT EXISTS fleet_cars (
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

CREATE TABLE IF NOT EXISTS fleet_cards (
  id TEXT PRIMARY KEY,
  company_vehicle_number TEXT NOT NULL,
  linked_car_efleets_id TEXT REFERENCES fleet_cars(efleets_id),
  notes TEXT
);

CREATE TABLE IF NOT EXISTS fleet_gas_stations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL, address TEXT, merchant_id TEXT, lat DOUBLE PRECISION, lng DOUBLE PRECISION
);

CREATE TABLE IF NOT EXISTS fleet_maintenance_locations (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, address TEXT, notes TEXT
);

CREATE TABLE IF NOT EXISTS fleet_fills (
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

CREATE TABLE IF NOT EXISTS fleet_shop_ros (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  odometer INTEGER NOT NULL,
  at TIMESTAMPTZ NOT NULL,
  location_name TEXT,
  ro_id TEXT,
  service_desc TEXT,
  UNIQUE (efleets_id, at, odometer)
);

CREATE TABLE IF NOT EXISTS fleet_onestep_devices (
  factory_id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  display_name TEXT,
  linked_car_efleets_id TEXT REFERENCES fleet_cars(efleets_id),
  linked_car_pdi_id TEXT,
  dead BOOLEAN NOT NULL DEFAULT false,
  active BOOLEAN NOT NULL DEFAULT true,
  retired_at TIMESTAMPTZ,
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_drive_stop_miles (
  factory_id TEXT NOT NULL,
  since_at TIMESTAMPTZ NOT NULL,
  miles DOUBLE PRECISION NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (factory_id, since_at)
);

CREATE TABLE IF NOT EXISTS fleet_hold_events (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  detail TEXT,
  at TIMESTAMPTZ NOT NULL DEFAULT now(),
  open BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS fleet_oil_changes (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  miles INTEGER NOT NULL,
  date DATE NOT NULL,
  location TEXT,
  source TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS fleet_card_transactions (
  id BIGSERIAL PRIMARY KEY,
  card_id TEXT NOT NULL,
  at TIMESTAMPTZ NOT NULL,
  station_name TEXT,
  station_address TEXT,
  gallons DOUBLE PRECISION,
  amount DOUBLE PRECISION,
  recorded_efleets_id TEXT,
  recorded_cvn TEXT,
  plate TEXT,
  driver_first TEXT,
  driver_last TEXT,
  source_row TEXT,
  odometer INTEGER,
  UNIQUE (card_id, at, recorded_efleets_id, odometer)
);

CREATE TABLE IF NOT EXISTS fleet_card_pairings (
  card_id TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_key TEXT NOT NULL,
  evidence_n INTEGER NOT NULL,
  score DOUBLE PRECISION NOT NULL,
  best INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (card_id, entity_type, entity_key)
);

ALTER TABLE fleet_cars ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_cards ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_gas_stations ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_maintenance_locations ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_fills ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_shop_ros ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_onestep_devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_drive_stop_miles ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_hold_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_oil_changes ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_card_transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_card_pairings ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS fleet_cars_select_anon ON fleet_cars;
DROP POLICY IF EXISTS fleet_cars_select_authenticated ON fleet_cars;
CREATE POLICY fleet_cars_select_anon ON fleet_cars FOR SELECT TO anon USING (true);
CREATE POLICY fleet_cars_select_authenticated ON fleet_cars FOR SELECT TO authenticated USING (true);

DROP POLICY IF EXISTS deny_anon_fleet_cards ON fleet_cards;
DROP POLICY IF EXISTS deny_authenticated_fleet_cards ON fleet_cards;
CREATE POLICY deny_anon_fleet_cards ON fleet_cards FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_cards ON fleet_cards FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_gas ON fleet_gas_stations;
DROP POLICY IF EXISTS deny_authenticated_fleet_gas ON fleet_gas_stations;
CREATE POLICY deny_anon_fleet_gas ON fleet_gas_stations FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_gas ON fleet_gas_stations FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_ml ON fleet_maintenance_locations;
DROP POLICY IF EXISTS deny_authenticated_fleet_ml ON fleet_maintenance_locations;
CREATE POLICY deny_anon_fleet_ml ON fleet_maintenance_locations FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_ml ON fleet_maintenance_locations FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_fills ON fleet_fills;
DROP POLICY IF EXISTS deny_authenticated_fleet_fills ON fleet_fills;
CREATE POLICY deny_anon_fleet_fills ON fleet_fills FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_fills ON fleet_fills FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_ros ON fleet_shop_ros;
DROP POLICY IF EXISTS deny_authenticated_fleet_ros ON fleet_shop_ros;
CREATE POLICY deny_anon_fleet_ros ON fleet_shop_ros FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_ros ON fleet_shop_ros FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_dev ON fleet_onestep_devices;
DROP POLICY IF EXISTS deny_authenticated_fleet_dev ON fleet_onestep_devices;
CREATE POLICY deny_anon_fleet_dev ON fleet_onestep_devices FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_dev ON fleet_onestep_devices FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_dsm ON fleet_drive_stop_miles;
DROP POLICY IF EXISTS deny_authenticated_fleet_dsm ON fleet_drive_stop_miles;
CREATE POLICY deny_anon_fleet_dsm ON fleet_drive_stop_miles FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_dsm ON fleet_drive_stop_miles FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_hold ON fleet_hold_events;
DROP POLICY IF EXISTS deny_authenticated_fleet_hold ON fleet_hold_events;
CREATE POLICY deny_anon_fleet_hold ON fleet_hold_events FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_hold ON fleet_hold_events FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_oc ON fleet_oil_changes;
DROP POLICY IF EXISTS deny_authenticated_fleet_oc ON fleet_oil_changes;
CREATE POLICY deny_anon_fleet_oc ON fleet_oil_changes FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_oc ON fleet_oil_changes FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_cardtx ON fleet_card_transactions;
DROP POLICY IF EXISTS deny_authenticated_fleet_cardtx ON fleet_card_transactions;
CREATE POLICY deny_anon_fleet_cardtx ON fleet_card_transactions FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_cardtx ON fleet_card_transactions FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_cardpair ON fleet_card_pairings;
DROP POLICY IF EXISTS deny_authenticated_fleet_cardpair ON fleet_card_pairings;
CREATE POLICY deny_anon_fleet_cardpair ON fleet_card_pairings FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_cardpair ON fleet_card_pairings FOR ALL TO authenticated USING (false);
