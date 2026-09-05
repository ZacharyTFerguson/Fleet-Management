-- Owner fill assignments. Separate from card_eras / pairings so cards rebuild
-- cannot wipe a drag. Never writes Last Reading. Enterprise Vehicle stays on
-- card_transactions.recorded_efleets_id.

CREATE TABLE IF NOT EXISTS transaction_assignments (
  tx_key TEXT PRIMARY KEY,
  assigned_efleets_id TEXT,
  assigned_pdi_id TEXT,
  source TEXT NOT NULL DEFAULT '',
  gps_called_efleets_id TEXT,
  gps_disagrees INTEGER NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS assignment_events (
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

CREATE INDEX IF NOT EXISTS assignment_events_tx_key ON assignment_events (tx_key, at);
