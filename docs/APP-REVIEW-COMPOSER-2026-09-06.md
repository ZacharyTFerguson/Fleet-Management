# Oil Desk / oilchange review — Composer 2026-09-06

Baseline: `main` after PR #38 (`TestNormalOilUpdateOpsContract`). Three review → improve → test → improve cycles.

## Invariants checked

| Rule | Status |
|------|--------|
| Last Reading = Enterprise odo @ second + measured drive-stop | Unchanged; `internal/oil/lastreading.go` only |
| Never invent miles; never OneStep odo as Last Reading | `sync-enterprise` still skips Last Reading |
| Last oil forward-only; older `oil-done` ignored | Covered by `TestNormalOilUpdateOpsContract` |
| HOLD `NO_DRIVESTOP` when miles missing | Covered by contract test + box score tests |
| Never write remaining/due columns | `export.Headers` + report contract test |
| No secrets / no PHI dumps in git | Demo `testdata/enterprise/*` only; `live_sep6/README.md` documents local paths |

## Operator data path

```
Downloads CSVs (Fleet Summary, Fuel DETAILS, Maintenance Detail)
        ↓
oilchange sync-enterprise [--vehicles …] [--fuel-details …] [--shop-ro …]
        ↓
sqlite: cars, fills, shop_ros, card_transactions, last_oil_*
        ↓
sync-onestep → compute (Last Reading / HOLD)
        ↓
sync --mirror web/data/cars.json  → Oil Desk
```

Full-dump ingest is tested in `TestSyncEnterpriseFullDumpIngest` and documented in `testdata/enterprise/live_sep6/README.md`.

---

## Cycle 1 — Transaction sort contract

### Review

- `ListCardTxs` returned oldest-first (`ORDER BY at`).
- History `BuildBoard` sorted **assigned** car fills oldest-first but **unassigned** tray newest-first — inconsistent for operators dragging fills.
- `cards.NewestFillsFirst` duplicated sort logic used only in the watch loop.

### Improve

- Added `internal/model/sort.go`: `CardTxLessDesc`, `SortCardTxsDesc`, `FillLessDesc`, `FillBlockLessDesc` — Provider Transaction Date+Time **desc**, stable ties on `card_id` / `tx_key`.
- `ListCardTxs` now `ORDER BY at DESC, card_id, recorded_efleets_id, odometer, source_row`.
- History board sorts assigned + unassigned fills the same way (newest first).
- `cards.NewestFillsFirst` delegates to `model.SortCardTxsDesc`.

### Tests

- `internal/model/sort_test.go`
- `internal/history/board_test.go` — `TestBuildBoardFillsNewestFirst`, `TestBuildBoardUnassignedNewestFirst`
- `internal/store/store_test.go` — `TestListCardTxsNewestFirst`

### Improve again

- Tie-break expectation aligned: same-second rows sort by ascending `card_id` (stable, deterministic).

---

## Cycle 2 — Enterprise ingest path

### Review

- Maintenance Detail often lands before Fleet Summary on the operator desktop.
- PR #37/38 fixed `reconcileLastOil` on roster import, but there was no standalone ingest contract test outside `TestNormalOilUpdateOpsContract`.
- `details_live.csv` / `fleetsummary_live.csv` exist for scale checks but are not committed as operator PHI.

### Improve

- `TestSyncEnterpriseFullDumpIngest`: maintenance-first → roster + DETAILS → asserts last oil reconciled, fills + `card_transactions` counts, no invented Last Reading, newest-first tx list.
- `testdata/enterprise/live_sep6/README.md` documents full-dump `sync-enterprise` flags for live-scale CSVs kept outside git.

### Tests

- `internal/app/enterprise_ingest_test.go`

### Improve again

- `CarByEFleets` on maintenance-only ingest correctly returns `sql.ErrNoRows` (no phantom roster row).

---

## Cycle 3 — Box score / ledger display order

### Review

- `ScorePunches` correctly walks punches **oldest-first** for trend math, but desk/API consumers saw chronological ASC rows after `RebuildBoxScore` / `ListBoxScore`.
- `ListLedger` returned `punch_at` ASC per car.

### Improve

- `oil.SortLedgerRowsDesc` + `LedgerRowLessDesc` for display lists.
- `RebuildBoxScore` and `listStoredBoxScore` sort rows newest-first before returning.
- `ListLedger` → `ORDER BY efleets_id, punch_at DESC, recorded_odo`.
- Fleet vehicles in stored box score sorted by `efleets_id` for stable desk paging.

### Tests

- `internal/oil/boxscore_test.go` — `TestSortLedgerRowsDescNewestFirst`
- `internal/app/boxscore_test.go` — `TestRebuildBoxScoreRowsNewestFirst`
- `internal/store/ledger_test.go` — `TestListLedgerNewestFirst`

### Improve again

- Trend scoring still uses chronological ASC inside `ScorePunches`; only presentation layers sort DESC.

---

## Commands

```bash
go test ./...
go test ./internal/app -run TestNormalOilUpdateOpsContract -v
go test ./internal/model -run Sort -v
go test ./internal/history -run Newest -v
```

## Files touched (summary)

| Area | Files |
|------|-------|
| Sort contract | `internal/model/sort.go`, `internal/history/board.go`, `internal/store/store.go`, `internal/cards/watchloop.go` |
| Enterprise ingest | `internal/app/enterprise_ingest_test.go`, `testdata/enterprise/live_sep6/README.md` |
| Box score display | `internal/oil/boxscore.go`, `internal/app/boxscore.go`, `internal/store/ledger.go` |
| Tests | `*_test.go` in model, history, store, app, oil |
