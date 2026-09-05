-- Read-only cache of Neon Fleet_Manage_Oil.places (Canon 7_3_5_1_1 labels).
-- Copy existing catalog rows only. Do not mint general_codes or reseed.

CREATE TABLE IF NOT EXISTS places (
  general_code TEXT PRIMARY KEY,
  type_code TEXT NOT NULL,
  brand_code TEXT NOT NULL,
  toptier TEXT NOT NULL DEFAULT 'A',
  toptier_grade TEXT NOT NULL DEFAULT 'A',
  label TEXT,
  name TEXT,
  address TEXT,
  city TEXT,
  state TEXT,
  merchant_location TEXT,
  merchant_id TEXT,
  site_key TEXT,
  active INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS places_site_key_idx ON places (site_key);
