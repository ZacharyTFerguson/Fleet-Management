# Shared status

Updated: 2026-09-04 (UTC) after real named DETAILS file-drop + live GPS (known 25.4%, not 95%).

## True now

- Daily driver is sqlite (`OILCHANGE_DB=./oilchange.sqlite`). Neon `Fleet_Manage_Oil` on project `Fleet_Management_Neon` (`icy-thunder-13848536`, branch `production`) is the backup. Unpooled `DATABASE_URL` (no `-pooler`, not XRAY `chjqcznyxvtjbamttqdj`). Do not add S3, extra Neon, or direct AWS. This fleet’s Supabase is Oil Desk today; later a second full copy if Neon is down; today `sync` is Oil Desk `fleet_cars` only.
- Oil Desk: `oilchange serve` at `http://127.0.0.1:4739`, mirror `web/data/cars.json` (`source=mock-mirror`, synced ~2026-09-04T03:03:06Z).
- Roster: **205** cars on the operator desktop. **146** Last Reading (fill odo + drive-stop miles). **189** last oil from Enterprise shop ROs. **0** `NO_DRIVESTOP`. Open HOLDs: **55** `NO_DEVICE`, **4** `NO_TRUSTED_FILL`. This Cloud Agent sqlite is a file-drop mix (205 live ids + demo `27TESTA`/`27TESTB` = **207** rows) and does **not** hold those Last Reading values — do not copy an empty agent db over the desktop sqlite.
- Live OneStep map: `data/runtime/onestep-map.csv` (gitignored) — 146 `factory_id,device_id,efleets_id`. Live `devices csv --live` (2026-09-04): **188** boxes, **148** with a car link, **40** unpaired. Do **not** use `testdata/onestep/map.csv` on this roster.
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

2026-09-04 later run (real Drive `DETAILS_583424_30-Days` + live OneStep stops, May 16–Jun 17 window): **28,583** stop windows (`with_pos=28582`), **1,187** pump clusters, **251** GPS-first matches, **58** BEST, **1,172** calls, **138** geocoded stations. Synthetic TRACKER 10:00 AM file-drop is no longer the coverage input.

## Station ladder (3 / 5 / 10) — this wave

`oilchange devices csv [--live] [--out data/runtime/onestep-devices.csv]` dumps `factory_id,device_id,display_name,linked_car_efleets_id,status`. `display_name` is a label only.

`oilchange cards ladder` / `cards coverage` climb exclusive GPS pump sits and persist `card_eras` (car / person / office). Driver-kept and logistics-personnel cards stay on the person and never create a device↔car join. `FLEET DRIVER` is a DETAILS placeholder, not a person.

**Known** = factory_id link **and** a GPS-named **car** card era. Target 95%.

This Cloud Agent live run (2026-09-04): file-drop Drive `DETAILS_583424_30-Days` (2,076 named punches, 0 TRACKER, 1,175 distinct times) + Automations sheet factory_id map (155 links) + `--devices-live`. Do **not** invent punches. Matcher was **not** rewritten — unknown fell.

| Metric | Value |
|---|---|
| sqlite roster | 205 (fleetsummary_live) |
| device_link | **155 / 205 (75.6%)** |
| card_era | **52 / 205 (25.4%)** |
| known | **52 / 205 (25.4%)** |
| gps-first matches | 251 |
| ladder cars at 3/5/10 | 4 / 1 / 0 |
| ladder locked | 39 |
| PERSON (driver-kept) cards | 96 |
| missing device | 50 |
| Last Reading written | no |

**Ceiling without more factory_id links:** 155/205 = **75.6%**. 95% known is impossible until ~40 more boxes are mapped (not via `display_name`). Remaining card-era gap is the June 30-day window plus driver-kept PERSON cards, not empty GPS.

**Live eFleets:** this pod injects `enterprise_login_name` / `enterprise_password` (PR #22). `oilchange env` shows username+password set. `EFLEETS_CUST_NUM` and `EFLEETS_DETAILS_URL` still missing. Login with portal field `userId` reaches `/fleetweb/mfaRegistration` — fail closed (no MFA in chat). HTTPAdapter still posts `username`/`j_username`; portal form is `userId`. CDP / captured export URL / a 90-day DETAILS file-drop is the live path.

Tests lock the ladder with synthetic exclusive pumps (`internal/cards/ladder_test.go`).

## Cards desk (car↔card)

`http://127.0.0.1:4739/cards/` — unknown matchups first, then mapped stations. Built from DETAILS swipes (`oilchange cards rebuild` writes `web/data/cards.json`). Majority score is evidence, not Enterprise last-write-wins. Last Reading is untouched.

90-day DETAILS ingest skipped punches for vehicles not on the roster (do not invent cars). Station key is merchant name + city/state. `TRACKER` / empty names dropped.

## Next (reasonable)

- File-drop a **90-day** (or current 30-day) Fuel DETAILS CSV — June 2026 30-day already proved the matcher. Do not ask for an eFleets password or MFA code in chat. Optional: `EFLEETS_DETAILS_URL` + CDP on an already-logged-in tab.
- Map the remaining unpaired OneStep boxes (~38 unlinked of 193 live) onto the **50** roster cars without a factory_id, then `devices sync --map`. Do not join on `display_name`. Known cannot pass 75.6% until those links exist.
- Do not rewrite the GPS matcher to chase 95% — unknown already fell with staggered real punch times.
- Enterprise DETAILS for the 4 `NO_TRUSTED_FILL` cars.
- Capture a real Maintenance Detail export URL (or CDP) so last oil is not stuck on Downloads CSVs.
- Optional: collapse stacked `hold_events` so event count matches cars on HOLD.
- Do not “fill in” Last Reading for those HOLDs.
- Later (not now): full sqlite-table copy onto this fleet’s Supabase so there is still a backup if Neon is down. Do not add S3, extra Neon, or direct AWS.

## Env (presence only)

Canonical file: repo-root `oilchange.env` (gitignored). Nested `Fleet-Management/oilchange.env` had a blank template above real secrets — first assignment wins (`setEnvIfEmpty`). Names: `ONE_STEP_FULL_API_KEY` / `ONESTEP_API_KEY`, OneStep PEMs, eFleets user/pass/cust (`EFLEETS_*` or Cloud Agent `EFleetsUsername` / `enterprise_login_name`, `EFleetsPassword` / `enterprise_password`, `EFleetsCustNum`), `SUPABASE_GROK_BUILD_KEY`, `DATABASE_URL` unpooled Neon, `OILCHANGE_DB`. This Cloud Agent injects OneStep plus `enterprise_login_name` / `enterprise_password`. Cust num and DETAILS URL are still missing; live portal then hits MFA.

`oilchange env` prints presence, never values.
