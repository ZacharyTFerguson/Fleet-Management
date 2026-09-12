-- Shared ZacharyTFerguson's Project (hdtwfdjdvdzdxfdriyzn).
-- Full durable fleet_* copy schema so Oil Desk still has a backup if Neon is down.
-- Prefixed so reception (cd_reception_records) and Users stay untouched.
-- Never apply to XRAY (chjqcznyxvtjbamttqdj).
-- Never create fleet_vault_secrets / fleet_desk_users (sqlite working store only).
-- Anon SELECT stays on fleet_cars only. Service role writes the rest.

CREATE TABLE IF NOT EXISTS fleet_place_types (
  code TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  notes TEXT
);

CREATE TABLE IF NOT EXISTS fleet_place_brands (
  code TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_toptier_codes (
  code TEXT PRIMARY KEY,
  meaning TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS fleet_toptier_grades (
  code TEXT PRIMARY KEY,
  meaning TEXT NOT NULL
);

INSERT INTO fleet_place_types (code, slug, name) VALUES
  ('001', 'gas', 'Gas / fuel station'),
  ('002', 'shop', 'Maintenance / repair shop'),
  ('003', 'gas_shop', 'Fuel + service same site'),
  ('004', 'other', 'Other visit place'),
  ('005', 'parking', 'Parking lot / overnight park'),
  ('006', 'yard', 'Yard / depot / staging'),
  ('007', 'office', 'Office / HQ stop'),
  ('008', 'facility', 'Medical / customer facility')
ON CONFLICT (code) DO NOTHING;

INSERT INTO fleet_toptier_codes (code, meaning) VALUES
  ('A', 'Don''t know / not checked'),
  ('B', 'False — not Top Tier'),
  ('C', 'True — Top Tier')
ON CONFLICT (code) DO NOTHING;

INSERT INTO fleet_toptier_grades (code, meaning) VALUES
  ('A', 'Don''t know'),
  ('B', 'Basic — TOP TIER (base)'),
  ('C', 'Top Tier Plus — TOP TIER+')
ON CONFLICT (code) DO NOTHING;

INSERT INTO fleet_place_brands (code, name) VALUES
  ('7ELEV', '7-Eleven'),
  ('ALBRT', 'Albertsons'),
  ('ALONX', 'Alon'),
  ('ARCOX', 'ARCO'),
  ('BPUSA', 'BP'),
  ('CASEY', 'Casey''s'),
  ('CENEX', 'Cenex'),
  ('CHEVR', 'Chevron'),
  ('CIRCL', 'Circle K'),
  ('CITGO', 'Citgo'),
  ('CONOC', 'Conoco'),
  ('COSTC', 'Costco'),
  ('CUMBF', 'Cumberland Farms'),
  ('EXXON', 'Exxon'),
  ('GETGO', 'GetGo'),
  ('GIANT', 'Giant'),
  ('GLOBL', 'Global'),
  ('GULFX', 'Gulf'),
  ('HESSS', 'Hess'),
  ('IRVIN', 'Irving'),
  ('KROGR', 'Kroger'),
  ('KUMGO', 'Kum & Go'),
  ('KWIKF', 'Kwik Fill'),
  ('KWIKT', 'Kwik Trip'),
  ('LOVES', 'Love''s'),
  ('LUKOI', 'Lukoil'),
  ('MARTH', 'Marathon'),
  ('MAVER', 'Maverik'),
  ('MOBIL', 'Mobil'),
  ('MURPH', 'Murphy'),
  ('PHIL6', 'Phillips 66'),
  ('PILOT', 'Pilot'),
  ('QCHEK', 'QuickChek'),
  ('QTRAC', 'QuikTrip'),
  ('RACET', 'RaceTrac'),
  ('RACEW', 'Raceway'),
  ('ROYAL', 'Royal Farms'),
  ('RUTTR', 'Rutter''s'),
  ('SAMSC', 'Sam''s Club'),
  ('SEV76', '76'),
  ('SHELL', 'Shell'),
  ('SHETZ', 'Sheetz'),
  ('SINCL', 'Sinclair'),
  ('SPEED', 'Speedway'),
  ('STEWT', 'Stewart''s'),
  ('SUNOC', 'Sunoco'),
  ('TACEN', 'TA / TravelCenters'),
  ('TESOR', 'Tesoro'),
  ('TEXAC', 'Texaco'),
  ('THINK', 'Think / independent'),
  ('THORN', 'Thorntons'),
  ('TURKH', 'Turkey Hill'),
  ('UNKWN', 'Unknown brand'),
  ('VALER', 'Valero'),
  ('WAWAA', 'Wawa'),
  ('WEISX', 'Weis')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS fleet_places (
  general_code TEXT PRIMARY KEY,
  type_code TEXT NOT NULL REFERENCES fleet_place_types(code),
  brand_code TEXT NOT NULL REFERENCES fleet_place_brands(code),
  toptier TEXT NOT NULL DEFAULT 'A' REFERENCES fleet_toptier_codes(code),
  toptier_grade TEXT NOT NULL DEFAULT 'A' REFERENCES fleet_toptier_grades(code),
  label TEXT NOT NULL,
  name TEXT,
  address TEXT,
  city TEXT,
  state TEXT,
  merchant_location TEXT,
  merchant_id TEXT,
  lat DOUBLE PRECISION,
  lng DOUBLE PRECISION,
  onestep_marker_id TEXT,
  onestep_zone_id TEXT,
  toptier_source TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  visit_count INTEGER NOT NULL DEFAULT 1,
  site_key TEXT,
  notes TEXT,
  hold_reason TEXT,
  source TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS fleet_places_brand_idx ON fleet_places (brand_code);
CREATE INDEX IF NOT EXISTS fleet_places_site_key_idx ON fleet_places (site_key);
CREATE INDEX IF NOT EXISTS fleet_places_state_idx ON fleet_places (state);

CREATE TABLE IF NOT EXISTS fleet_card_eras (
  card_id TEXT NOT NULL,
  holder_type TEXT NOT NULL,
  holder_key TEXT NOT NULL,
  efleets_id TEXT,
  nickname TEXT,
  from_at TIMESTAMPTZ NOT NULL,
  to_at TIMESTAMPTZ NOT NULL,
  evidence_n INTEGER NOT NULL,
  stations TEXT,
  split INTEGER NOT NULL DEFAULT 0,
  rung INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (card_id, holder_type, holder_key, from_at)
);

CREATE TABLE IF NOT EXISTS fleet_transaction_assignments (
  tx_key TEXT PRIMARY KEY,
  assigned_efleets_id TEXT,
  assigned_pdi_id TEXT,
  source TEXT NOT NULL DEFAULT '',
  gps_called_efleets_id TEXT,
  gps_disagrees INTEGER NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_assignment_events (
  id BIGSERIAL PRIMARY KEY,
  tx_key TEXT NOT NULL,
  from_efleets_id TEXT,
  to_efleets_id TEXT,
  from_pdi_id TEXT,
  to_pdi_id TEXT,
  actor TEXT NOT NULL DEFAULT 'owner',
  reason TEXT NOT NULL,
  at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS fleet_assignment_events_tx_key ON fleet_assignment_events (tx_key, at);

CREATE TABLE IF NOT EXISTS fleet_gas_station_marker_jobs (
  id TEXT PRIMARY KEY,
  general_code TEXT NOT NULL,
  stage TEXT NOT NULL,
  draft_json TEXT NOT NULL,
  review_notes TEXT,
  third_party_lat DOUBLE PRECISION,
  third_party_lng DOUBLE PRECISION,
  third_party_provider TEXT,
  onestep_lat DOUBLE PRECISION,
  onestep_lng DOUBLE PRECISION,
  confirm_token TEXT,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_mileage_ledger (
  id BIGSERIAL PRIMARY KEY,
  efleets_id TEXT NOT NULL,
  card_id TEXT,
  punch_at TIMESTAMPTZ NOT NULL,
  merchant TEXT,
  recorded_odo INTEGER NOT NULL,
  maint_odo INTEGER,
  maint_at TIMESTAMPTZ,
  miles_since DOUBLE PRECISION,
  expected_odo INTEGER,
  difference INTEGER,
  overage INTEGER NOT NULL DEFAULT 0,
  shortage INTEGER NOT NULL DEFAULT 0,
  abs_diff INTEGER,
  trend TEXT,
  status TEXT NOT NULL,
  hold_reason TEXT,
  hold_detail TEXT,
  in_trend BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (efleets_id, punch_at, recorded_odo)
);

CREATE TABLE IF NOT EXISTS fleet_drive_stop_windows (
  factory_id TEXT NOT NULL,
  from_at TIMESTAMPTZ NOT NULL,
  to_at TIMESTAMPTZ NOT NULL,
  miles DOUBLE PRECISION NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (factory_id, from_at, to_at)
);

CREATE UNIQUE INDEX IF NOT EXISTS fleet_fills_one_null_odo_per_second
  ON fleet_fills (efleets_id, provider_transaction_time)
  WHERE odometer IS NULL;

ALTER TABLE fleet_place_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_place_brands ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_toptier_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_toptier_grades ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_places ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_card_eras ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_transaction_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_assignment_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_gas_station_marker_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_mileage_ledger ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_drive_stop_windows ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS deny_anon_fleet_ptypes ON fleet_place_types;
DROP POLICY IF EXISTS deny_authenticated_fleet_ptypes ON fleet_place_types;
CREATE POLICY deny_anon_fleet_ptypes ON fleet_place_types FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_ptypes ON fleet_place_types FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_pbrands ON fleet_place_brands;
DROP POLICY IF EXISTS deny_authenticated_fleet_pbrands ON fleet_place_brands;
CREATE POLICY deny_anon_fleet_pbrands ON fleet_place_brands FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_pbrands ON fleet_place_brands FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_ttcodes ON fleet_toptier_codes;
DROP POLICY IF EXISTS deny_authenticated_fleet_ttcodes ON fleet_toptier_codes;
CREATE POLICY deny_anon_fleet_ttcodes ON fleet_toptier_codes FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_ttcodes ON fleet_toptier_codes FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_ttgrades ON fleet_toptier_grades;
DROP POLICY IF EXISTS deny_authenticated_fleet_ttgrades ON fleet_toptier_grades;
CREATE POLICY deny_anon_fleet_ttgrades ON fleet_toptier_grades FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_ttgrades ON fleet_toptier_grades FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_places ON fleet_places;
DROP POLICY IF EXISTS deny_authenticated_fleet_places ON fleet_places;
CREATE POLICY deny_anon_fleet_places ON fleet_places FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_places ON fleet_places FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_eras ON fleet_card_eras;
DROP POLICY IF EXISTS deny_authenticated_fleet_eras ON fleet_card_eras;
CREATE POLICY deny_anon_fleet_eras ON fleet_card_eras FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_eras ON fleet_card_eras FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_asg ON fleet_transaction_assignments;
DROP POLICY IF EXISTS deny_authenticated_fleet_asg ON fleet_transaction_assignments;
CREATE POLICY deny_anon_fleet_asg ON fleet_transaction_assignments FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_asg ON fleet_transaction_assignments FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_asgev ON fleet_assignment_events;
DROP POLICY IF EXISTS deny_authenticated_fleet_asgev ON fleet_assignment_events;
CREATE POLICY deny_anon_fleet_asgev ON fleet_assignment_events FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_asgev ON fleet_assignment_events FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_marker ON fleet_gas_station_marker_jobs;
DROP POLICY IF EXISTS deny_authenticated_fleet_marker ON fleet_gas_station_marker_jobs;
CREATE POLICY deny_anon_fleet_marker ON fleet_gas_station_marker_jobs FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_marker ON fleet_gas_station_marker_jobs FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_ledger ON fleet_mileage_ledger;
DROP POLICY IF EXISTS deny_authenticated_fleet_ledger ON fleet_mileage_ledger;
CREATE POLICY deny_anon_fleet_ledger ON fleet_mileage_ledger FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_ledger ON fleet_mileage_ledger FOR ALL TO authenticated USING (false);

DROP POLICY IF EXISTS deny_anon_fleet_dsw ON fleet_drive_stop_windows;
DROP POLICY IF EXISTS deny_authenticated_fleet_dsw ON fleet_drive_stop_windows;
CREATE POLICY deny_anon_fleet_dsw ON fleet_drive_stop_windows FOR ALL TO anon USING (false);
CREATE POLICY deny_authenticated_fleet_dsw ON fleet_drive_stop_windows FOR ALL TO authenticated USING (false);

-- Oil Desk publish surface unchanged: anon/authenticated SELECT on fleet_cars only.
DROP POLICY IF EXISTS fleet_cars_select_anon ON fleet_cars;
DROP POLICY IF EXISTS fleet_cars_select_authenticated ON fleet_cars;
CREATE POLICY fleet_cars_select_anon ON fleet_cars FOR SELECT TO anon USING (true);
CREATE POLICY fleet_cars_select_authenticated ON fleet_cars FOR SELECT TO authenticated USING (true);
