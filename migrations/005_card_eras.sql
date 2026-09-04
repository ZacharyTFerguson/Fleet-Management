-- GPS / station-ladder history of where a gas card sat (car eras, driver-kept
-- person eras, office cards). Never Last Reading. Safe to re-run.

CREATE TABLE IF NOT EXISTS card_eras (
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
