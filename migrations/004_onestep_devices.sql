-- Durable OneStep device registry enrichment (fleet-oil).
-- factory_id is the join key; device_id is history identity; display_name is label only.
-- Additive ALTER so existing sqlite/pg installs keep data. Safe to re-run (dup columns ignored).

ALTER TABLE onestep_devices ADD COLUMN linked_car_pdi_id TEXT;
ALTER TABLE onestep_devices ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE onestep_devices ADD COLUMN retired_at TIMESTAMPTZ;
ALTER TABLE onestep_devices ADD COLUMN last_synced_at TIMESTAMPTZ;
ALTER TABLE onestep_devices ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE onestep_devices ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE onestep_devices SET active = false WHERE dead = true;
UPDATE onestep_devices SET retired_at = COALESCE(retired_at, now()) WHERE dead = true AND retired_at IS NULL;
