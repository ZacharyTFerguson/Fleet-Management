# Oil Desk app review — 2026-09-06 (Fable)

Three review → improve → test → improve-again cycles over the enterprise-dump →
sqlite → Desk/report path. Baseline: `main` after PR #38
(`TestNormalOilUpdateOpsContract` green). Everything below shipped as code +
tests in the same branch as this document.

> **Reconciliation note (post-merge of PR #39):** this branch was rebased onto
> the merged Composer review (PR #39), which introduced `internal/model/sort.go`.
> The canonical newest-first order and its tie rule now live there (time DESC,
> higher odometer first with missing odometer last, then card id, then stable
> keys); the `history.SortBlocksNewestFirst` helper described in Cycle 2 was
> folded into that shared comparator, and `store.ListCardTxs`/`ListLedger`
> return newest-first to match. `docs/APP-REVIEW-COMPOSER-2026-09-06.md` covers
> the other half of the reconciliation.

## How the app is actually used (observed path)

1. **Dumps in.** An operator exports three eFleets reports — Fleet Summary,
   Fuel & Charging DETAILS, Maintenance Detail — and runs:

   ```
   oilchange sync-enterprise --vehicles fleetsummary.csv \
       --fuel-details details.csv --shop-ro maintenance.csv
   ```

   With `EFLEETS_*` secrets set and no flags, the same command pulls live.
   Parsing is header-name based (`internal/enterprise/parse.go`), never column
   position; rows land in sqlite (`cars`, `fills`, `card_transactions`,
   `shop_ros`, `gas_stations`, `cards`, `maintenance_locations`,
   `oil_changes`). Oil/lube shop ROs seed forward-only `cars.last_oil_*`.
   Ingest never computes Last Reading.

2. **Full 90d/12mo dump.** The same flags handle any window — eFleets caps one
   DETAILS export, so a 12-month load is several overlapping files run through
   `sync-enterprise` back to back (or one file per quarter). Because every
   upsert is idempotent (see Cycle 1), overlap between pulls is safe and the
   expected mode of operation. Order does not matter for fills/ROs; running
   `--vehicles` first (or together, as above) avoids the "skipped punches for
   vehicles not on the roster" path.

3. **GPS + compute.** `sync-onestep` fetches drive-stop miles-since after each
   car's trusted anchor second; `compute` is the only writer of
   `last_reading_*` (Enterprise odo at a known second + measured OneStep
   drive-stop; HOLD otherwise). `report` writes the lean CSV for the sheet.

4. **Desk.** `oilchange serve` (or `desk`) serves the embedded UI: History
   turnstile (region car columns + unassigned tray fed by
   `Store.ListCardTxs` → `history.BuildBoard`), Cards, Stations, Box score.

## Cycle 1 — enterprise dump ingest

**Findings**

- `UpsertFill` deduped on `UNIQUE (efleets_id, provider_transaction_time,
  odometer)` with `ON CONFLICT DO NOTHING`. NULL odometers never conflict in
  SQLite or Postgres, so every punch with a blank `Provider Odometer` (EV
  charging rows, provider omissions) **duplicated on every re-ingest**. With
  overlapping 90d/12mo dumps this compounds: three syncs of one file tripled
  those rows.
- `oil.IsOilChangeService` missed the common Firestone/Midas wording
  `Engine Oil & Filter Change` (needle list only had adjacent `oil change`),
  so those ROs never seeded `last_oil_*`.
- There were no fixtures matching the current live export headers for an
  end-to-end dump→sqlite test.

**Shipped**

- `testdata/enterprise/live_sep6/` — Fleet Summary + DETAILS + Maintenance
  fixtures with the real Sep 2026 export headers (fictional vehicles/data;
  no PHI). They include a no-odometer EV punch, same-second punches, an
  unusual-Y punch, an off-roster punch, a multi-line-item oil RO, and an open
  RO whose completed date is `-`.
- `UpsertFill` guards the NULL-odometer case with `NOT EXISTS ... AND odometer
  IS NULL` (portable across sqlite/pgx).
- `oil & filter` / `oil and filter` needles (surcharge and filter-only rows
  still excluded).
- `TestLiveSep6IngestLandsInSQLite` (internal/app): ingests the fixtures,
  asserts roster/fills/ROs/oil seeds and that ingest never invents Last
  Reading, then re-runs `sync-enterprise` twice and asserts not a single row
  was added.

## Cycle 2 — transaction ordering

**Findings (pre-existing bugs)**

- On one History page, **assigned car columns sorted oldest-first while the
  unassigned tray sorted newest-first** (`history.BuildBoard`); the frontend
  renders API order verbatim, so the two lanes read in opposite directions.
- No sort had tie-breakers. `ListCardTxs` (`ORDER BY at`), `ListFills`
  (`ORDER BY provider_transaction_time`), `ListShopROs` (`ORDER BY at`) return
  same-second rows in **undefined SQL order**, so board/tray positions of
  same-second punches could shuffle between reads.
- The cards watch path (`cards.NewestFillsFirst`) broke ties only by card id —
  a different rule than the board.

**Canonical order (now defined and implemented)**

- **Storage** returns deterministic chronological order: time ascending plus
  explicit tie-breakers (`odometer`, `card_id`, rowid) on `ListFills`,
  `ListShopROs`, `ListCardTxs`. Compute paths (`walkFills`, box score) keep
  walking ascending chains.
- **Display** is Provider Transaction Date+Time **descending** (newest first)
  with stable ties: higher odometer first (missing odometer last), then card
  id, then tx key. `history.SortBlocksNewestFirst` applies it to both car
  columns and the tray.

**Shipped tests** — `TestBuildBoardOrdersNewestFirstWithStableTies`
(input-order independent, ties locked) and
`TestListOrderDeterministicOnSameSecond` (storage contract).

## Cycle 3 — remaining sort sites + hardening

**Findings**

- `cards.NewestFillsFirst` (watch loop, `FillsForCard`) still used the old
  card-id-only tie rule, so a same-second pair could render in one order on
  the History board and the opposite order in the watch list.
- `oil.walkFills` sorted with non-stable `sort.Slice` on time only; equal-second
  fills could be walked in different orders across runs (conflicting
  same-second odos already HOLD `SAME_SECOND_FILL`, and nil-odo punches are
  skipped, so Last Reading was never wrong — but the walk order was not
  reproducible).
- `Store.ListLedger` ordered by `efleets_id, punch_at` only; `recorded_odo` is
  part of the ledger's unique key, so same-second box-score punches could swap
  between renders.

**Shipped**

- `NewestFillsFirst` now uses the canonical tie order (odometer desc, card id,
  tx key), locked by `TestNewestFillsFirstTiesMatchHistoryOrder`.
- `walkFills` uses `sort.SliceStable`, preserving the store's deterministic
  order.
- `ListLedger` adds the `recorded_odo` tie-breaker.

## Invariants checked each cycle (unchanged, still locked)

- Last Reading = Enterprise odo at a known second + measured OneStep
  drive-stop; ingest and Desk never invent miles; OneStep odometer is never a
  reading (`TestNormalOilUpdateOpsContract`, `TestLiveSep6IngestLandsInSQLite`).
- Last oil is forward-only; missing measured miles → HOLD `NO_DRIVESTOP`.
- The report never writes remaining/due columns.
- Fixtures are small and fictional; no secrets, no real PHI dumps in git.

## Remaining improvements (not shipped here)

- **History pagination/windowing.** `/api/history` loads every
  `card_transaction` ever ingested; a 12-month dump makes the board heavy.
  A `?since=` window (default 90d) with the same canonical order would keep
  the turnstile fast.
- **`fills.card_id` is not persisted.** `model.Fill.CardID` exists in memory
  only; `fills` dedupes on time+odo. Persisting it would let the fill picker
  explain "which card" without joining `card_transactions`.
- **`sync-enterprise` should report row counts** (parsed/upserted/skipped per
  file) so operators can eyeball a 90d load; today only the off-roster skip
  count prints.
- **Report row order** relies on `ListCars` (`ORDER BY efleets_id`), which is
  fine but undocumented in `export.WriteCSV`; a comment or explicit sort would
  prevent regressions if the car query ever changes.
- **`ParseMileage` is parse-and-discard** by design (context only); the CLI
  accepts `--mileage-history` but stores nothing — worth stating in `--help`
  text to avoid operator confusion.
