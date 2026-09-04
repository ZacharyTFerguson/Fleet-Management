# Shared status

Updated: 2026-09-04 (UTC) — watch-loop (newest 10 fills, watched boxes only, 35s) + OneStep VIN ask (`devices vin` / watch `--live` pairs unpaired `factory_id` → `cars.vin`). Do **not** re-run the 260-box nearby `--live`. Known on this sqlite was **62 / 205 (30.2%)** after the nearby fleet hunt (certain 0, likely 1 card `57770`→`292NCX`).

## True now

- Daily driver is sqlite (`OILCHANGE_DB=./oilchange.sqlite`). Neon `Fleet_Manage_Oil` on project `Fleet_Management_Neon` (`icy-thunder-13848536`, branch `production`) is the backup. Unpooled `DATABASE_URL` (no `-pooler`, not XRAY `chjqcznyxvtjbamttqdj`). Do not add S3, extra Neon, or direct AWS. This fleet’s Supabase is Oil Desk today; later a second full copy if Neon is down; today `sync` is Oil Desk `fleet_cars` only.
- Oil Desk: `oilchange serve` at `http://127.0.0.1:4739`, mirror `web/data/cars.json` (`source=mock-mirror`, synced ~2026-09-04T03:03:06Z).
- Roster: **205** cars on the operator desktop. **146** Last Reading (fill odo + drive-stop miles). **189** last oil from Enterprise shop ROs. **0** `NO_DRIVESTOP`. Open HOLDs: **55** `NO_DEVICE`, **4** `NO_TRUSTED_FILL`. This Cloud Agent sqlite is a file-drop mix (205 live ids + demo `27TESTA`/`27TESTB` = **207** rows) and does **not** hold those Last Reading values — do not copy an empty agent db over the desktop sqlite.
- `oilchange cards watch [--live] [--persist] [--fills 10] [--pace 35s]` loops unknown cards: newest 10 punches, drive-stop **only watched** `factory_id`s (VA seed + 1-mile hits). After the hunt it asks OneStep `GET /device?device_id=&latest_point=true` for OBD VIN on unpaired watched boxes and joins exact 17-char `device_state.vin` = `cars.vin`. `oilchange devices vin` is the same VIN ask for the whole unpaired set. Then `cards history --no-gps` rematches GPS-at-the-pump. Never Last Reading. Never `display_name`.
- Live OneStep map: `data/runtime/onestep-map.csv` (gitignored) — 155 sheet `factory_id,device_id,efleets_id` plus OBD VIN attach for unpaired boxes. Live `/device?limit=1000`: **264** boxes, **204** with a car link, **8** roster cars still missing a device. Do **not** use `testdata/onestep/map.csv` on this roster. **`display_name` is never a join key.** Exact 17-char `device_state.vin` = `cars.vin` is allowed for unpaired boxes only.
- Drive-stop was HTTP 500 on invented params `factory_id` + `from`. Live names: `device_id`, `dt_tracker_from`, `dt_tracker_to`, `stop_duration=5m0s`. Do not send `return_points`. Miles on `distance.value` (+ `unit`); ignore `odometer_from` / `odometer_to`. Smoke: factory `3271116717` 48h = **263.66** miles. Fleet fetch: **146/146** stored, including measured 0. Code: `internal/onestep/client.go`.
- `oilchange probe-onestep` is the one-shot live GET (no Last Reading write). 2026-09-04 jwt-rs256: Bing-2 6h/24h/48h = 0 / 161.10 / **263.66**; three other boxes 48h = 293.85 / 322.13 / 0. See `docs/collab/DRIVE-STOP-LIVE-PROBES.md`. Drive-stop HTTP is mutex-serialized.
- Example: eFleets `26LSZW` (Bing-2) Last Reading **180312**, no HOLD.
- Neon backup after the prior wave: cars=205, devices=186, miles=146.
- `go test ./...` green after the station-ladder change.
- `hold_events` still has stacked open rows for cars that remain on HOLD (sync reported holds=236 events, 59 cars). `oilchange holds` / car `hold_reason` are the operator surface.

## Do not

- Invent miles or use OneStep odometer / `odometer_from` / `odometer_to`.
- Join on `display_name`.
- Use `testdata/onestep/map.csv` as the live roster.
- Push `sync --mirror` to Supabase unless asked (this wave used `--no-remote`).
- Recreate the Neon project or point `DATABASE_URL` at pooled / XRAY / supabase.co.
- Ask Zachary to paste the OneStep key. It is already in `oilchange.env`.
- Ask Zachary to log into eFleets or paste `EFLEETS_PASSWORD` in chat. Put `EFLEETS_USERNAME` / `EFLEETS_PASSWORD` / `EFLEETS_CUST_NUM` in gitignored `oilchange.env` or Cloud Agent secrets (`EFleetsUsername` / `EFleetsPassword` / `EFleetsCustNum`).
- Seed Last Reading to clear HOLD. Station ladder never writes Last Reading.

## Last oil (Enterprise shop RO)

Ingested eFleets Maintenance Detail CSVs (cust 583424) from Downloads into sqlite via `sync-enterprise --shop-ro`. Join `Vehicle` = eFleets id. Oil lines: `IsOilChangeService` (lube oil filter / synthetic engine oil / oil change). Not oil pan, filter-only, surcharge, chassis lube.

Files (copied to `data/runtime/enterprise/`, gitignored): 12-month (2026-08-04), 90-day (2026-08-21), 30-day (2026-09-02). `last_oil_*` only advances. RO completed `-` (Under Shop Review) uses RO created date — Bing-2 179598 on 2026-08-30.

**189 / 205** cars have last oil. **16** have none in these exports (mostly new units). Last Reading still **146**. HOLDs unchanged (`NO_DEVICE` 55, `NO_TRUSTED_FILL` 4).

Portal `EFLEETS_MAINT_URL` is still the HTML tab, not a CSV. Next live pull needs a captured export URL or CDP download.

## GPS card match

`oilchange cards rebuild` pulls OneStep **stop** windows (drive-stop list, no odometer) for linked boxes, then assigns a swipe to a car only when **exactly one** GPS car was in a short stop (≤2h, ±20 min slack) at that swipe time. Overnight sits are ignored. Two cars stopped → no vote. Enterprise Vehicle is not the join.

Cache: `data/runtime/gps-stops.json` (gitignored). `--no-gps` rematches from cache without hitting OneStep.

2026-09-04 later run (real Drive `DETAILS_583424_30-Days` + live OneStep stops, May 16–Jun 17 window, then VIN-linked boxes filled into the cache): **35,404** stop windows (`with_pos=35389`), **1,421** pump clusters, **214** GPS-first matches, **63** BEST, **1,098** calls, **119** geocoded stations. Adding boxes reduced exclusive sits (251 → 214) and still raised known cars. Nearby `--live` later filled 260 uncovered boxes into the same gitignored cache (**72,074** visits / **1,737** pumps) without rewriting `card_eras`. Synthetic TRACKER 10:00 AM file-drop is no longer the coverage input.

## Station ladder (3 / 5 / 10) — this wave

`oilchange devices csv [--live] [--out data/runtime/onestep-devices.csv]` dumps `factory_id,device_id,display_name,linked_car_efleets_id,status`. `display_name` is a label only.

`oilchange cards ladder` / `cards coverage` climb exclusive GPS pump sits and persist `card_eras` (car / person / office). Driver-kept and logistics-personnel cards stay on the person and never create a device↔car join. `FLEET DRIVER` is a DETAILS placeholder, not a person.

**Known** = factory_id link **and** a GPS-named **car** card era. Target 95%.

This Cloud Agent live run (2026-09-04): file-drop Drive `DETAILS_583424_30-Days` (2,076 named punches, 0 TRACKER, 1,175 distinct times) + Automations sheet factory_id map (155 links) + `--devices-live`, then **exact OBD VIN** for unpaired boxes + GPS fill for 53 newly linked boxes (**39** had sight in the DETAILS window, **14** were dark). Do **not** invent punches. Matcher exclusive-sit rule was **not** loosened.

| Metric | Value |
|---|---|
| sqlite roster | 205 (fleetsummary_live) |
| device_link | **196 / 205 (95.6%)** |
| card_era | **63 / 205 (30.7%)** |
| known | **63 / 205 (30.7%)** |
| gps-first matches | 214 |
| ladder cars at 3/5/10 | (see `cards ladder`) |
| ladder locked | 58 |
| PERSON (driver-kept) cards | 75 |
| missing device | 9 (coverage; sqlite distinct = 8) |
| Last Reading written | no |

**Ceiling without more factory_id / VIN links:** 196/205 = **95.6%**. Remaining known gap is card eras (June 30-day window, exclusive-sit collisions, driver-kept PERSON cards), not empty GPS. Vehicles with **both GPS sight and API identity** are VIN-matched boxes that returned `HasPos` drive-stop windows — no Slack needed; `/device` `device_state.vin` is that identity.

**Live eFleets:** this pod injects `enterprise_login_name` / `enterprise_password` (PR #22). `oilchange env` shows username+password set. `EFLEETS_CUST_NUM` and `EFLEETS_DETAILS_URL` still missing. Login with portal field `userId` reaches `/fleetweb/mfaRegistration` — fail closed (no MFA in chat). HTTPAdapter still posts `username`/`j_username`; portal form is `userId`. CDP / captured export URL / a 90-day DETAILS file-drop is the live path.

Tests lock the ladder with synthetic exclusive pumps (`internal/cards/ladder_test.go`).

## Cards desk (car↔card)

`http://127.0.0.1:4739/cards/` — unknown matchups first, then mapped stations. Built from DETAILS swipes (`oilchange cards rebuild` writes `web/data/cards.json`). Majority score is evidence, not Enterprise last-write-wins. Last Reading is untouched.

90-day DETAILS ingest skipped punches for vehicles not on the roster (do not invent cars). Station key is merchant name + city/state. `TRACKER` / empty names dropped.

## Near-address hunt (2026-09-04)

Live generate-reports type **`near_address`** exists. Spec that actually echoes: `report_options_near_address.search_address_string` + `range: {value:1, unit:mi}`, `datetime_from`/`datetime_to` = Eastern **fill day ±1** (provider swipe, not bank posting), `all_user_devices` + `exclude_inactive_devices`, `time_zone=America/New_York`, JSON output fields including `near_address_factory_id`. Top-level `address`/`radius` are dropped. Public JSON download 404s; hunt rows use drive-stop stops at 1 mile. CLI: `oilchange cards nearby [--live] [--report] [--persist] [--report-cap N]`. GPS-called fills and car-era fills are skipped. Exclusive **Eastern days** (1=watch, 2=likely, 3=certain). Incomplete GPS coverage is watch-only. `--persist` writes **certain linked** car eras only; never unpaired boxes, never PERSON cards, never Last Reading. Method: `docs/collab/NEAR-ADDRESS.md`.

**OneStep support rate note** ([support.php](https://track.onestepgps.com/support.php) Data and Functionality): real-time API; recommend **15–30 s+** between routine pulls; **5,000 calls/hour** (~120k/day theoretical; large fleets often 5k–10k/day); prefer **batching multi-device** requests. `near_address` already batches via `all_user_devices`. Drive-stop is still **one `device_id` per GET** (no proven multi-id param) — nearby `--live` now paces ≥1 s between boxes + progress every 25; do not invent comma-lists. Do not spam `--report` (`--report-cap`). Do not re-fetch all 260 boxes unless drive-stop batching is proven.

**Live `--live` run (2026-09-04T18:55:55Z–19:16:20Z, exit 0, ~20.4 min).** Cache-only first: `certain=0 likely=0 watch=45 cards=20 coverage_complete=false`. Then mutex-serialized drive-stop for **260** boxes not covered in the fill-day window (no `--report`, no `--persist`). Summary artifact: `/opt/cursor/artifacts/nearby_live_summary.txt`.

| Metric | Cache-only | Live `--live` |
|---|---|---|
| certain / likely / watch | 0 / 0 / 45 | **0 / 1 / 47** |
| unknown cards | 20 | 20 |
| coverage_complete | false | **true** |
| gps-stops visits | 35,404 (`with_pos=35389`, pumps=1421) | 72,074 (`with_pos=71798`, pumps=1737) |
| car eras persisted | — | **0** (certain=0) |
| Last Reading written | no | no |

One **likely** (2 exclusive Eastern days at fill ±20 min; 3 would be certain): card `xxxxxxxxxxxxx57770` → `factory_id=7000335987` / `292NCX` (`fills=5 at_fill=2 exclusive=2 min_mi=0.21`). Keep watching. Two unpaired `factory_id`s showed up in the 1-mile list (`4572242789`, `3271251658`) and were **not** joined. 8 of 20 cards had no GPS box within 1 mile of a mapped pump. `cards coverage --no-gps` after the fetch: **known 62 / 205 (30.2%)**, device_link still **196 / 205 (95.6%)**. Nearby did not beat the VIN-wave **63 / 205**.

## Next (reasonable)

- Nearby live hunt is done for this 30-day DETAILS window: **0 certain**, 1 likely (`292NCX` / `7000335987` needs a 3rd exclusive fill day). Do not persist watches. A **90-day** DETAILS file-drop would add more exclusive days — that is the next card-era path, not loosening 1-mile to a join.
- File-drop a **90-day** (or current 30-day) Fuel DETAILS CSV — June 2026 30-day already proved the matcher. Do not ask for an eFleets password or MFA code in chat. Optional: `EFLEETS_DETAILS_URL` + CDP on an already-logged-in tab.
- Remaining unpaired / no-VIN boxes (~8–9 roster cars, plus 14 VIN-linked boxes with no GPS in this window). Do not join on `display_name`.
- Do not rewrite the GPS matcher to chase 95% — exclusive sit is the lock; more boxes correctly *reduce* exclusive votes.
- Enterprise DETAILS for the 4 `NO_TRUSTED_FILL` cars.
- Capture a real Maintenance Detail export URL (or CDP) so last oil is not stuck on Downloads CSVs.
- Optional: collapse stacked `hold_events` so event count matches cars on HOLD.
- Do not “fill in” Last Reading for those HOLDs.
- Later (not now): full sqlite-table copy onto this fleet’s Supabase so there is still a backup if Neon is down. Do not add S3, extra Neon, or direct AWS.

## Env (presence only)

Canonical file: repo-root `oilchange.env` (gitignored). Nested `Fleet-Management/oilchange.env` had a blank template above real secrets — first assignment wins (`setEnvIfEmpty`). Names: `ONE_STEP_FULL_API_KEY` / `ONESTEP_API_KEY`, OneStep PEMs, eFleets user/pass/cust (`EFLEETS_*` or Cloud Agent `EFleetsUsername` / `enterprise_login_name`, `EFleetsPassword` / `enterprise_password`, `EFleetsCustNum`), `SUPABASE_GROK_BUILD_KEY`, `DATABASE_URL` unpooled Neon, `OILCHANGE_DB`. This Cloud Agent injects OneStep plus `enterprise_login_name` / `enterprise_password`. Cust num and DETAILS URL are still missing; live portal then hits MFA.

`oilchange env` prints presence, never values.
