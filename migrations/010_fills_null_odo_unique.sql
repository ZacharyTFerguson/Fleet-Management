-- Punches with no Provider Odometer (EV charging, provider omissions) escape
-- UNIQUE (efleets_id, provider_transaction_time, odometer): NULL never
-- conflicts, so overlapping 90d/12mo dumps duplicated them on every
-- sync-enterprise before the UpsertFill guard. Enforce it in the schema too:
-- first collapse any duplicates the old bug left behind, then add a partial
-- unique index so one (car, second) pair can hold at most one NULL-odo punch.

DELETE FROM fills
WHERE odometer IS NULL
  AND id NOT IN (
    SELECT MIN(id) FROM fills
    WHERE odometer IS NULL
    GROUP BY efleets_id, provider_transaction_time
  );

CREATE UNIQUE INDEX IF NOT EXISTS fills_one_null_odo_per_second
  ON fills (efleets_id, provider_transaction_time)
  WHERE odometer IS NULL;
