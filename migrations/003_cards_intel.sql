-- Card intelligence: every swipe, not last-write-wins on cards.linked_car_*.
-- Ready for a dedicated fleet-oil project. Do not apply to XRAY.

CREATE TABLE IF NOT EXISTS card_transactions (
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

CREATE TABLE IF NOT EXISTS card_pairings (
  card_id TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_key TEXT NOT NULL,
  evidence_n INTEGER NOT NULL,
  score DOUBLE PRECISION NOT NULL,
  best INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (card_id, entity_type, entity_key)
);
