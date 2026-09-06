# Mileage box-score game cards

How `/boxscore/` builds one **unit card** per car, and how to rebuild those cards after a fresh Enterprise drop.

Parent lock (Last Reading vs box score, never invent miles): [`MILEAGE-BOX-SCORE.md`](MILEAGE-BOX-SCORE.md). Scoring lives in `internal/oil.ScorePunches`. Persist is SQLite `mileage_ledger` + `drive_stop_windows`. Desk UI is `web/src/components/BoxScoreBoard.tsx`.

## TLDR (Slack paste)

Mileage box-score cards (`/boxscore/`): each unit is a “game” card for **gas card vs maint+GPS**, not oil due.

- **Recorded** = DETAILS gas-card punch odo at Provider Transaction Time.
- **Expected** = last **good shop RO** odo + OneStep **drive-stop miles** since that RO stamp. Never last oil + interval. Never OneStep odometer (not as expected, not as Last Reading).
- **Diff** = recorded − expected. Positive = **overage** (card ahead). Negative = **shortage** (card behind). `|gap|` = abs(diff).
- Trend = whether `|gap|` is **growing** (worse) or **shrinking** (improving). **Trusted** punches only. Suspect / HOLD stay **visible** and **out of trend**.
- Rebuild: `oilchange boxscore --rebuild` (or Desk **Rebuild from stored windows**). Uses stored `drive_stop_windows` only — missing GPS is HOLD, not zero.
- Fresh Enterprise: file-drop CSV (`--vehicles` + `--fuel-details` + `--shop-ro`) → `sync-enterprise` → pair devices → rebuild → Desk **Measure** for `NO_DRIVESTOP`. Do not invent miles.

---

## 1. Purpose of the “game” cards

`/boxscore/` is a sports-style **box score**: fleet rollup on top, then one **unit card** per eFleets car, then a punch table.

The card answers, for that unit over time:

> When the gas card moves, how far is the card odometer from maintenance + OneStep expectation, and is that `|gap|` getting worse or better?

It is **not**:

- Last Reading on the Oil sheet (`oilchange compute` — Ent last-good @ a known second + OneStep since that second).
- Oil **due** (`last_oil_miles + interval_miles`).
- Card ↔ vehicle identity (`cards rebuild` / GPS-at-the-pump).

Cards are a rollup of `mileage_ledger` rows. The punch table is the play-by-play. Click a card to filter the table to that unit.

---

## 2. Data inputs

Three sources. None of them is OneStep odometer. Mileage History (`--mileage-history`) is context only and is **not** a box-score input.

### Maintenance (good shop RO) → expected **base**

| What | Where |
|---|---|
| Export | eFleets **Maintenance Detail** (shop RO lines) |
| Ingest | `oilchange sync-enterprise --shop-ro PATH` → `ParseShopROs` |
| Store | `shop_ros` (one row per `Vehicle` + `RO ID`; line items collapse to one odo at one shop) |
| Join | `Vehicle` = `cars.efleets_id` |
| Stamp | `RO Completed Date` / `RO Complete Date`, else `RO Created Date`. Naive → `America/New_York` |
| Odo | named `Odometer` column (not column letter) |

**Good maintenance** (`oil.GoodMaintenance`): latest shop RO with `Odometer > 0` and a real timestamp, **unless** it looks abandoned (later DETAILS punches sit far below the shop spike). Fuel punches are **never** the expected base. OneStep odometer is **never** consulted.

Oil/lube lines also seed `oil_changes` / last oil for the Oil Desk. That last-oil number is **not** box-score expected.

### Gas card transactions (DETAILS) → **recorded**

| What | Where |
|---|---|
| Export | eFleets Fuel & Charging **DETAILS** |
| Ingest | `oilchange sync-enterprise --fuel-details PATH` → `ParseFills` |
| Store | `fills` (and `card_transactions` for cards intel) |
| Join | `Vehicle` = roster `efleets_id`. Punches for unknown vehicles are **skipped** (stderr count). Do not invent cars. |
| Recorded odo | `Provider Odometer` |
| Punch second | `Provider Transaction Date` + `Provider Transaction Time` (naive → Eastern, truncated to the second) |
| Unusual | `Provider Unusual Odometer Flag` = `Y` → **suspect** |
| Card / merchant | `Provider Card Number` (else `Provider Vehicle Number`); `Provider Location` |

`Recorded` on the card/ledger is this punch odometer. The DETAILS **Vehicle** column is last-write-wins and is the ingest join, not GPS truth.

### OneStep drive-stop windows → miles **since** maintenance

| What | Where |
|---|---|
| API | `GET /v3/api/public/route/drive-stop` with `device_id`, `dt_tracker_from`, `dt_tracker_to`, `stop_duration=5m0s` |
| Sum | `distance` / `drive_stop_list` trip miles only. **`odometer` / `odometer_from` / `odometer_to` ignored.** |
| Store | `drive_stop_windows` keyed by `(factory_id, from_at, to_at)` where `from_at` = good-maint stamp, `to_at` = punch second |
| Pairing | live `onestep_devices` linked to the car (`factory_id`, not dead, not logistics-personnel label). `display_name` is never a join key. |

`oilchange sync-onestep` writes **`drive_stop_miles`** (trusted fill second → now) for **Last Reading**. That table is **not** the box-score window. Box score needs maint → punch windows in `drive_stop_windows`.

**Rebuild never fetches live GPS.** Missing window → HOLD `NO_DRIVESTOP`, `expected` stays NULL. A measured empty trip list **is** stored as `0` (honest zero miles). Missing is not zero.

---

## 3. Locked formulas

Implemented in `ScorePunches` / `scoreOne`. Do not invent expected. Do not treat missing drive-stop as `maint + 0`.

```
expected  = maint_odo + round(miles_since)     // miles_since from drive-stop only
recorded  = gas card Provider Odometer
difference = recorded − expected               // locked sign
overage   = difference  if difference > 0 else 0
shortage  = −difference if difference < 0 else 0
abs_diff  = |difference|
```

**Trend** (trusted rows only, chronological by punch time then recorded):

| vs previous trusted `abs_diff` | `trend` | UI badge |
|---|---|---|
| first trusted punch | `flat` | flat |
| this `|gap|` larger | `up` | **growing** (worse vs maint+GPS) |
| this `|gap|` smaller | `down` | **shrinking** (improving) |
| same `|gap|` | `flat` | flat |

Fleet / card rollups (`sum_overage`, `sum_shortage`, `trend_up` / `down` / `flat`, `abs_diff_series`, `latest_abs_diff`, `latest_trend`) count **trusted** punches only.

---

## 4. Trusted vs Suspect / HOLD

Every punch that has a recorded odo becomes a **visible** ledger row. Only **trusted** rows enter trend.

| `status` | Typical reason | On the card? | In trend / `|gap|` series? |
|---|---|---|---|
| `trusted` | Good maint + live device + measured window + usable punch | Yes — over/short, latest `|gap|`, series | **Yes** (`in_trend=true`) |
| `suspect` | Unusual-Y flag, or punch **before** the good-maint stamp | Yes — counted in “N gas card transactions”; not in series | **No** |
| `hold` | Missing punch odo/time; no good maint; logistics-personnel nickname; `NO_DEVICE`; `NO_DRIVESTOP` (missing or non-finite miles) | Yes — punch count; “no good maintenance — HOLD” when `has_maint` is false; trend badge stays hold/`—` until a trusted punch exists | **No** (`expected` / `difference` / `abs_diff` stay NULL) |
| `dismissed` / `corrected` | Operator button on suspect/HOLD | Still listed | **No**. Rebuild **will not overwrite** these two statuses |

Punch-table filters: **All gas card transactions** / **Trusted (in trend)** / **Suspect / HOLD**.

HOLD `hold_reason` codes you will see: `NO_TRUSTED_FILL`, `UNUSUAL_Y`, `LOGISTICS_PERSONNEL`, `NO_DEVICE`, `NO_DRIVESTOP`.

Desk actions (login required):

- **Measure** on `NO_DRIVESTOP` — live drive-stop for that maint→punch window, save `drive_stop_windows`, rescore the car.
- **Dismiss** / **Corrected** — still visible, `in_trend=0`.

---

## 5. How each unit card is built (UI + sqlite)

### Rebuild pipeline (one car)

`App.RebuildBoxScore` → `scoreCar` (`fetchLive=false`) → `ScorePunches` → `Store.UpsertLedgerRow`.

1. Roster: `ListCars`.
2. Inputs: `ListFills`, `ListShopROs`, `ListDevicesForCar`.
3. Base: `GoodMaintenance(ros, fills)` → `maint_odo` / `maint_at` / `has_maint`.
4. Device: `liveLinked` — skip dead, empty `factory_id`, logistics-personnel `display_name`.
5. Punches: each fill with a non-nil odometer → `GasCardPunch{recorded, at=ProviderTransactionTime, unusual_y, merchant, card_id}`.
6. Miles: look up `drive_stop_windows` for each live `factory_id` from `maint_at` → punch second. Rebuild: cache only. Measure: live GET then save.
7. Score every punch (HOLD/suspect/trusted). Upsert ledger. Unique key: `(efleets_id, punch_at, recorded_odo)`.

`GET /api/boxscore` (`ListBoxScore`) reads `mileage_ledger` and re-rolls cards. **Empty ledger auto-rebuilds** from stored windows.

### Card face (`BoxScoreBoard` → `Vehicle` JSON)

| Card field | JSON | sqlite / derivation |
|---|---|---|
| Title | `nickname` or `efleets_id` | `cars.nickname` |
| Id line | `efleets_id` | `mileage_ledger.efleets_id` |
| Trend badge | `latest_trend` → growing / shrinking / flat | last **trusted** row `trend` |
| `over N · short N` | `sum_overage`, `sum_shortage` | sum of trusted `overage` / `shortage` |
| `· \|gap\| N` | `latest_abs_diff` | last trusted `abs_diff` |
| `\|gap\| a → b → c` | `abs_diff_series` | trusted `abs_diff` in punch order (not a column — derived) |
| `no trusted \|gap\| series yet` | empty series | no trusted rows yet |
| `maint N` or `no good maintenance — HOLD` | `has_maint`, `maint_odo` | good-maint odo copied onto rows; rollup takes a positive `maint_odo` |
| `N gas card transactions` | `rows.length` | **all** statuses, including suspect/HOLD |

### Punch row ↔ `mileage_ledger`

| UI / JSON | Column | Notes |
|---|---|---|
| Car | `efleets_id` | |
| Provider Transaction Time | `punch_at` | UTC RFC3339 in the API |
| merchant | `merchant` | DETAILS `Provider Location` |
| Gas card transaction | `recorded` → `recorded_odo` | |
| Expected (maint + OneStep) | `expected` → `expected_odo` | NULL on HOLD/suspect skip |
| Diff (rec − exp) | `difference` | NULL when not scored. Legacy rows without the column reconstruct as `overage − shortage` |
| Over / short | `overage`, `shortage` | 0 / 0 when not scored |
| \|gap\| | `abs_diff` | NULL when not scored |
| Trend | `trend`, `in_trend` | `hold` + `in_trend=0` when excluded |
| Status + detail | `status`, `hold_reason`, `hold_detail` | |
| (hidden on card) | `card_id`, `maint_odo`, `maint_at`, `miles_since` | `miles_since` is the measured window, not an odometer |

Working store is SQLite (`OILCHANGE_DB`). `oilchange backup-neon` copies `mileage_ledger` and `drive_stop_windows`. Do not write invented miles into Neon or Supabase.

---

## 6. Rebuild CLI / Desk

```bash
export OILCHANGE_DB=./oilchange.sqlite   # daily driver
go build -o bin/oilchange ./cmd/oilchange

# List ledger (rebuilds automatically if the table is empty)
./bin/oilchange boxscore

# Recompute every car from shop ROs + fills + stored drive-stop windows
./bin/oilchange boxscore --rebuild

# Same rebuild, print one unit
./bin/oilchange boxscore --rebuild --efleets-id 27VA15
```

`--rebuild` does **not** call OneStep. It does **not** write Last Reading.

Desk (login required: `/boxscore/`):

| Control | API | Same as |
|---|---|---|
| Page load | `GET /api/boxscore` | `ListBoxScore` |
| **Rebuild from stored windows** | `POST /api/boxscore/rebuild` | `RebuildBoxScore` |
| **Measure** | `POST /api/boxscore/measure` `{efleets_id, punch_at}` | `MeasureBoxScorePunch` (live window) |
| **Dismiss** / **Corrected** | `POST /api/boxscore/dismiss` or `/correct` | status update, `in_trend=0` |

CLI one-liner after rebuild:

`boxscore trusted=… suspect=… hold=… overage=… shortage=… trend_up=… trend_down=… trend_flat=…`

---

## 7. Load fresh Enterprise DETAILS + Maintenance, then rebuild cards

Implemented path is **file-drop CSV** into `sync-enterprise`. Parsers are **header-by-name CSV** (`encoding/csv`). The CLI help mentions xlsx; `FileAdapter` reads the file as bytes and the parsers expect CSV. Convert Excel to CSV first. Put **headers on row 1** (named columns). Live eFleets Excel often has a title on row 1 and headers on row 2 — strip the title or the ingest fails closed (`eFleets file missing headers`). Do not guess columns by letter.

Typical drop dir (gitignored): `data/runtime/enterprise/`. Do not commit PII dumps. Do not pair a live 205-car Fleet Summary with `testdata/enterprise/maintenance.csv` (demo ids only).

### Operator steps (as implemented)

```bash
export OILCHANGE_DB=./oilchange.sqlite

# 1) Ingest roster + DETAILS (recorded) + Maintenance Detail (expected base).
#    Does not compute Last Reading. Does not score the box score.
./bin/oilchange sync-enterprise \
  --vehicles data/runtime/enterprise/FleetSummary.csv \
  --fuel-details data/runtime/enterprise/DETAILS.csv \
  --shop-ro data/runtime/enterprise/Maintenance.csv

# 2) GPS pairing (or punches HOLD NO_DEVICE). factory_id only; never display_name.
#    Optional if cars already have a live linked box in onestep_devices.
./bin/oilchange devices sync --map data/runtime/onestep-map.csv
# and/or: ./bin/oilchange devices vin --from data/runtime/device-information.json

# 3) Score cards from stored windows. Missing windows → HOLD, expected stays blank.
./bin/oilchange boxscore --rebuild

# 4) Desk: oilchange desk  →  /boxscore/
#    Click Measure on NO_DRIVESTOP rows (needs OneStep token in oilchange.env).
#    That writes drive_stop_windows, then rescores that car.
```

Dry run on committed fixtures (demo cars, not fleet miles):

```bash
./bin/oilchange sync-enterprise \
  --vehicles testdata/enterprise/fleetsummary.csv \
  --fuel-details testdata/enterprise/details.csv \
  --shop-ro testdata/enterprise/maintenance.csv
./bin/oilchange boxscore --rebuild
```

### What is **not** a live rebuild (yet)

| Path | Status |
|---|---|
| `sync-enterprise` with no file flags + `EFLEETS_USERNAME` / `PASSWORD` / `CUST_NUM` + captured `EFLEETS_DETAILS_URL` / `EFLEETS_MAINT_URL` / `EFLEETS_FLEETSUMMARY_URL` | Implemented `HTTPAdapter`. MFA / JS wall **fails closed**. Never invent a CSV. |
| Chrome CDP already-logged-in tab | Documented in [`CHROME-SESSION-ENTERPRISE.md`](CHROME-SESSION-ENTERPRISE.md). **`ChromeSessionAdapter` is not in code.** File-drop is the working live path. |
| `oilchange cards history --fuel-details` | Ingests DETAILS (and optional Fleet Summary). **Does not ingest `--shop-ro`.** Box-score expected still needs Maintenance via `sync-enterprise --shop-ro`. |
| `oilchange sync-onestep` | Last Reading miles-since, not box-score maint→punch windows. |
| `oilchange compute` | Writes Last Reading only. Does **not** build game cards. |

Never ask anyone to type an eFleets password in chat. File-drop CSVs do not need a portal login.

---

## Related

- [`MILEAGE-BOX-SCORE.md`](MILEAGE-BOX-SCORE.md) — Last Reading vs box-score formulas
- [`FOR-LLMS.md`](FOR-LLMS.md) — command locks
- [`DATA-OWNERS.md`](DATA-OWNERS.md) — SQLite vs Neon vs Supabase
- Code: `internal/oil/boxscore.go`, `internal/app/boxscore.go`, `cmd/oilchange/boxscore_cmd.go`, `internal/store/ledger.go`, `migrations/008_mileage_ledger.sql`, `migrations/009_ledger_difference.sql`
