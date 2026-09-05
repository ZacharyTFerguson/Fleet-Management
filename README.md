# oilchange

PDI fleet oil-change binary in **Fleet-Management**. Last Reading is computed only in `internal/oil`.
Do not invent miles. HOLD skips the Last Reading write.

Project purpose, vehicle/card identity model, and completion review:
[`docs/FLEET-MANAGEMENT-OVERVIEW.md`](docs/FLEET-MANAGEMENT-OVERVIEW.md).

LLM intro (goal, what it does, locks): [`docs/FOR-LLMS.md`](docs/FOR-LLMS.md).

Canon Place names and catalog lifecycle: [`docs/CANON-PLACE-AND-CATALOG-RULES.md`](docs/CANON-PLACE-AND-CATALOG-RULES.md).

```
go build -o oilchange ./cmd/oilchange
go test ./...
```

Toolchain: Go 1.27.1.

OneStep live auth: short-lived RS256 JWT when a PEM is present in gitignored `oilchange.env`; otherwise `api-key` query (see [`docs/onestep-api-auth.md`](docs/onestep-api-auth.md)).

Sit-still / important-location Node app lives in a separate PR (`cursor/important-location-trigger-61d4`); it is intentionally not bundled here.

## Commands

| Cmd | What |
|---|---|
| `sync-enterprise` | Live eFleets if `EFLEETS_*` is set, or `--vehicles` `--fuel-details` `[--shop-ro]` `[--mileage-history]`. Oil/lube shop ROs seed last oil. Does not compute Last Reading. |
| `sync-onestep` | OneStep API devices + drive-stop miles-since after the trusted fill second. Join `factory_id` only. Optional `--map PATH`. |
| `devices sync` | Upsert durable `onestep_devices` registry (`factory_id` PK, `device_id` history id, `display_name` label only). Optional `--map PATH`. `--information PATH` applies a saved Device Information JSON (no live `/device`). Unpaired boxes may attach by exact 17-char OBD VIN (`device_state.vin` = `cars.vin`). Does not fetch miles. |
| `devices list` | Print registry rows (status + optional `efleets_id` car link). `--csv` writes the inventory CSV. |
| `devices csv` | Write `factory_id,device_id,display_name,linked_car_efleets_id,status`. `display_name` is a label only. `[--out PATH]` `[--live]` `[--map PATH]`. `--live` refreshes from OneStep if a token is present (no miles). |
| `devices vin` | Ask OneStep `GET /device?device_id=&latest_point=true` for OBD VIN on unpaired boxes; join exact 17-char `device_state.vin` = `cars.vin`. `[--factory-id ID]` `[--pace 35s]`. `--from PATH` reads a saved Device Information JSON instead (default drop: `data/runtime/device-information.json`) — no live `/device`. Never display_name. Never Last Reading. |
| `compute` | Last Reading + HOLD. `[--override-lower]` is the only way to write a lower reading. |
| `oil-done` | `--efleets-id ID --miles N --date YYYY-MM-DD [--location NAME]` |
| `report` | `[--interval 5000] [--due-within N] [--out PATH.csv]` |
| `holds` | open HOLDs |
| `sync` | Push local SQLite cars/holds to **ZacharyTFerguson's Project** (`hdtwfdjdvdzdxfdriyzn`, table `fleet_cars`) when `SUPABASE_URL` + (`SUPABASE_SERVICE_ROLE` or `SUPABASE_SYNC_SECRET`) are set, and refresh `web/data/cars.json`. `[--interval 5m]` for throughout-the-day refresh. Never targets XRAY. After a successful sync, also runs a **best-effort** Neon backup when `DATABASE_URL` is set (`[--require-neon]` to fail if backup is down). |
| `pull-supabase` | GET `fleet_cars` from Zachary’s project using `SUPABASE_GROK_BUILD_KEY` (or service role). Merges non-null last_reading/HOLD into sqlite. Does not compute Last Reading. Never XRAY. |
| `backup-neon` | Copy sqlite oilchange tables into **Neon** (`Fleet_Management_Neon` / `Fleet_Manage_Oil`). SQLite stays the working store. Alias: `backup`. |
| `serve` | Host Oil Desk UI + `/api/cars` on `127.0.0.1:4739` from the **embedded** static export (`web/out`). **No npm/Node required.** `[--addr]` `[--mirror]` `[--web-dir]` `[--app]` `[--start /history/]`. **History** (`/history/`): one fill is one block; drag onto a car; SQLite `assignment_events` logs PDI-0003 → PDI-0020. Rebuilds do not wipe owner calls. **Devices** is GPS: cached evidence + one-box / one-fill live probe. Button **Apply saved OneStep device information** reads `data/runtime/device-information.json` (no live `/device`). Phone: Share → Add to Home Screen (standalone PWA). |
| `desk` | Same as `serve --app`: opens a Chrome/Edge window with no address bar. Secrets stay in `oilchange.env`, not the UI. |
| `cards rebuild` | ingest optional `--fuel-details` then score every swipe into `card_pairings` (never writes Last Reading). GPS-first: stop windows from OneStep unless `--no-gps` (rematch from `data/runtime/gps-stops.json`). Station lat/lng from exclusive GPS sits; later swipes match the car at that pump even when another box is sitting elsewhere. |
| `cards history` | **One operator path** for card/vehicle history: devices CSV → ask OneStep OBD VIN on unpaired boxes (`device_state.vin` = `cars.vin`) → ingest DETAILS (file or live `EFLEETS_*` in env) → GPS stops + `cards rebuild` → ladder 3/5/10 → persist `card_eras` → print coverage. `[--vehicles PATH]` `[--fuel-details PATH]` `[--devices-live]` `[--map PATH]` `[--devices-out PATH]` `[--no-gps]`. Never Last Reading. Never `display_name`. |
| `cards suspect` | cards whose latest Enterprise Vehicle is not the swipe-majority / GPS-called car |
| `cards trace` | `--card ID [--window-days 2]` other cars at the same station on nearby days |
| `cards pairings` | `[--card ID]` scored car/person links; `BEST` is evidence, not Enterprise last-write-wins |
| `cards split` | GPS eras: which vehicle each card was in over time (`SPLIT` when one card sat in two cars). Driver-kept / logistics cards print as `PERSON`. |
| `cards call` | what to call each swipe (`VA15`, not the DETAILS Vehicle column). Default: only rows where GPS ≠ Enterprise. `--all` prints every GPS-named swipe. |
| `cards ladder` | Exclusive GPS pump sits → cards at 3, then 5, then 10 stations. Buckets Cars / People / Offices. Persists `card_eras`. Prints coverage (device link + GPS-named card era). Never Last Reading. |
| `cards coverage` | Coverage one-liner only (`cards ladder` metric). |
| `cards nearby` | Fill-day ±1, 1 mile: watch GPS boxes at unknown-card pumps (provider swipe time, not bank posting). `--live` / `--report` / `--persist`. Never Last Reading. |
| `cards watch` | Loop unknown cards: newest `--fills` (default 10) punches, **watched boxes only**, `--pace 35s` (Retry-After wins). Virginia recorded vehicles seed which `factory_id` to ask about. `--live` / `--persist`. Do not re-run a 260-box nearby `--live`. |
| `env` | which `oilchange.env` keys loaded (presence only; never prints values) |

Exit: `0` ok, `1` error, `2` compute finished with open HOLDs (report still allowed).


## Run on desktop (no npm)

**Supabase target:** [ZacharyTFerguson's Project](https://supabase.com/dashboard/project/hdtwfdjdvdzdxfdriyzn) — ref `hdtwfdjdvdzdxfdriyzn`, host `https://hdtwfdjdvdzdxfdriyzn.supabase.co` (us-east-2). Fleet oil uses `fleet_*` tables only. **Never** write oil/fleet data to XRAY (`chjqcznyxvtjbamttqdj`).

Desktop only needs **Go** (or a prebuilt `oilchange` binary) plus env files. The Oil Desk UI is a **committed static export** under `web/out`, embedded into the binary. You do **not** need Node/npm on the machine that runs the desk.

```bash
# 0) One-time: copy env template (fill secrets locally; never commit them)
cp oilchange.env.example oilchange.env
# Edit oilchange.env: OILCHANGE_DB, optional SUPABASE_URL + SUPABASE_SERVICE_ROLE or SUPABASE_SYNC_SECRET
# Without secrets the CLI still writes a mock mirror and serve reads /api/cars from it.

# 1) Build the backend (embeds web/out — no npm)
go build -o bin/oilchange ./cmd/oilchange
# Or download a release binary for your OS and skip this step.

# 2) Load sample Enterprise CSVs → SQLite, compute, refresh the mirror
export OILCHANGE_DB=./oilchange.sqlite
./bin/oilchange sync-enterprise \
  --vehicles testdata/enterprise/fleetsummary.csv \
  --fuel-details testdata/enterprise/details.csv \
  --shop-ro testdata/enterprise/maintenance.csv
./bin/oilchange compute || true   # exit 2 with open HOLDs is OK without OneStep miles
./bin/oilchange sync --mirror web/data/cars.json
# Optional all-day refresh (separate terminal):
# ./bin/oilchange sync --interval 5m --mirror web/data/cars.json

# 3) Start Oil Desk (http://127.0.0.1:4739) — static UI + /api/cars, no npm
./bin/oilchange serve --addr 127.0.0.1:4739 --mirror web/data/cars.json
```

Binary path after build: `bin/oilchange` (Windows: `bin/oilchange.exe`). Alias: `sync-supabase` ≡ `sync`.

For the full ~205-car roster, use `testdata/enterprise/fleetsummary_live.csv` and `details_live.csv` instead of the tiny demo CSVs.

### Windows (PowerShell)

Same stack: Go only, no npm. `go build` embeds `web/out`. Open **http://127.0.0.1:4739** on this PC.

```powershell
Copy-Item oilchange.env.example oilchange.env
# Edit oilchange.env: OILCHANGE_DB, optional SUPABASE_URL + SUPABASE_SERVICE_ROLE
# or SUPABASE_SYNC_SECRET. Never commit oilchange.env.

go build -o bin/oilchange.exe ./cmd/oilchange

$env:OILCHANGE_DB = "$PWD\oilchange.sqlite"
.\bin\oilchange.exe sync-enterprise `
  --vehicles testdata\enterprise\fleetsummary_live.csv `
  --fuel-details testdata\enterprise\details_live.csv `
  --shop-ro testdata\enterprise\maintenance.csv
.\bin\oilchange.exe compute   # exit 2 with open HOLDs is OK without OneStep miles
.\bin\oilchange.exe sync --mirror web\data\cars.json
.\bin\oilchange.exe serve --addr 127.0.0.1:4739 --mirror web\data\cars.json
```

Process env set with `$env:NAME = "..."` wins over `oilchange.env`. Cross-compile from another OS with `GOOS=windows go build -o bin/oilchange.exe ./cmd/oilchange`.

### Optional: Next.js cloud/dev (npm only when editing the UI)

```bash
cp web/.env.local.example web/.env.local
# NEXT_PUBLIC_SUPABASE_* for live PostgREST reads in next dev
cd web && npm ci && npm run dev          # App Router + /api/cars in Node
# Rebuild the committed static export after UI changes:
# cd web && npm ci && npm run build:static && cd .. && go build -o bin/oilchange ./cmd/oilchange
```

### Env

| Where | Vars |
|---|---|
| CLI (`oilchange.env`) | `OILCHANGE_DB` (working sqlite), `DATABASE_URL` (Neon **unpooled** backup only — not the DSN while sqlite is set), `SUPABASE_URL`, then either `SUPABASE_SERVICE_ROLE` (PostgREST upsert into `fleet_cars`) or `SUPABASE_SYNC_SECRET` (`/functions/v1/fleet-sync`). Optional `FLEET_MIRROR_PATH`. Live eFleets: `EFLEETS_USERNAME` / `EFLEETS_PASSWORD` / `EFLEETS_CUST_NUM` (Cloud Agent aliases `EFleetsUsername` / `enterprise_login_name`, `EFleetsPassword` / `enterprise_password`, `EFleetsCustNum`). Never paste those into chat. |
| Web (`web/.env.local`) | Only needed for `npm run dev`. `NEXT_PUBLIC_SUPABASE_URL`, `NEXT_PUBLIC_SUPABASE_ANON_KEY`. Never put service role / sync secret in `NEXT_PUBLIC_*`. Templates: `oilchange.env.example`, `web/.env.local.example`. |

Without Supabase credentials the CLI writes a **mock mirror** at `web/data/cars.json` and `oilchange serve` (or Next `/api/cars`) serves that. With credentials, sync upserts into `fleet_cars`. Schema/RLS: `supabase/migrations/` + `migrations/005_shared_project_fleet_prefix.sql` (anon SELECT on `fleet_cars` only).

## Neon backup

SQLite (`OILCHANGE_DB`) is still the working store. `oilchange sync` still pushes Oil Desk cars to Supabase `fleet_cars`. Neon (`DATABASE_URL`, unpooled / no `-pooler`) is a durable copy of the sqlite tables — run `oilchange backup-neon` (also attempted after a successful `sync` when `DATABASE_URL` is set). Later the same fleet Supabase project is a second **full copy** if Neon is down — not implemented yet (`sync` is still desk `fleet_cars`). Do not add S3, extra Neon, or direct AWS. Never point `DATABASE_URL` at XRAY or Supabase.

```powershell
.\bin\oilchange.exe backup-neon
```

Cadence: `oilchange sync --interval 5m` keeps the mirror (and Supabase) fresh while the UI polls (`NEXT_PUBLIC_REFRESH_MS`, default 120s; baked into the static export at build time).

## Secrets (never commit)

Paste live values into **`oilchange.env`** in this directory (gitignored), or into **Cloud Agent secrets** with the same names. Template: `oilchange.env.example`. Never paste passwords into chat, PRs, or Slack.

The binary loads `oilchange.env` on startup (or `OILCHANGE_ENV`, `secrets/oilchange.env`, `.env`). Already-set process env (including Cloud Agent injection) wins.

`EFLEETS_CUST_NUM` has **no** hardcoded default — set it in env or Cloud Agent secrets for live eFleets sync. File-drop `--vehicles` / `--fuel-details` does not need a portal login.

Tests never hit live eFleets, OneStep, or Supabase.

## OneStep devices registry

`onestep_devices` is the durable GPS-box store. Join cars on **`factory_id`** (and optional `linked_car_efleets_id` / `linked_car_pdi_id`). `device_id` is history identity; **`display_name` is never a join key**. Lifecycle: `active` / `dead` / `retired_at`, plus `last_synced_at` and timestamps. Ingest: `oilchange devices sync --map testdata/onestep/map.csv` or live API; `sync-onestep` also upserts then fetches drive-stop miles. When OneStep is rate-limited, save the portal **Device Information** JSON to gitignored `data/runtime/device-information.json` and apply it with `oilchange devices vin --from data/runtime/device-information.json` or the Oil Desk button (Cards / Devices). That apply is file-parse + sqlite upsert — do not spam live `GET /device`. Pairs to cars the same way Last Reading compute already does: devices linked by `efleets_id`, never by tracker label.

## Bad-data / fuel-card debug

Enterprise `cards.linked_car_*` is last-write-wins and is often wrong (card used on the wrong car). `card_transactions` keeps every swipe. Pairings are scored from that history; they never write G/H remaining/due or U/V pairing columns.

**SYNTHETIC wrong-card fixture** lands with the cards intel add-on (not in this devices commit).

```bash
export OILCHANGE_DB=./oilchange.sqlite
go build -o bin/oilchange ./cmd/oilchange

# stock Last Reading path (HOLDs are expected without live OneStep drive-stop miles)
./bin/oilchange sync-enterprise \
  --vehicles testdata/enterprise/fleetsummary.csv \
  --fuel-details testdata/enterprise/details.csv \
  --shop-ro testdata/enterprise/maintenance.csv
./bin/oilchange sync-onestep --map testdata/onestep/map.csv
./bin/oilchange compute   # exit 2 if HOLDs remain; Last Reading is not written on HOLD
./bin/oilchange holds
./bin/oilchange report

# card intelligence (wrong-car / station-day trace)
./bin/oilchange sync-enterprise \
  --vehicles testdata/enterprise/fleetsummary_wrongcard.csv \
  --fuel-details testdata/enterprise/details_wrongcard.csv
./bin/oilchange cards rebuild
./bin/oilchange cards suspect
./bin/oilchange cards trace --card CARD-MIX-99
./bin/oilchange cards pairings --card CARD-MIX-99
```

Heuristic: best car = 1.0 per swipe on that eFleets id, +0.25 if the swipe is within 30 days. Trace joins other swipes at `lower(station name)|lower(address)` within ±`--window-days` (default 2).

### Card history finder (one command)

Reconstruct where each fuel card sat (car eras, driver-kept person eras, office cards) from OneStep GPS + eFleets DETAILS. Does **not** write Last Reading. TRACKER merchants yield 0% card eras until a named DETAILS export is ingested — that is honest, not a bug.

```bash
export OILCHANGE_DB=./oilchange.sqlite
go build -o bin/oilchange ./cmd/oilchange

# File-drop (CI / desktop without live portals)
./bin/oilchange cards history \
  --vehicles testdata/enterprise/fleetsummary_live.csv \
  --fuel-details testdata/enterprise/details_live.csv \
  --map data/runtime/onestep-map.csv \
  --no-gps

# Live when tokens/secrets are already in oilchange.env (never prompts for a password)
./bin/oilchange cards history --devices-live --map data/runtime/onestep-map.csv
```

Steps inside `cards history`: (1) optional `devices sync` + write `data/runtime/onestep-devices.csv`, (2) ask OneStep OBD VIN on unpaired boxes and join exact 17-char `device_state.vin` = `cars.vin`, (3) ingest DETAILS, (4) refresh `gps-stops.json` + `cards rebuild` (pairings + `card_eras`, keep watch-persisted car eras), (5) station ladder 3 → 5 → 10 with coverage %. Use `cards split --card ID` or `cards coverage` after a run.

HOLD `LOGISTICS_PERSONNEL` (third-spec name `RICH_TYLER_PAIRING`): logistics-personnel names on a punch or OneStep label never create a device↔car link.

## Accidental twin

See [`docs/ACCIDENTAL-TWIN.md`](docs/ACCIDENTAL-TWIN.md) — do not use `fleetmanager` / `fleetmanagerlocalvscode`.
