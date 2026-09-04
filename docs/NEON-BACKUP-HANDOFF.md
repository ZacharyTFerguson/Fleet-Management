# Handoff: oilchange → Neon Postgres backup

Paste this whole file into another Grok in workspace `C:\Users\Zacha\go\local9_3_2026`.

## Goal

Keep **oilchange** (the PDI fleet oil-change CLI under Fleet-Management) using **local SQLite as the working store**. Add **Neon Postgres as a backup** of that SQLite data.

This is **not**:

- Replacing SQLite as the daily driver
- Replacing the existing Supabase `oilchange sync` path (UI + `fleet_cars`)
- Changing Last Reading, HOLD, or Enterprise/OneStep ingest
- Writing fleet data to XRAY Supabase (`chjqcznyxvtjbamttqdj`)

Backup means: after local compute/sync, a durable copy of oilchange tables lives in Neon so the desktop SQLite file is not the only copy.

## Workspace

- **Work in repo root:** `C:\Users\Zacha\go\local9_3_2026`
- GitHub source of truth: `https://github.com/ZacharyTFerguson/Fleet-Management`
- Current branch (when this was written): `cursor/desktop-fe-be-supabase-de76`
- **Ignore** the nested untracked copy `Fleet-Management/` unless you are explicitly syncing it. Implement in the **root** tree (`cmd/oilchange`, `internal/`, `migrations/`).
- Toolchain: Go 1.27.1. UI is a committed static export in `web/out`; desktop does not need npm.

## Current data flow (do not break)

```
eFleets CSVs / live  ─┐
OneStep GPS          ─┼─► oilchange CLI ─► SQLite (OILCHANGE_DB, default ./oilchange.sqlite)
oil-done / compute   ─┘         │
                                ├─► oilchange sync ─► Supabase fleet_cars (hdtwfdjdvdzdxfdriyzn)
                                │                 └─► web/data/cars.json (Oil Desk /api/cars)
                                └─► NEW: backup ─► Neon Postgres (this task)
```

- Last Reading is computed **only** in `internal/oil`. Do not invent miles. HOLD skips the Last Reading write.
- `config.DSN()` **prefers SQLite** when `OILCHANGE_DB` is set, else `DATABASE_URL` + pgx. Daily commands must keep using SQLite.
- `store.Open` already speaks **sqlite** (tests/desktop) and **pgx** (Postgres). Migrations in `migrations/*.sql` already apply to pgx, including `002_rls.sql` on Postgres only.
- `oilchange` loads **`oilchange.env`** (gitignored), not `.env.local`. Process env wins if already set.

## Neon project (already set up)

Do **not** create a new Neon project. Use this one.

| | |
|---|---|
| Project name | `Fleet_Management_Neon` |
| Project id | `icy-thunder-13848536` |
| Org | `org-cold-wave-52433048` |
| Region | `aws-us-east-2` |
| Postgres | 18 |
| Branch | `production` (`br-billowing-night-a5dlobr7`, default) |
| Database | `Fleet_Manage_Oil` |
| Role | `Fleet_Manage_Oil_owner` |
| Host (pooled) | `ep-raspy-moon-a52206a2-pooler.us-east-2.aws.neon.tech` |
| Object storage | private bucket `uploads` (not required for this backup) |
| Linked context | `.neon` (gitignored) |
| IaC | `neon.ts` (`preview.buckets.uploads` private) |
| Env file from `neon env pull` / `neon deploy` | `.env.local` (gitignored) |

CLI is already authenticated. Node lives at `C:\Program Files\nodejs` (may be missing from PATH). Neon CLI is global (`npm i -g neon@latest`).

```powershell
$env:PATH = "C:\Program Files\nodejs;$env:APPDATA\npm;$env:PATH"
neon me
neon status
neon connection-string          # pooled
# Direct/unpooled is DATABASE_URL_UNPOOLED in .env.local — use that for migrations.
```

Skills already installed in this repo: `.grok/skills/neon/SKILL.md`, `neon-postgres`, `neon-object-storage`. Read those before changing Neon config.

## Credentials (never commit, never print values)

1. `neon env pull` (already done) writes into `.env.local`:
   - `DATABASE_URL` — pooled, app traffic
   - `DATABASE_URL_UNPOOLED` — **direct; use this to apply schema / backup writes**
   - `NEON_BRANCH`, plus `AWS_*` for the uploads bucket
2. oilchange does **not** read `.env.local`. Copy into gitignored `oilchange.env`:

```
# keep SQLite as the working store
OILCHANGE_DB=./oilchange.sqlite

# Neon backup (direct/unpooled — hostname must NOT contain -pooler)
DATABASE_URL=<value of DATABASE_URL_UNPOOLED from .env.local>
```

3. If both `OILCHANGE_DB` and `DATABASE_URL` are set, **DSN stays sqlite**. That is required. Backup code must open a **second** store with pgx + `DATABASE_URL` (unpooled), not `cfg.DSN()`.
4. Never put Neon URLs in `NEXT_PUBLIC_*`. Never commit `.env.local`, `oilchange.env`, or `.neon`.
5. `oilchange env` reports key presence only; it never prints secret values. Keep it that way.

Pooled vs direct (Neon): migrations, dumps, and session-level SQL must use the **unpooled** URL. Pooled (`-pooler`) is for app query traffic only.

## Schema to back up

Canonical schema: `migrations/001_schema.sql` plus `003_cards_intel.sql` and `004_onestep_devices.sql`. On pgx, `store.applyMigrations` also runs `002_rls.sql`.

Tables (unprefixed names — this is the dedicated Neon DB, **not** the shared Supabase `fleet_*` names):

- `cars`, `cards`, `gas_stations`, `maintenance_locations`
- `fills`, `shop_ros`, `oil_changes`, `hold_events`
- `onestep_devices`, `drive_stop_miles`
- `card_transactions`, `card_pairings`

Do **not** rename these to `fleet_*` on Neon. `fleet_*` is only for the shared Supabase project so reception/Users stay intact.

Opening `store.Open("pgx", unpooledURL)` already applies those migrations. First backup should create empty tables, then copy rows.

## Recommended implementation

### 1. Backup command (preferred)

Add `oilchange backup-neon` (alias ok: `backup`).

Behavior:

1. Open **source** via existing `openApp(cfg)` / `cfg.DSN()` (SQLite).
2. If `cfg.DatabaseURL` is empty, print a clear error: set `DATABASE_URL` in `oilchange.env` to Neon **unpooled**.
3. Refuse anything that looks like XRAY / the old Supabase host if someone stuffed it in `DATABASE_URL`.
4. Open **dest** with `store.Open("pgx", cfg.DatabaseURL)` so migrations run.
5. Copy durable rows SQLite → Neon using existing store list/upsert methods where they exist (`ListCars`/`UpsertCar`, fills, shop ROs, devices, miles, holds, oil changes, cards, card txs, pairings, stations). Add list/upsert helpers only where missing (keep them small and tested).
6. Print counts (cars, fills, holds, devices, …). Do not print DSNs or passwords.
7. Do **not** run `compute`. Do **not** write Last Reading except as copied columns on `cars`.
8. Optional later: hook `oilchange sync` so a successful sync also runs the Neon backup. First PR can be the explicit command; wiring into `sync` is fine if it stays best-effort (SQLite + Supabase + mirror still succeed if Neon is down, unless `--require-neon`).

### 2. Do not switch the primary DSN

Do not unset `OILCHANGE_DB`. Do not make `serve` / `compute` / `sync-enterprise` talk to Neon. Desktop must keep working offline on SQLite.

### 3. Tests

- Unit-test the copier with two temp sqlite stores **or** sqlite → sqlite, then one integration test skipped unless `DATABASE_URL` is set.
- Do not hit live Neon from default `go test ./...`.
- Keep existing tests green: `go test ./...`

### 4. Env / docs

- `oilchange.env.example`: document `DATABASE_URL` as Neon **backup** (unpooled), while `OILCHANGE_DB` remains the working store.
- README: one short section — Neon is backup, Supabase `sync` is still the Oil Desk remote, SQLite is still local.
- `oilchange env` already lists `DATABASE_URL`; add a note it is backup/pgx, not the DSN when sqlite is set.

## Constraints (hard)

- Do not invent miles. Last Reading only via `internal/oil`.
- HOLD skips Last Reading write; backup must copy HOLD state, not “fix” it.
- Join OneStep on `factory_id` only; `display_name` is never a join key.
- Never target XRAY (`chjqcznyxvtjbamttqdj`).
- Do not break `oilchange sync` → Supabase `fleet_cars` / `web/data/cars.json`.
- Do not require npm for desktop `serve`.
- Do not commit secrets, `.neon`, `.env.local`, `oilchange.env`, PEMs, or sqlite files.
- Do not apply shared-project `supabase/migrations/*fleet*` SQL to Neon.
- Prefer CLI over Neon MCP. If you change `neon.ts`, `neon config plan` then `neon deploy`. Object storage is unrelated to this backup.

## Verify

```powershell
$env:PATH = "C:\Program Files\nodejs;$env:APPDATA\npm;$env:PATH"
go test ./...
go build -o bin/oilchange.exe ./cmd/oilchange
.\bin\oilchange.exe env    # OILCHANGE_DB set; DATABASE_URL set; no secret values
.\bin\oilchange.exe backup-neon
```

Then, without printing the URL:

```powershell
# row counts on Neon should match sqlite cars / open holds / devices
neon connection-string --disable-psql   # if needed, use psql against UNPOOLED
```

Sanity checks:

- SQLite file still used by `compute` / `serve`
- `oilchange sync --mirror web/data/cars.json` still writes the mirror
- Neon `cars` row count ≈ sqlite `cars`
- No `-pooler` host used for the backup connection
- `neon status` still on branch `production`

## Files you will likely touch

- `cmd/oilchange/main.go` — dispatch
- new `cmd/oilchange/backup_cmd.go` (or similar)
- `internal/app/` — `BackupNeon` that opens dest store separately
- `internal/store/` — any missing list helpers
- `oilchange.env.example`, `README.md`
- tests next to the new copier

## Out of scope

- Neon Auth, Functions, AI Gateway, Data API
- Using the `uploads` bucket for sqlite file dumps (row copy into Postgres is the backup)
- Migrating Oil Desk reads off Supabase onto Neon
- Nested `Fleet-Management/` tree
- Changing OneStep JWT / eFleets live download
