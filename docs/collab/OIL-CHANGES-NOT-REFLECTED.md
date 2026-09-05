# Oil Desk: last oil not showing, and eFleets presort

Investigation on **`main`** of [ZacharyTFerguson/Fleet-Management](https://github.com/ZacharyTFerguson/Fleet-Management) (tip at write time: `4618cb7`, after PR #30). Whole-repo walk: `cmd/oilchange`, `internal/{app,enterprise,oil,store,desk,syncsupabase,export,history,config}`, Oil Desk (`web/src` + embedded `web/out`), schema (`migrations/`, `supabase/migrations/`), testdata, and collab docs.

This is a diagnosis, not a code fix. **Do not invent miles or oil dates.** Do not copy a Cloud Agent sqlite over the operator desktop db. Do not seed `last_oil_*` from Last Reading, fuel odo, or OneStep odometer.

---

## Short answers

### 1) Why oil changes are not reflected in Oil Desk

Oil Desk is a **read-only list** of `cars.json` (or Supabase `fleet_cars`). Last oil is a **separate column** from Last Reading. It is written only by:

- `oilchange sync-enterprise --shop-ro` (Maintenance Detail oil/lube lines), or
- `oilchange oil-done --efleets-id … --miles N --date YYYY-MM-DD`

Neither writes the UI surface. After sqlite changes you still need `oilchange sync [--mirror]`. The committed mirror `web/data/cars.json` is `source=mock-mirror`, **205 cars, 0 last oil, 0 Last Reading** (synced 2026-09-04). That is why the desk shows `—` for Last oil / Remaining even when the operator desktop sqlite already has shop ROs.

There is **no Oil Desk button** and **no `/api` write** for oil-done. The Automations Google Sheet is a different path; Desk never reads it. Live eFleets maintenance is file-drop only until a real CSV export URL (or CDP) exists — `EFLEETS_MAINT_URL` today is treated as the HTML tab.

### 2) Can we presort the roster the way eFleets All Cars / Automations does?

**Partially now; a true portal match needs a small build.**

| Want | Now | Needs building |
|---|---|---|
| Group / filter by unit prefix (VA / NJ / MD / …) | Region is already derived from nickname letters. Oil Desk search includes `region`. History already has a **region turnstile**. | Same turnstile (or grouped sections) on Oil Desk. Normalize `BING`/`Bing`, `NJ OCEAN`, `Office, was CT9`. |
| Same order as eFleets All Cars | Fleet Summary live CSV is *roughly* grouped by Customer Vehicle ID. Oil Desk re-sorts by **nickname** only. sqlite / Supabase list by **`efleets_id`**. | Persist Fleet Summary **row index** at ingest (`portal_order`). Sort Desk by that, not by Vehicle id. |
| Due-soon first | CLI `oilchange report --due-within N` filters in-app. UI already computes remaining miles when both last oil and Last Reading exist. | Optional due-soon sort on Desk. **Never write remaining/due columns.** Cars missing last oil or Last Reading stay at the end — do not invent miles to rank them. |
| Automations sheet order | Sheet is documented as a **OneStep factory_id map**, not a car roster. No Google Sheets client in this repo. | Do not treat the sheet as All Cars order unless a dedicated roster-order column is exported and parsed. |
| Live portal grid via Chrome / DETAILS | `ChromeSessionAdapter` is **docs only**. DETAILS is fuel punches, not roster order. | Capture Fleet Summary / All Cars export (file or CDP). DETAILS cannot drive a stable car presort. |

---

## How the pieces actually connect

```
eFleets Fleet Summary / Fuel DETAILS / Maintenance Detail
        │
        ▼
sync-enterprise   FileAdapter (any --vehicles/--fuel-details/--shop-ro)
                  or HTTPAdapter (live login; MFA fails closed)
        │
        ├─ ParseVehicles  → cars (nickname, region). Does not set last oil.
        ├─ ParseFills     → fills / cards. Skips punches whose Vehicle is not on the roster.
        └─ ParseShopROs   → shop_ros + oil_changes (IsOilChangeService only)
                            → cars.last_oil_*  (forward only; silent no-op if car missing)
        │
        ▼
SQLite OILCHANGE_DB     optional: oil-done  (source "oil-done")
                        optional: compute   (Last Reading / HOLD only — never last oil)
        │
        ▼
oilchange sync          always writes web/data/cars.json
                        upserts fleet_cars unless --no-remote
        │
        ▼
oilchange serve /api/cars   reads the JSON mirror only (not live sqlite)
Next /api/cars              Supabase fleet_cars if env set, else data/cars.json
        │
        ▼
CarsBoard   Last oil = last_oil_miles (date is unused)
            Remaining = interval − (last_reading − last_oil); both required
```

Cards / History / Devices desks do **not** write last oil. `cards *`, OneStep, and `compute` are locked off Last Reading invention; they also do not seed last oil.

---

## Question 1 — evidence

### Last oil is not computed from miles

`internal/oil` owns Last Reading only:

```11:26:internal/oil/lastreading.go
// LastReading is the only place Last Reading miles are computed.
func LastReading(enterpriseOdo int, fillTime time.Time, milesSince float64) (reading int, holds []model.Hold, err error) {
```

Last oil comes from maintenance text, not fuel or GPS:

```5:6:internal/oil/oilservice.go
// Last oil comes from these maintenance records, not from fuel punches or OneStep.
```

Needles: `lube oil filter`, `synthetic engine oil`, `oil change`, `conventional lube oil`, … Excludes: oil pan, oil leak, filter surcharge, chassis lube, drain plug, etc. Tests lock `R/R OIL PAN` as not an oil change (`internal/oil/oil_test.go`, `internal/enterprise/parse_test.go`).

`compute` (`internal/app/app.go`) writes `last_reading_*` or HOLD. It never writes `last_oil_*`. Shop ROs used inside `EvaluateHolds` are a Last Reading **anchor**, not a last-oil seed.

### Only two writers

| Writer | Source string | What happens |
|---|---|---|
| `ParseShopROs` → `InsertOilChange` | `shop_ro` | Oil/lube line with a date. Completed `-` (Under Shop Review) uses **RO Created Date**. |
| `oilchange oil-done` | `oil-done` | Operator miles + date. Must not invent either. |

`InsertOilChange` (`internal/store/store.go`):

1. Always inserts an `oil_changes` row.
2. If no `cars` row for that `efleets_id` → **returns nil** (oil log exists, desk column stays empty).
3. If a last oil already exists, only a **later date** (or same date + higher miles) advances `cars.last_oil_*`.
4. Never touches Last Reading (`TestInsertOilChangeAdvancesLastOilNotLastReading`, `TestOilDoneDoesNotChangeLastReading`).

`UpsertCar` on a re-sync updates nickname / plate / VIN / region only. It does not wipe last oil.

### Desk never sees sqlite

`oilchange serve` `/api/cars` is `serveCars` → `readMirror(cars.json)` (`internal/desk/desk.go`). History APIs talk to sqlite; the **Oil** list does not.

`oilchange sync` (`internal/app/sync_supabase.go`) is the only refresh of that mirror. `--no-remote` still writes JSON (`TestRemoteFailureStillRefreshesMirror`) but does not upsert `fleet_cars`. Next.js `loadFleetSnapshot` prefers live `fleet_cars` when publishable env is set (`web/src/lib/fleet.ts`), then falls back to `data/cars.json`.

**Committed `web/data/cars.json` (main):** 205 cars, `source=mock-mirror`, every `last_oil_miles` and `last_reading_miles` is `null`. CarsBoard renders null as `—`. Remaining is also `—` because `remainingMiles` requires both columns (`web/src/lib/types.ts`).

`docs/collab/STATUS.md` (2026-09-04) says the **operator desktop** sqlite had **189 / 205** last oil from real Maintenance Detail drops, and **146** Last Reading. The same STATUS says the Cloud Agent sqlite / `--no-remote` wave **does not hold those values**. The committed mirror matches the empty agent path, not the desktop.

### Maintenance ingest is thin (file-drop)

Live shop pull (`internal/enterprise/adapter.go`):

- `HTTPAdapter.urlFor(ReportShopRO)` requires `EFLEETS_MAINT_URL`.
- There is no guessed portal path (wrong CSV would invent miles).
- `oilchange env` flags URLs containing `maintenanceTab=` / `fuelTab=` / `/dashboard` as **"portal page, not a CSV export"**.
- Login posts `username` / `j_username`. Portal field is `userId`. MFA / JS wall **fails closed**. No Playwright.
- `ChromeSessionAdapter` is specified in `docs/CHROME-SESSION-ENTERPRISE.md` and **is not implemented** on main.

Any `--vehicles` / `--fuel-details` / `--shop-ro` flag selects **FileAdapter** and disables live fetch for the other reports (`adapter()` in `internal/app/app.go`). So:

```text
sync-enterprise --vehicles fleetsummary_live.csv --fuel-details details_live.csv
```

loads 205 cars and **skips last oil entirely** unless `--shop-ro` is also passed.

README’s live-roster Windows/Linux example pairs `fleetsummary_live.csv` with **`testdata/enterprise/maintenance.csv`**. That maintenance file is the **2-car demo** (`27TESTA`, `27TESTB`). Those Vehicle ids are **not** in the 205-car live summary. Result: oil_changes rows for testdata cars; **zero** `cars.last_oil_*` on the live roster.

DETAILS punches for unknown vehicles are skipped with a stderr count. Shop RO ingest does **not** skip unknown Vehicle ids the same way — it writes `oil_changes` / `shop_ros` and then silently fails the `cars` update. Testdata CVI `VA19` / `VA20` look like live nicknames; the join key is still **`Vehicle`**, not Customer Vehicle ID.

There is **no `testdata/enterprise/maintenance_live.csv`**. Real 12-month / 90-day / 30-day Maintenance Detail files live only under gitignored `data/runtime/enterprise/` (STATUS).

### Sheet writes vs Desk sqlite

`docs/onestep-api-auth.md` and STATUS mention **Automations Copy** headers `OneStep factory id` / `OneStep device id`. That CSV is a **GPS box map** (`oilchange devices sync --map`). Grep of `*.go` / `*.ts` finds **no Google Sheets API**. `oilchange report` is a lean CSV (no remaining/due headers — forbidden by `internal/export`). Pasting that CSV into the sheet does not refresh Oil Desk. Desk does not read the sheet.

### UI design (not a missing CSS column)

CarsBoard shows Last oil as **miles only**. `last_oil_date` is in the snapshot type and is **not rendered**. HOLD blanks Last Reading and Remaining in the mirror (`TestFromCarsBlanksStaleReadingOnHold`) and **keeps last oil**. Empty last oil on a HOLD car is still empty last oil — not a HOLD side effect.

16 of 205 cars had no oil line in the ingested exports (STATUS: mostly new units). That is honest empty, not a render bug.

Supabase `fleet_oil_changes` exists (`supabase/migrations/20260902123000_shared_project_fleet_prefix.sql`). `sync` upserts **`fleet_cars` only**. The UI never selects `fleet_oil_changes`.

`pull-supabase` copies `fleet_cars` **into** sqlite. It does not refresh the desk mirror unless you then `sync`.

### Mock-mirror / `--no-remote` / no live Enterprise

| Mode | Last oil in Desk |
|---|---|
| `sync --no-remote` | Mirror from **this** sqlite. Agent sqlite with null last oil → empty Desk. |
| No Supabase write creds | Same: `source=mock-mirror`. |
| `serve` without a prior `sync` | Stale or missing `cars.json` (`mock-seed` empty list). |
| Live `sync-enterprise` with no files and no export URLs | Errors before ingest (cust / DETAILS / MAINT / Fleet Summary URLs). |
| File-drop without `--shop-ro` | Roster + fills only. Last oil stays whatever it was (often null). |

### What “oil change not reflected” usually is

Most likely, stacked — not one bug:

1. Looking at Oil Desk backed by the committed / agent **empty mirror**.
2. Ran `oil-done` or `--shop-ro` and did not `sync --mirror`.
3. Used **live roster + demo maintenance** (ID mismatch).
4. Used live roster **without** `--shop-ro`.
5. Expected the portal or Automations sheet to appear automatically.
6. Expected Remaining / due to fill from last oil alone (needs Last Reading too).
7. Expected an Oil Desk “mark oil done” control (does not exist).

Operator desktop with real Maintenance CSVs already ingested should show last oil **after** `oilchange sync --mirror` against **that** sqlite. Confirm with `oilchange report` (sqlite) vs Desk “Source / Synced / Last oil”.

---

## Question 2 — eFleets presort

### What eFleets All Cars looks like in-repo

`testdata/enterprise/fleetsummary_live.csv`: **205** cars. `ParseVehicles` join key = `Vehicle` (e.g. `26LSZW`). Display name = `Customer Vehicle ID**` (e.g. `Bing-2`). Region = leading letters of that nickname (`VA18` → `VA`, `WNY8` → `WNY`). Plate state is explicitly the wrong region.

File order is **not** sorted by `Vehicle` and **not** exactly sorted by CVI, but it is grouped the way the All Cars grid usually is (prefix clusters):

| Prefix (from CVI) | Count |
|---|---|
| MD | 40 |
| VA | 35 |
| NJ | 25 |
| PA | 25 |
| WNY | 12 |
| RI | 10 |
| FL | 9 |
| NYC | 9 |
| CT | 8 |
| RPI | 6 |
| BK | 4 |
| MN | 4 |
| (none / `-`) | 4 |
| DE, PK | 3 each |
| BING, OH, WESTCHESTER | 2 each |
| BRONX, OFFICE | 1 each |

First rows: `BING-1`, `Bing-2`, `BK-2`, `BK4`, … Last rows: `WNY14`, four `-` units, `NJ OCEAN`, `NYC-SI1`, `Office, was CT9`.

`PDI-NNNN` is **row-order opaque** (`formatPDI`). It must not encode region. Re-ingest can reassign PDI if a new car’s derived id collides (`upsertCarTx`). **Do not sort Desk by PDI** and call it eFleets order.

sqlite `ListCars` and PostgREST `fleet_cars` are `ORDER BY efleets_id` (`internal/store/store.go`, `web/src/lib/fleet.ts`). That is Enterprise **unit number**, not All Cars CVI order.

### What Oil Desk does today

`CarsBoard` (`web/src/components/CarsBoard.tsx`):

- Filter: nickname, plate, region, efleets_id, pdi_id (search box).
- Sort: `nickname` `localeCompare` (case-insensitive).
- No region turnstile, no due-soon sort, no portal row index.

Nickname sort is **close** to All Cars for `VA1` / `NJ23` / `MD11`, and **wrong** for `Office, was CT9`, `NJ OCEAN`, `Bing-2` vs `BING-1` if case/punctuation differs, and it **destroys** the exported grid order.

History (`internal/history/board.go`, `web/src/components/HistoryBoard.tsx`) already turnstiles **one region at a time**. Storage stays car-keyed. That pattern is the right one to copy onto Oil Desk — it is not wired there today (`web/src/app/page.tsx` is `CarsBoard` only).

### Chrome session / DETAILS

DETAILS (`ParseFills`) is a punch stream. Using swipe order or “vehicles that fueled recently” as a roster presort would mix missing cars and skip new units. Fleet Summary / All Cars export is the roster order source.

CDP (`docs/CHROME-SESSION-ENTERPRISE.md`) can eventually download that export with an already-logged-in tab. On main there is no adapter, no `EFLEETS_CDP_URL` in `config.Load`, and live HTTP still needs captured `EFLEETS_FLEETSUMMARY_URL`. Until then, **file-drop Fleet Summary row order is the stable eFleets-shaped list**.

### Automations sheet

Used as `factory_id,device_id,efleets_id` (or header-named OneStep columns) for GPS pairing. It does not define All Cars order. Building a presort that “follows the sheet” without an explicit roster-order column would be a new, untested join.

### Due-soon

`oil.DueAt` = last oil + interval (default 5000). `export.WriteCSV` may **filter** `--due-within` but must not emit remaining/due headers. Desk `remainingMiles` is the same formula. A due-soon presort is safe **in memory** when both miles exist. It is **not** possible for cars still missing last oil or Last Reading without inventing miles — leave those unranked.

---

## Concrete next steps (no invented miles)

### Make last oil show (operator desktop)

1. Use the **desktop** `OILCHANGE_DB` (STATUS: do not replace it with an agent copy).
2. Ingest a **real** Maintenance Detail export whose `Vehicle` column matches Fleet Summary ids:
   ```bash
   oilchange sync-enterprise \
     --vehicles testdata/enterprise/fleetsummary_live.csv \
     --fuel-details <current DETAILS file> \
     --shop-ro data/runtime/enterprise/<12-month-or-current-maint>.csv
   ```
   Do not pair the live roster with `testdata/enterprise/maintenance.csv`.
3. `oilchange report` — count non-empty `last_oil_miles`. STATUS expectation after the 2026-08/09 drops: about **189 / 205**.
4. `oilchange sync --mirror web/data/cars.json` (add `--no-remote` if you must not push null Last Reading to `fleet_cars`).
5. Restart or Refresh Oil Desk. Source should still be `mock-mirror` or `supabase`; Last oil miles should appear for cars that had oil lines.
6. After any later `oil-done`, run `sync --mirror` again. There is no desk write-back.

### Unblock live maintenance (later)

- Capture the **download/export** URL from a logged-in Network panel into `EFLEETS_MAINT_URL` (must look like an export, not `maintenanceSummary?maintenanceTab=detail`).
- Or land `ChromeSessionAdapter` as documented (CDP, no password in chat).
- Do not parse the HTML tab as CSV.

### Honest empties

- 16 cars with no oil line in those exports stay empty until a new export or a measured `oil-done`.
- Remaining / due stay `—` until `compute` has a Last Reading (or HOLD). Do not seed Last Reading to make Remaining look finished.

### If you want eFleets-shaped Desk order

Smallest useful build (suggested, not done here):

1. Oil Desk region turnstile (reuse History regions).
2. Optional “portal order” = Fleet Summary encounter index stored at ingest.
3. Optional due-soon sort using existing `remainingMiles`, unranked when null.
4. Leave Automations sheet as the device map.

Do not sort by PDI. Do not invent a due date column in sqlite or the sheet.

### Docs / README hygiene

README’s live-roster example should not cite the 2-car `maintenance.csv` as the shop-RO for 205 cars. That pairing is the ID-mismatch trap above.

---

## Repo map (this walk)

| Area | Role for these questions |
|---|---|
| `cmd/oilchange` | `sync-enterprise`, `oil-done`, `report`, `sync`, `serve` / `desk`. No oil write on serve. |
| `internal/enterprise` | Header parsers + File/HTTP adapters. Shop RO = Maintenance Detail. |
| `internal/oil` | Last Reading + `IsOilChangeService` + in-app `DueAt`. |
| `internal/store` | `cars.last_oil_*`, `oil_changes`, silent skip if car missing. |
| `internal/app` | Wires ingest / oil-done / compute / sync. File flags win over live. |
| `internal/desk` | Static UI + `/api/cars` from mirror. History/VIN APIs are sqlite; cars list is not. |
| `internal/syncsupabase` | Mirror + `fleet_cars`. HOLD blanks Last Reading, keeps last oil. |
| `internal/export` | Lean report CSV; remaining/due headers forbidden. |
| `internal/history` | Region turnstile — pattern for Desk presort. |
| `web/src/components/CarsBoard.tsx` | Oil UI: nickname sort, last oil miles, remaining. |
| `web/data/cars.json` | What Desk shows today on a stock serve: 205 × null last oil. |
| `testdata/enterprise` | Demo 2-car + live 205-car summary. **No live maintenance fixture.** |
| `docs/CHROME-SESSION-ENTERPRISE.md` | Planned CDP path; not on main. |
| `docs/collab/STATUS.md` | Operator last-oil counts vs empty agent mirror. |
| Scripts | `web/scripts/static-export.sh`, `make-icons.py` only. No sheet sync script. |

Tests that lock the oil story: `TestSyncAndComputeFileDrop`, `TestInsertOilChangeAdvancesLastOilNotLastReading`, `TestOilDoneDoesNotChangeLastReading`, `TestParseShopROOilSeedAndNotOilPan`, `TestFromCarsBlanksStaleReadingOnHold`, `TestLiveFleetHasMoreVehiclesThanTwoCarDemo` (205 cars, **no** `--shop-ro`).

---

## What this doc is not

It does not change ingest, Desk, or Neon. It does not claim the operator desktop last-oil counts from STATUS still hold — re-check with `oilchange report` on that machine. It does not recommend inventing last oil for the 16 cars (or anyone else) so the board looks finished.
