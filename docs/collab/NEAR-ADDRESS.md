# Near-address hunt (fill day ±1, 1 mile)

How oilchange finds GPS boxes at a fuel punch when the card is still unknown. Last Reading is untouched. `display_name` is never a join key.

## Why this exists

GPS-first matching only names a swipe when **exactly one linked car** is in a short stop at the pump (≤2 h, ±20 min, ~350 m). Unknown cards are the leftovers: collisions, unpaired boxes, or a car that sat a bit off the pump cluster.

The OneStep portal **Generate Reports → Nearby Address** (`near_address`) answers a wider question: *which devices were within one mile of this station between these dates?* That is a watch list, not a join. Repeat fills until one `factory_id` is exclusive.

## Fill time (not the bank)

Use the DETAILS **provider transaction / swipe second** (`card_transactions.at` / `Fill.ProviderTransactionTime`). Do **not** use bank posting / recognition time. Naive stamps are `America/New_York`.

Window (inclusive calendar days in Eastern):

- `datetime_from` = **00:00:00** on the day **before** the fill date
- `datetime_to` = **23:59:59.999** on the day **after** the fill date

Example: fill Tuesday 16 Jun 2026 10:14 AM EDT → from Mon 15 Jun 00:00 EDT (`2026-06-15T04:00:00Z`) through Wed 17 Jun 23:59:59.999 EDT (`2026-06-18T03:59:59.999Z`).

Radius: **1 mile**.

## Live generate-reports contract (2026-09-04)

Auth: same jwt-rs256 as drive-stop (`OneStepAPIKEY` PEM + `OneStepAPIKEYTobeSigned`). Do not log tokens.

Catalog: `GET /v3/api/public/report-type` includes `near_address`. Output fields:

`near_address_entity`, `near_address_factory_id`, `near_address_device_id`, `near_address_device_groups`, `near_address_device_group_ids`, `near_address_vin`, `near_address_license_plate`, `stop_start_time`, `stop_end_time`, `stop_duration`, `distance_from_location`, `position`, `address`.

Queue a job:

```
POST /v3/api/public/report/generate
```

Body is the **spec at the top level** (not a guessed `address` / `radius` / `dt_from`). Extra unknown keys are dropped. Proven fields copied from completed portal jobs plus a live probe that echoed them back:

```json
{
  "user_report_name": "oilchange near_address",
  "report_type": "near_address",
  "all_user_devices": true,
  "exclude_inactive_devices": true,
  "datetime_from": "2026-06-15T04:00:00Z",
  "datetime_to": "2026-06-18T03:59:59.999Z",
  "time_zone": "America/New_York",
  "report_file_type_list": ["json"],
  "report_output_field_list": [
    "near_address_entity",
    "near_address_factory_id",
    "near_address_device_id",
    "near_address_vin",
    "stop_start_time",
    "stop_end_time",
    "stop_duration",
    "distance_from_location",
    "position",
    "address"
  ],
  "report_options_near_address": {
    "search_address_string": "203 CRAIGDELL RD, LOWER BURRELL, PA",
    "range": { "value": 1, "unit": "mi", "display": "1 mi" }
  }
}
```

Poll: `GET /v3/api/public/report-generated/{report_generated_id}` until `status` is `done` (or `failed`). Live probe job `6le7jf-2QnaYqk81f07-1k` completed in ~3 minutes over 223 authed devices with the spec above.

Do **not** send top-level `address` / `radius` / `dt_from` — those never land on `report_spec`. The address lives only in `report_options_near_address.search_address_string`. Device selection that works on this account is `all_user_devices: true` + `exclude_inactive_devices: true` (portal default), not an empty `device_id_list` with every `all_*` flag false (that fails `Invalid date time range` or yields no devices).

JSON download: the job carries `OutputFilePath` on the OneStep webfiles volume. Public `GET` siblings (`/download`, `/file`, `/data`, `/csv`, …) were **404**. `{id}.json` was **403** (controller `ReportGeneratedAPIController.Fetch`, not a row payload). oilchange still generates+polls so the spec stays honest; **rows used for the hunt come from drive-stop stop windows** (same 1 mile / fill-day ±1 contract) which this repo already fetches with mutex-serialized HTTP.

## OneStep API rate / batching (support.php)

From OneStep support **Data and Functionality** ([track.onestepgps.com/support.php](https://track.onestepgps.com/support.php)):

- API is **real-time**.
- Recommended pull cadence: **15–30 seconds or more** between routine polls of the same live feed.
- Hourly API limit: **5,000** calls (~120,000/day theoretical). In practice even large customers land **5,000–10,000 calls/day**.
- 5,000/hour is more than 1/second (3,600 s/hour).
- **Recommend batching requests for multiple devices** when the endpoint supports it.

How oilchange applies that:

| Path | Batching? | Pace |
|---|---|---|
| `near_address` generate-reports | **Yes** — one job with `all_user_devices: true` covers the fleet | `--report-cap` (default 3) so we never spam generate |
| `GET /route/drive-stop` | **No** — live contract is a single `device_id` query param (`dt_tracker_from` / `dt_tracker_to` / `stop_duration`). Comma-lists / multi-id params are **not** proven; do not invent them | `Client.mu` serializes HTTP; nearby `--live` also enforces **≥1 s** between boxes and logs progress every **25** devices so a ~260-box fill-day pull stays under the hourly ceiling without looking hung |
| `cards watch --live` | **Batch by shrinking the set** — one drive-stop per **watched** `factory_id` covering the union of the newest 10 fill-day windows (not 260 boxes, not 10 GETs per fill) | **35 s** between GETs unless `Retry-After` is longer. Do not re-run nearby `--live`. |

A strict 15–30 s gap between each of 260 boxes would take hours; that cadence is for routine realtime polling, not a one-shot coverage backfill. Prefer the batched `near_address` job when JSON download works; until then, paced one-device drive-stop is the honest path.

## Hunt rules (watch → certain)

1. Unknown cards only (no GPS-named **car** era yet, or coverage `unknown remaining`). Driver-kept / logistics PERSON cards never create a device↔car join.
2. Pick a gas station with an address (prefer a geocoded pump from exclusive GPS sits).
3. Window = fill day ±1. Radius = 1 mile from station lat/lng (or the report’s `distance_from_location` when JSON rows exist).
4. Every `factory_id` in that window is a **watch**. One hit is not a join.
5. Prefer devices whose stop **overlaps the fill second** (±20 min slack). Those are “at the pump when it filled up.”
6. Repeat fills. Same exclusive `factory_id` at fill time on **2** days = likely; **3** = certain enough to name the card as that box’s **linked car** (if the box already has a `factory_id`→car link). Unpaired boxes stay on the watch list unless exact 17-char OBD VIN matches `cars.vin` (existing `LinkByVIN`, not this hunt).
7. Two boxes at fill time → not exclusive; keep watching.
8. Never invent a `factory_id`. Never join on `near_address_entity` / `display_name` / plate.

## Code

- `internal/onestep/nearaddress.go` — generate + poll + JSON parse; each HTTP call holds `Client.mu` (do not lock inside `get()`).
- `internal/cards/nearby.go` — fill-day window, 1-mile haversine on stop visits, watch/likely/certain.
- CLI: `oilchange cards nearby [--card ID] [--live] [--report] [--persist] [--report-cap N]`
- CLI: `oilchange cards watch [--card ID] [--live] [--persist] [--fills 10] [--pace 35s]` — newest 10 punches per unknown card, drive-stop **only watched** `factory_id`s, 35s (or `Retry-After`) between GETs. Virginia DETAILS vehicles seed which box to ask about; GPS exclusive days still required to persist.
- Tests: parse fixture, httptest generate/poll/mutex, fill-day bounds, exclusive vs collision, same-day watch, incomplete coverage, PERSON persist veto. Never live OneStep in `go test`.

## Rewrite (review loop 1)

A second pass found four bugs in the first wiring. They are closed:

1. **Unknown filter.** `CalledEFleetsID` is in-memory. The hunt `ApplyCalls` then skips GPS-named fills and fills already inside a **car** era. PERSON eras stay watch-only; persist never writes them.
2. **Certainty counted fills, not days.** Three punches on one Eastern calendar day is one exclusive day = watch. Likely/certain need 2/3 distinct `America/New_York` `YYYY-MM-DD` keys.
3. **Incomplete GPS cache.** Cache is often only the linked boxes. Likely/certain require every eligible active box to have a covering visit (positioned or `HasPos=false` placeholder) in the union fill-day window. `--persist` is skipped when coverage is incomplete.
4. **PERSON persist.** Hard veto. `ReplaceEras` is a single `BEGIN`/`COMMIT` so a failed insert cannot leave eras deleted.

## Rewrite (review loop 2)

Coverage is a **full-window fetch**, not “one short stop somewhere in the union.” `--live` writes a spanning `HasPos=false` sentinel after each successful drive-stop pull. A cached 20-minute sit is not coverage. `--live` also refreshes the OneStep device list so a box missing from sqlite cannot be ignored.

`--live` is required for live HTTP. Plain `cards nearby` uses the GPS cache only (it will not fetch linked boxes through GPS-first). `--report-cap` counts generate **attempts**, including failures.

Exclusivity is per **swipe** (only one `factory_id` overlapping fill ±20 min), then unique Eastern days of those wins. Persist only if exactly one certain device and it already has a car link; the era span is those exclusive days only. Two certain boxes (even if one is unpaired) do not persist.

Station lookup prefers name + full street. Two SHELL pumps in the same city stay separate.

## Live hunt results (2026-09-04)

Cache-only: `certain=0 likely=0 watch=45 cards=20 coverage_complete=false` (linked-box cache only).

Live `oilchange cards nearby --live` (no `--report`, no `--persist`): fetched **260** uncovered boxes one-at-a-time under `Client.mu`, ~20.4 min, exit 0. That run predated the ≥1 s nearby pace + progress logs; it already averaged ~4.7 s/box from network time alone. **Do not re-run a full 260-box live fetch** unless batching is proven on drive-stop (it is not).

```
nearby certain=0 likely=1 watch=47 cards=20 radius=1mi window=fill-day±1 coverage_complete=true
```

- **Certain = 0** → did not `--persist`. Zero car eras added. Last Reading untouched.
- **Likely = 1:** card `xxxxxxxxxxxxx57770`, `factory_id=7000335987` already linked to `292NCX`, exclusive at fill time on **2** Eastern days (need 3). Watch list, not a join.
- **Watch = 47** across 12 cards (print lists at most 5 watch devices per card). 8 cards had no 1-mile hit.
- Unpaired `factory_id` `4572242789` and `3271251658` appeared in the mile list and stayed unpaired.
- After the fetch, `cards coverage --no-gps` is **known 62 / 205 (30.2%)** vs the VIN-wave **63 / 205**. Extra stop visits did not raise exclusive GPS-first eras.
- Artifact: `/opt/cursor/artifacts/nearby_live_summary.txt`.

## Watch loop (newest 10, watched boxes only)

A second full-fleet nearby `--live` is the wrong next step. After the 2026-09-04 260-box pull the leftover work is: **ask only the cars already on the watch list** (plus Virginia recorded vehicles, which this fleet treats as the right seed).

`oilchange cards watch [--live] [--persist] [--fills 10] [--pace 35s]`:

1. GPS-first uses the stop cache only (`OneStep` is niled around that call). Watch `--live` must not fleet-fetch linked boxes through `cards rebuild` GPS-first.
2. Unknown fills only (same `EligibleUnknownFills` as nearby). PERSON stays watch-only.
3. Card order: highest exclusive-day count first (the prior likely card), then Virginia recorded vehicles, then newest swipe.
4. Per card: take the **newest 10** provider swipes. Seed `factory_id`s from cache 1-mile hits, plus **only the newest** VA recorded vehicle’s linked box (mixed DETAILS Vehicle columns are not extra fetches), plus one hypothesis roster car if the list is still empty. Never invent an unpaired `factory_id`. Persist ranks that same newest VA box, so 1-mile spectators do not have to span the window.
5. `--live` drive-stop those boxes for the **union** of those 10 fill-day ±1 windows, skipping a box already spanning-covered. One `device_id` per GET. Default **35 s** between calls; a `429`/`503` `Retry-After` waits that long and retries once.
6. Hunt still scores **all** eligible fills for that card (May exclusive days + August fetches). Persist if watched-set coverage is complete, exactly one certain **linked** car, not PERSON, not unpaired. Persist ranks the newest VA seed box (1-mile spectators do not block). No VA seed → rank the hunt hits that were fetched; a DETAILS Vehicle hypothesis is not a join.
7. `cards rebuild --no-gps` keeps nearby-certain car eras the ladder did not replace, so a rematch cannot wipe a watch persist.
8. After the GPS watch, **ask OneStep for VIN** on leftover unpaired boxes (not only hunt hits): `GET /device?device_id=&latest_point=true` (OBD `device_state.vin` only). Exact 17-char match to `cars.vin` writes the factory_id→Enterprise link. `display_name` is never a join. CLI: `oilchange devices vin`. Then `cards history --no-gps` rematches GPS-at-the-pump and keeps watch-persisted car eras in coverage.

Maintenance VIN remains exact 17-char OBD `device_state.vin` = `cars.vin` from the roster (Fleet Summary / shop-backed VIN). A Fuel DETAILS drop is not a shop RO; do not `--shop-ro` a DETAILS file.

The Aug–Sep 2026 DETAILS file (`DETAILS_583424_30-Days` (14)/(13) and the identically hashed `.xls`) is a new swipe window, not a duplicate of the May–June ingest.

## Do not

- Use bank posting time.
- Auto-join from a single 1-mile hit.
- Loosen GPS-first exclusive-sit (350 m / short stop) to 1 mile.
- Write Last Reading from this hunt.
- Commit `oilchange.env`, sqlite, or `data/runtime/`.
- Re-run a 260-box `cards nearby --live` to chase leftovers — use `cards watch`.
