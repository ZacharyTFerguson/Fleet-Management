-- Covering indexes for Canon Place filters / FKs. Safe to re-run.

CREATE INDEX IF NOT EXISTS places_type_idx ON places (type_code);
