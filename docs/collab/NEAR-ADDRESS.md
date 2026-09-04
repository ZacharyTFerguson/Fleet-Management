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
- CLI: `oilchange cards nearby [--card ID] [--live] [--report]`
- Tests: parse fixture, httptest generate/poll/mutex, fill-day bounds, exclusive vs collision. Never live OneStep in `go test`.

## Do not

- Use bank posting time.
- Auto-join from a single 1-mile hit.
- Loosen GPS-first exclusive-sit (350 m / short stop) to 1 mile.
- Write Last Reading from this hunt.
- Commit `oilchange.env`, sqlite, or `data/runtime/`.
