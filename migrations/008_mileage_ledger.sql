-- Gas card transaction vs maintenance+drive-stop box score.
-- Does not write Last Reading. Suspect/HOLD rows stay visible; only trusted rows trend.

CREATE TABLE IF NOT EXISTS mileage_ledger (
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

CREATE TABLE IF NOT EXISTS drive_stop_windows (
  factory_id TEXT NOT NULL,
  from_at TIMESTAMPTZ NOT NULL,
  to_at TIMESTAMPTZ NOT NULL,
  miles DOUBLE PRECISION NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (factory_id, from_at, to_at)
);
