-- Canon Place lookup tables + extra places columns already live on Neon.
-- Additive only. Do NOT DROP places (Neon holds the catalog). Do not prefix
-- fleet_* here — that name is Supabase-only so reception/Users stay intact.
-- Safe to re-run: CREATE IF NOT EXISTS, ON CONFLICT DO NOTHING, duplicate columns ignored.

CREATE TABLE IF NOT EXISTS place_types (
  code TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  notes TEXT
);

CREATE TABLE IF NOT EXISTS place_brands (
  code TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS toptier_codes (
  code TEXT PRIMARY KEY,
  meaning TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS toptier_grades (
  code TEXT PRIMARY KEY,
  meaning TEXT NOT NULL
);

INSERT INTO place_types (code, slug, name) VALUES
  ('001', 'gas', 'Gas / fuel station'),
  ('002', 'shop', 'Maintenance / repair shop'),
  ('003', 'gas_shop', 'Fuel + service same site'),
  ('004', 'other', 'Other visit place'),
  ('005', 'parking', 'Parking lot / overnight park'),
  ('006', 'yard', 'Yard / depot / staging'),
  ('007', 'office', 'Office / HQ stop'),
  ('008', 'facility', 'Medical / customer facility')
ON CONFLICT (code) DO NOTHING;

INSERT INTO toptier_codes (code, meaning) VALUES
  ('A', 'Don''t know / not checked'),
  ('B', 'False — not Top Tier'),
  ('C', 'True — Top Tier')
ON CONFLICT (code) DO NOTHING;

INSERT INTO toptier_grades (code, meaning) VALUES
  ('A', 'Don''t know'),
  ('B', 'Basic — TOP TIER (base)'),
  ('C', 'Top Tier Plus — TOP TIER+')
ON CONFLICT (code) DO NOTHING;

INSERT INTO place_brands (code, name) VALUES
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

-- sqlite 007 already has marker/hold/source. Neon catalog has city/state/site_key/etc.
-- ADD both directions; duplicate-column is ignored on reopen.
ALTER TABLE places ADD COLUMN city TEXT;
ALTER TABLE places ADD COLUMN state TEXT;
ALTER TABLE places ADD COLUMN merchant_location TEXT;
ALTER TABLE places ADD COLUMN toptier_source TEXT;
ALTER TABLE places ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE places ADD COLUMN visit_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE places ADD COLUMN site_key TEXT;
ALTER TABLE places ADD COLUMN notes TEXT;
ALTER TABLE places ADD COLUMN onestep_marker_id TEXT;
ALTER TABLE places ADD COLUMN hold_reason TEXT;
ALTER TABLE places ADD COLUMN source TEXT;

CREATE INDEX IF NOT EXISTS places_brand_idx ON places (brand_code);
CREATE INDEX IF NOT EXISTS places_site_key_idx ON places (site_key);
CREATE INDEX IF NOT EXISTS places_state_idx ON places (state);
