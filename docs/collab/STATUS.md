# Shared status

Updated: 2026-09-04 (UTC) after live drive-stop → compute → Oil Desk mirror.

## True now

- Daily driver is sqlite (`OILCHANGE_DB=./oilchange.sqlite`). Neon `Fleet_Manage_Oil` on project `Fleet_Management_Neon` (`icy-thunder-13848536`, branch `production`) is backup only (AWS). Unpooled `DATABASE_URL` (no `-pooler`, not XRAY `chjqcznyxvtjbamttqdj`). Do not add S3 or another AWS store. This fleet’s Supabase is the planned second full backup later; today `sync` is Oil Desk `fleet_cars` only.
- Oil Desk: `oilchange serve` at `http://127.0.0.1:4739`, mirror `web/data/cars.json` (`source=mock-mirror`, synced ~2026-09-04T03:03:06Z).
- Roster: **205** cars. **146** Last Reading (fill odo + drive-stop miles). **189** last oil from Enterprise shop ROs. **0** `NO_DRIVESTOP`. Open HOLDs: **55** `NO_DEVICE`, **4** `NO_TRUSTED_FILL`.
- Live OneStep map: `data/runtime/onestep-map.csv` (gitignored) — 146 `factory_id,device_id,efleets_id`. Do **not** use `testdata/onestep/map.csv` on this roster.
- Drive-stop was HTTP 500 on invented params `factory_id` + `from`. Live names: `device_id`, `dt_tracker_from`, `dt_tracker_to`, `stop_duration=5m0s`. Do not send `return_points`. Miles on `distance.value` (+ `unit`); ignore `odometer_from` / `odometer_to`. Smoke: factory `3271116717` 48h = **263.66** miles. Fleet fetch: **146/146** stored, including measured 0. Code: `internal/onestep/client.go`.
- `oilchange probe-onestep` is the one-shot live GET (no Last Reading write). 2026-09-04 jwt-rs256: Bing-2 6h/24h/48h = 0 / 161.10 / **263.66**; three other boxes 48h = 293.85 / 322.13 / 0. See `docs/collab/DRIVE-STOP-LIVE-PROBES.md`. Drive-stop HTTP is mutex-serialized.
- Example: eFleets `26LSZW` (Bing-2) Last Reading **180312**, no HOLD.
- Neon backup after this wave: cars=205, devices=186, miles=146.
- `go test ./...` green after the drive-stop client change.
- `hold_events` still has stacked open rows for cars that remain on HOLD (sync reported holds=236 events, 59 cars). `oilchange holds` / car `hold_reason` are the operator surface.

## Do not

- Invent miles or use OneStep odometer / `odometer_from` / `odometer_to`.
- Join on `display_name`.
- Use `testdata/onestep/map.csv` as the live roster.
- Push `sync --mirror` to Supabase unless asked (this wave used `--no-remote`).
- Recreate the Neon project or point `DATABASE_URL` at pooled / XRAY / supabase.co.
- Ask Zachary to paste the OneStep key. It is already in `oilchange.env`.
- Ask Zachary to log into eFleets or paste `EFLEETS_PASSWORD` in chat. Put `EFLEETS_USERNAME` / `EFLEETS_PASSWORD` / `EFLEETS_CUST_NUM` in gitignored `oilchange.env` or Cloud Agent secrets (`EFleetsUsername` / `EFleetsPassword` / `EFleetsCustNum`).

## Last oil (Enterprise shop RO)

Ingested eFleets Maintenance Detail CSVs (cust 583424) from Downloads into sqlite via `sync-enterprise --shop-ro`. Join `Vehicle` = eFleets id. Oil lines: `IsOilChangeService` (lube oil filter / synthetic engine oil / oil change). Not oil pan, filter-only, surcharge, chassis lube.

Files (copied to `data/runtime/enterprise/`, gitignored): 12-month (2026-08-04), 90-day (2026-08-21), 30-day (2026-09-02). `last_oil_*` only advances. RO completed `-` (Under Shop Review) uses RO created date — Bing-2 179598 on 2026-08-30.

**189 / 205** cars have last oil. **16** have none in these exports (mostly new units). Last Reading still **146**. HOLDs unchanged (`NO_DEVICE` 55, `NO_TRUSTED_FILL` 4).

Portal `EFLEETS_MAINT_URL` is still the HTML tab, not a CSV. Next live pull needs a captured export URL or CDP download.

## GPS card match

`oilchange cards rebuild` pulls OneStep **stop** windows (drive-stop list, no odometer) for linked boxes, then assigns a swipe to a car only when **exactly one** GPS car was in a short stop (≤2h, ±20 min slack) at that swipe time. Overnight sits are ignored. Two cars stopped → no vote. Enterprise Vehicle is not the join.

Cache: `data/runtime/gps-stops.json` (gitignored). `--no-gps` rematches from cache without hitting OneStep.

2026-09-04: 117,547 stop windows, **15** GPS-best cars, **49** matched swipes, 9 of those BEST cards disagree with Enterprise.

## Cards desk (car↔card)

`http://127.0.0.1:4739/cards/` — unknown matchups first, then mapped stations. Built from DETAILS swipes (`oilchange cards rebuild` writes `web/data/cards.json`). Majority score is evidence, not Enterprise last-write-wins. Last Reading is untouched.

90-day DETAILS ingest skipped punches for vehicles not on the roster (do not invent cars). Station key is merchant name + city/state. `TRACKER` / empty names dropped.

## Next (reasonable)

- Map the 55 `NO_DEVICE` cars (sheet / OneStep inventory), then `devices sync --map` + `sync-onestep --map` + `compute`.
- Enterprise DETAILS for the 4 `NO_TRUSTED_FILL` cars.
- Capture a real Maintenance Detail export URL (or CDP) so last oil is not stuck on Downloads CSVs.
- Do not “fill in” Last Reading for those 59.
- Later (not now): full sqlite-table backup onto this fleet’s Supabase so Neon/AWS down is not the only copy. Do not add AWS S3/RDS/extra Neon.

## Env (presence only)

Canonical file: repo-root `oilchange.env` (gitignored). Nested `Fleet-Management/oilchange.env` had a blank template above real secrets — first assignment wins (`setEnvIfEmpty`). Names: `ONE_STEP_FULL_API_KEY` / `ONESTEP_API_KEY`, OneStep PEMs, eFleets user/pass/cust (`EFLEETS_*` or Cloud Agent `EFleetsUsername` / `EFleetsPassword` / `EFleetsCustNum`), `SUPABASE_GROK_BUILD_KEY`, `DATABASE_URL` unpooled Neon, `OILCHANGE_DB`. This Cloud Agent currently injects OneStep only — eFleets keys are missing until added as secrets or written locally.

`oilchange env` prints presence, never values.
