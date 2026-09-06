-- Desk login, encrypted secret vault, gas-station Canon places, and marker review jobs.
-- vault_secrets stays on the sqlite working store (never Neon, never Supabase).
-- places / marker jobs are gas stations only (type_code 001).

CREATE TABLE IF NOT EXISTS desk_users (
  username TEXT PRIMARY KEY,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS vault_secrets (
  key TEXT PRIMARY KEY,
  nonce BYTEA NOT NULL,
  ciphertext BYTEA NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS places (
  general_code TEXT PRIMARY KEY,
  type_code TEXT NOT NULL,
  brand_code TEXT NOT NULL,
  toptier TEXT NOT NULL DEFAULT 'A',
  toptier_grade TEXT NOT NULL DEFAULT 'A',
  label TEXT NOT NULL,
  name TEXT,
  address TEXT,
  merchant_id TEXT,
  lat DOUBLE PRECISION,
  lng DOUBLE PRECISION,
  onestep_marker_id TEXT,
  onestep_zone_id TEXT,
  hold_reason TEXT,
  source TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gas_station_marker_jobs (
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

CREATE TABLE IF NOT EXISTS desk_heartbeats (
  id BIGSERIAL PRIMARY KEY,
  target TEXT NOT NULL,
  ok BOOLEAN NOT NULL DEFAULT false,
  detail TEXT,
  at TIMESTAMPTZ NOT NULL DEFAULT now()
);
