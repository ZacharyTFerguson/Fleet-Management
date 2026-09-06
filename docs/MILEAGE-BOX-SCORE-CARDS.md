# Mileage box score: how unit cards are built

This is the operator/dev walkthrough for the **unit cards** on Oil Desk `/boxscore/`. Formula locks live in [`docs/MILEAGE-BOX-SCORE.md`](MILEAGE-BOX-SCORE.md). Code: `internal/oil/boxscore.go` (pure math), `internal/app/boxscore.go` (load + persist), `web/src/components/BoxScoreBoard.tsx` (cards).

The box score answers: **when a gas card transaction posts, how far is the card odometer from maintenance + OneStep expectation, and is that `|gap|` growing or shrinking?**

It does **not** write Last Reading. Last oil + oil interval is the Oil Desk **due** number only — never box-score expected. Never invent miles. Never use OneStep odometer as a base.

## Inputs (three sources, three jobs)

| Source | Enterprise export | Stored as | Role on the card |
|---|---|---|---|
| **Maintenance RO** | Maintenance Detail (`sync-enterprise --shop-ro`) | `shop_ros` | **Expected base.** Last *good* shop odometer + stamp. Fuel punches are never this base. |
| **Gas card transaction** | Fuel & Charging DETAILS (`--fuel-details`) | `fills` | **Recorded.** Provider Odometer at Provider Transaction Date + Time. One row on the punch table; one candidate for the `|gap|` series. |
| **OneStep drive-stop** | `GET /v3/api/public/route/drive-stop` (`device_id`, `dt_tracker_from`, `dt_tracker_to`) | `drive_stop_windows` | **Miles since the maintenance stamp** to that punch second. Sum `distance` / `drive_stop_list` only. Missing window is HOLD, not zero. |

Join rules (same as the rest of oilchange):

- Car key is `efleets_id` (Enterprise **Vehicle**).
- GPS box join is `factory_id`. `device_id` is the History / drive-stop query id. `display_name` is never a join key.
- Dead `factory_id`s and logistics-personnel labels are dropped before miles are read.

### Maintenance RO → good maint

`oil.GoodMaintenance` picks the latest shop RO with a positive odometer and a real timestamp (`RO Completed Date` / `RO Complete Date`, else `RO Created Date`). Line items that share an RO ID collapse to one odometer at one shop.

A shop spike that later fills walk away from is **abandoned** and is **not** a base (same VA10 idea as Last Reading). If there is no surviving RO, the card shows **no good maintenance — HOLD** and every punch is HOLD `NO_TRUSTED_FILL`. Fuel DETAILS never substitute as the expected base. OneStep odometer is never consulted.

### Gas card transaction → recorded

Each `fills` row with a Provider Odometer becomes a `GasCardPunch`:

- **Recorded** = Provider Odometer
- **At** = Provider Transaction Date + Time (second precision; naive times America/New_York)
- **Card** = Provider Card Number, else Provider Vehicle Number / company vehicle number
- **Unusual Y** = Provider Unusual Odometer Flag `Y` → Suspect (visible, out of trend)
- **Merchant** = Provider Location (label only)

A punch with missing odometer or zero time is HOLD, not scored.

### OneStep drive-stop → miles since stamp

For each punch at or after the good-maint stamp, the window is **maint timestamp → punch second**. Rebuild reads `drive_stop_windows` only. A live GET happens only on Desk **Measure** / `MeasureBoxScorePunch` (opt-in per punch). The first stored window on a live linked box wins.

Missing, NaN, infinite, or negative miles → HOLD `NO_DRIVESTOP`. Do not treat missing GPS as `maint + 0`.

## Expected, recorded, difference

For one punch that survives the checks:

```
expected  = good_maint_odo + round(drive_stop_miles_since_maint)
recorded  = gas card Provider Odometer
difference = recorded − expected     # locked sign
abs_diff   = |difference|
```

| Sign | Meaning | Stored |
|---|---|---|
| `difference > 0` | Card **ahead** of maint+GPS | `overage = difference`, `shortage = 0` |
| `difference < 0` | Card **behind** | `shortage = -difference`, `overage = 0` |
| `difference = 0` | Exact match | both 0; `abs_diff = 0` |

Example (from `TestScorePunchesExpectedVsGasCard`): maint 10 000 @ May 1; punch 10 120 after 100 drive-stop miles → expected 10 100, difference +20, overage 20.

## Trusted vs Suspect / HOLD — what enters the trend

Every punch stays **visible** on the table. Only **trusted** punches enter the trend and the card’s `|gap|` series.

| Status | When | On the card | In trend? |
|---|---|---|---|
| **trusted** | Good maint, live `factory_id`, measured non-negative miles, punch at/after maint, recorded odo present, not Unusual Y, not logistics-personnel | Counts toward overage / shortage / `|gap|` series | **Yes** (`in_trend`) |
| **suspect** | Unusual Y, or punch **before** the good-maint stamp | Badge + reason; still listed | **No** |
| **hold** | No recorded odo/time; no good maint; no live device; no/invalid drive-stop window; logistics-personnel nickname | Badge + `hold_reason` / `hold_detail` | **No** |
| **dismissed** | Operator **Dismiss** on a suspect/HOLD row | Stays visible; rebuild will not overwrite this status | **No** |
| **corrected** | Operator **Corrected** | Same persist lock as dismissed | **No** |

HOLD reasons the scorer writes:

| `hold_reason` | Typical `hold_detail` |
|---|---|
| `NO_TRUSTED_FILL` | Missing punch odo/time, or no good maintenance before this transaction |
| `UNUSUAL_Y` | Unusual-Y gas card transaction — visible, excluded from trend |
| `LOGISTICS_PERSONNEL` | Logistics-personnel label; not a pairing key |
| `NO_DEVICE` | No live `factory_id`; cannot measure drive-stop since maintenance |
| `NO_DRIVESTOP` | No stored/measured window from maint → this punch (or miles not a real non-negative distance) |

Trend is **`|gap|` vs the previous trusted `|gap|`**, not signed overage/shortage:

- first trusted punch → `flat`
- later `|gap|` larger → `up` (growing / worse vs maint+GPS)
- later `|gap|` smaller → `down` (shrinking / improving)
- same `|gap|` → `flat`

Suspect and HOLD never update `prevAbs`, so they cannot create a fake shrink or grow.

## How each unit card is assembled

Desk `/boxscore/` loads `GET /api/boxscore` → `ListBoxScore`. If `mileage_ledger` is empty, that call rebuilds first.

### Fleet strip (above the cards)

| Field | Meaning |
|---|---|
| Trusted / Suspect / HOLD | Punch counts (`trusted_n`, `suspect_n`, `hold_n`) |
| Overage / Shortage | Sum of trusted `overage` / `shortage` across the fleet |
| \|gap\| growing / shrinking / flat | Count of trusted punches whose trend is `up` / `down` / `flat` |
| **Rebuild from stored windows** | `POST /api/boxscore/rebuild` — same as `oilchange boxscore --rebuild` |

### Per-unit card (`car-card unit-card`)

One card per vehicle that has ledger rows. Click selects that unit and filters the punch table.

| Card face | Source |
|---|---|
| Title | `nickname`, else `efleets_id` |
| Trend badge | Latest **trusted** trend: growing / shrinking / flat (`latest_trend`) |
| Mono id | `efleets_id` |
| `over N · short M · \|gap\| K` | Trusted sums + latest trusted `abs_diff` |
| `\|gap\| a → b → c` | `abs_diff_series`: trusted `abs_diff` in punch-time order. Empty → **no trusted \|gap\| series yet** |
| `maint {odo}` or **no good maintenance — HOLD** | `has_maint` / `maint_odo` from `GoodMaintenance` (or last ledger row on list) |
| `N gas card transactions` | All rows for that car (trusted + suspect + HOLD + dismissed/corrected) |

`abs_diff_series` is the game strip: you can see whether the card error is tightening or walking away without opening the table.

### Punch table (under the cards)

Filters: all / trusted (in trend) / suspect+HOLD. Columns:

| Column | Field |
|---|---|
| Car | `efleets_id` |
| Provider Transaction Time | `punch_at` + merchant |
| Gas card transaction | `recorded` |
| Expected (maint + OneStep) | `expected` or — on HOLD |
| Diff (rec − exp) | `difference` (signed) |
| Over / short | `overage` / `shortage` |
| \|gap\| | `abs_diff` |
| Trend | growing / shrinking / flat / out (`in_trend=false`) |
| Status | trusted / suspect / hold / dismissed / corrected + `hold_detail` |

Row actions:

- **Measure** — only on HOLD `NO_DRIVESTOP`. Live drive-stop for that maint→punch window, persist the window, rescore the car.
- **Dismiss** / **Corrected** — suspect or HOLD. Persist lock: later rebuilds keep that status and keep `in_trend=false`.

## Rebuild path

```
DETAILS (fills) + Maintenance (shop_ros) + devices + drive_stop_windows
        │
        ▼
oilchange boxscore --rebuild
   or Desk “Rebuild from stored windows”
   or GET /api/boxscore when mileage_ledger is empty
        │
        ▼
per car: GoodMaintenance → punches from fills → stored window miles
        │
        ▼
oil.ScorePunches  →  mileage_ledger upsert  →  unit cards + punch table
```

| Path | Live OneStep? | Notes |
|---|---|---|
| `oilchange boxscore` | No | Lists persisted ledger (rebuilds if empty) |
| `oilchange boxscore --rebuild` | **No** | Re-scores every car from **stored** windows. Missing window → HOLD `NO_DRIVESTOP`. Never invents miles. Optional `--efleets-id` only filters printout. |
| Desk **Rebuild from stored windows** | **No** | `POST /api/boxscore/rebuild` → same `RebuildBoxScore` |
| Desk **Measure** | **Yes** (one punch) | `POST /api/boxscore/measure` `{efleets_id, punch_at}` |
| `oilchange desk` / `serve` | n/a | Hosts `/boxscore/` from the embedded `web/out` |

Rebuild upserts every scored row. Rows already marked **dismissed** or **corrected** keep that status.

`oilchange backup-neon` copies `mileage_ledger` and `drive_stop_windows` to Neon. Do not write invented miles into Neon or Supabase. Vault / desk users stay off Neon.

### Constructing live cards (DETAILS 90d + Maintenance)

1. Ingest Fuel & Charging DETAILS and Maintenance Detail (`sync-enterprise --fuel-details … --shop-ro …`). That loads recorded punches and candidate shop ROs. It does **not** score cards.
2. Confirm each unit has a live `factory_id` (`devices` registry). No box → HOLD `NO_DEVICE`.
3. Rebuild. Punches without a stored maint→punch window stay HOLD `NO_DRIVESTOP` with no invented expected.
4. **Measure** those HOLDs (or persist windows another way), then rebuild again. Trusted rows appear on the card’s `|gap|` series.

`oilchange compute` is a different job (Last Reading / oil due). It does not assemble these cards.

## Code map

| Piece | File |
|---|---|
| Expected / recorded / HOLD / trend | `internal/oil/boxscore.go` (`ScorePunches`, `GoodMaintenance`) |
| Load fills + ROs + windows; persist ledger | `internal/app/boxscore.go` |
| CLI | `cmd/oilchange/boxscore_cmd.go` |
| Desk HTTP | `internal/desk/api_desk.go` (`/api/boxscore`, `/api/boxscore/{rebuild,measure,dismiss,correct}`) |
| Unit cards + table | `web/src/components/BoxScoreBoard.tsx`, `web/src/app/boxscore/page.tsx` |
| SQLite | `internal/store/ledger.go` (`mileage_ledger`, `drive_stop_windows`) |
| Locks / tests | `internal/oil/boxscore_test.go`, `internal/app/boxscore_test.go` |
