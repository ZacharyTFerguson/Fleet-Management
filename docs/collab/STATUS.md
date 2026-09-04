# Shared status

Updated: 2026-09-04 (UTC) after OneStep-first devices CSV + station ladder (3/5/10).

## True now

- Daily driver is sqlite (`OILCHANGE_DB=./oilchange.sqlite`). Neon `Fleet_Manage_Oil` on project `Fleet_Management_Neon` (`icy-thunder-13848536`, branch `production`) is backup only. Unpooled `DATABASE_URL` (no `-pooler`, not XRAY `chjqcznyxvtjbamttqdj`).
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

2026-09-04 (prior GPS-first run with named merchants): 117,547 stop windows, **15** GPS-best cars, **49** matched swipes, 9 of those BEST cards disagree with Enterprise.

## Station ladder (3 / 5 / 10) — this wave

`oilchange devices csv [--live] [--out data/runtime/onestep-devices.csv]` dumps `factory_id,device_id,display_name,linked_car_efleets_id,status`. `display_name` is a label only.

`oilchange cards ladder` / `cards coverage` climb exclusive GPS pump sits and persist `card_eras` (car / person / office). Driver-kept and logistics-personnel cards stay on the person and never create a device↔car join. `FLEET DRIVER` is a DETAILS placeholder, not a person.

**Known** = factory_id link **and** a GPS-named **car** card era. Target 95%.

This Cloud Agent live run (2026-09-04), `--no-gps` cache **107,118** stops / **148** GPS cars / **2,503** pump clusters:

| Metric | Value |
|---|---|
| sqlite roster | 207 (205 live + 2 demo) |
| device_link | **148 / 207 (71.5%)** |
| card_era | **0 / 207 (0.0%)** |
| known | **0 / 207 (0.0%)** |
| ladder cars at 3/5/10 | 0 |
| missing device | 59 |
| Last Reading written | no |

**Blocked:** file-drop `details_live.csv` (and this sqlite `card_transactions`) is **201 TRACKER + 3 named demo SHELL/MARATHON**. TRACKER is not a pump. Do not invent punches. A real 90-day DETAILS export with merchant names is required before unknown cars can drop. GPS cache is not empty; the join is waiting on named stations.

Tests lock the ladder with synthetic exclusive pumps (`internal/cards/ladder_test.go`).

## Cards desk (car↔card)

`http://127.0.0.1:4739/cards/` — unknown matchups first, then mapped stations. Built from DETAILS swipes (`oilchange cards rebuild` writes `web/data/cards.json`). Majority score is evidence, not Enterprise last-write-wins. Last Reading is untouched.

90-day DETAILS ingest skipped punches for vehicles not on the roster (do not invent cars). Station key is merchant name + city/state. `TRACKER` / empty names dropped.

## Next (reasonable)

- Capture a real Fuel & Charging DETAILS export (not TRACKER placeholders) so the 3/5/10 ladder can name cards. File-drop still works; do not ask for eFleets login in chat.
- Map the remaining unpaired OneStep boxes (40 unlinked of 188) onto the 59 roster cars without a factory_id, then `devices sync --map` + `sync-onestep --map` + `compute`. Do not join on `display_name`.
- Enterprise DETAILS for the 4 `NO_TRUSTED_FILL` cars.
- Capture a real Maintenance Detail export URL (or CDP) so last oil is not stuck on Downloads CSVs.
- Optional: collapse stacked `hold_events` so event count matches cars on HOLD.
- Do not “fill in” Last Reading for those HOLDs.
- Later (not now): full sqlite-table backup onto this fleet’s Supabase so Neon/AWS down is not the only copy. Do not add AWS S3/RDS/extra Neon.

## Env (presence only)

Canonical file: repo-root `oilchange.env` (gitignored). Nested `Fleet-Management/oilchange.env` had a blank template above real secrets — first assignment wins (`setEnvIfEmpty`). Names: `ONE_STEP_FULL_API_KEY` / `ONESTEP_API_KEY`, OneStep PEMs, eFleets user/pass/cust (`EFLEETS_*` or Cloud Agent `EFleetsUsername` / `EFleetsPassword` / `EFleetsCustNum`), `SUPABASE_GROK_BUILD_KEY`, `DATABASE_URL` unpooled Neon, `OILCHANGE_DB`. This Cloud Agent currently injects OneStep only — eFleets keys are missing until added as secrets or written locally.

`oilchange env` prints presence, never values.
