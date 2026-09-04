# Handoff for other LLMs: Neon is already the sqlite backup

Paste this whole file into another agent in workspace `C:\Users\Zacha\go\local9_3_2026`.

**Status (2026-09-03):** Neon backup is **implemented and verified**. Do **not** create a new Neon project. Do **not** re-implement `backup-neon` unless it is broken. Your job is to **keep using** this project, keep secrets out of git, and not turn Neon into the daily driver.

## What Neon is (and is not)

| Neon is | Neon is not |
|---|---|
| Durable **Postgres backup** of oilchange sqlite tables | The working store for `compute` / `serve` / `sync-enterprise` |
| Database `Fleet_Manage_Oil` on project `Fleet_Management_Neon` | Supabase Oil Desk (`fleet_cars` on `hdtwfdjdvdzdxfdriyzn`) |
| Unprefixed table names (`cars`, `fills`, …) | Shared-project `fleet_*` names |
| `oilchange backup-neon` (also best-effort after `sync`) | A place to invent Last Reading or join OneStep on `display_name` |

```
eFleets / OneStep / compute  →  SQLite (OILCHANGE_DB)     ← daily driver
                                    ├─ oilchange sync     → Supabase fleet_cars + web/data/cars.json
                                    └─ oilchange backup-neon → Neon Postgres (this)
```

SQLite stays the DSN whenever `OILCHANGE_DB` is set. `DATABASE_URL` is a **second** pgx connection used only by backup.

Redundancy when needed: SQLite is the working store. Neon is the backup. This fleet’s Supabase is Oil Desk today; later a full sqlite-table copy if Neon is down. Do not add S3, extra Neon, or direct AWS.

## Project (already exists)

Do **not** run `neon init` / `create_project`. CLI is authenticated. Node: `C:\Program Files\nodejs` (prepend to PATH if missing).

| | |
|---|---|
| Name | `Fleet_Management_Neon` |
| Project id | `icy-thunder-13848536` |
| Org | `org-cold-wave-52433048` |
| Region | `aws-us-east-2` |
| Postgres | 18 |
| Branch | `production` (`br-billowing-night-a5dlobr7`, default) |
| Database | `Fleet_Manage_Oil` |
| Role | `Fleet_Manage_Oil_owner` |
| Pooled host | `ep-raspy-moon-a52206a2-pooler.us-east-2.aws.neon.tech` |
| Direct host | `ep-raspy-moon-a52206a2.us-east-2.aws.neon.tech` (no `-pooler`) |
| Linked | `.neon` (gitignored) |
| IaC | `neon.ts` (`preview.buckets.uploads` private — unused by backup) |
| `neon env pull` target | `.env.local` (gitignored) |

```powershell
$env:PATH = "C:\Program Files\nodejs;$env:APPDATA\npm;$env:PATH"
neon me
neon status          # must stay branch production
# Prefer CLI over Neon MCP. Do not change neon.ts unless asked; then neon config plan && neon deploy.
```

Skills in this repo: `.grok/skills/neon/SKILL.md`, `neon-postgres`, `neon-object-storage`. Read them before changing Neon config.

## Credentials (never commit, never print values)

oilchange loads **`oilchange.env`**, not `.env.local`. Process env wins if already set.

`.env.local` from `neon env pull`:

- `DATABASE_URL` — **pooled** (`-pooler`) — app query traffic only
- `DATABASE_URL_UNPOOLED` — **direct** — migrations, dumps, **backup writes**
- `NEON_BRANCH`, plus `AWS_*` for the uploads bucket

`oilchange.env` must have:

```
OILCHANGE_DB=./oilchange.sqlite
DATABASE_URL=<DATABASE_URL_UNPOOLED from .env.local>
```

Hostname **must not** contain `-pooler`. Never put Neon URLs in `NEXT_PUBLIC_*`. Never commit `.env.local`, `oilchange.env`, `.neon`, PEMs, or sqlite files.

`oilchange env` reports key presence only. Keep it that way.

If both `OILCHANGE_DB` and `DATABASE_URL` are set, `cfg.DSN()` is **sqlite**. Backup opens dest with `store.Open("pgx", cfg.DatabaseURL)`, not `cfg.DSN()`.

Refuse `DATABASE_URL` that looks like XRAY (`chjqcznyxvtjbamttqdj`), `supabase.co`, or `-pooler`.

## Code that already exists (edit, don’t redo)

| Path | Role |
|---|---|
| `cmd/oilchange/backup_cmd.go` | `oilchange backup-neon` / `backup` |
| `cmd/oilchange/sync_cmd.go` | after successful `sync`, best-effort backup (`--require-neon` to fail hard) |
| `internal/app/backup_neon.go` | `validateNeonBackupURL`, `BackupNeon`, `CopyDurable` |
| `internal/store/backup.go` | list-all helpers + `ApplyCarSnapshot` / hold-event copy (no `WriteLastReading`) |
| `internal/store/migrate.go` | `002_rls.sql` ignores missing Supabase `anon`/`authenticated` roles on Neon |
| `oilchange.env.example` | documents Neon as backup |

Tables copied (unprefixed): `cars`, `cards`, `gas_stations`, `maintenance_locations`, `fills`, `shop_ros`, `oil_changes`, `hold_events`, `onestep_devices`, `drive_stop_miles`, `card_transactions`, `card_pairings`.

Opening `store.Open("pgx", unpooledURL)` applies `migrations/*.sql` (and `002_rls.sql` on pgx). First backup creates empty tables then copies rows.

Last Reading is **copied as stored columns** on `cars`. Backup must **not** run `compute` or `WriteLastReading` (that path clears HOLD).

## Commands

```powershell
go test ./...
go build -o bin/oilchange.exe ./cmd/oilchange
.\bin\oilchange.exe env           # OILCHANGE_DB set; DATABASE_URL set; no secret values
.\bin\oilchange.exe backup-neon   # prints counts only
```

Last verified copy (approx): sqlite cars=205, fills=201, open holds=205, cards=201 matched Neon. Devices came later via live OneStep (100 boxes, unlinked). Re-run `backup-neon` after ingest if you need Neon to catch up.

## Hard rules (same as the rest of oilchange)

- Do not invent miles. Last Reading only via `internal/oil` on **compute**.
- HOLD skips Last Reading write; backup copies HOLD state, does not “fix” it.
- Join OneStep on `factory_id` only. `display_name` is never a join key.
- Never target XRAY (`chjqcznyxvtjbamttqdj`).
- Do not break `oilchange sync` → Supabase `fleet_cars` / `web/data/cars.json`.
- Do not require npm for desktop `serve`.
- Do not apply `supabase/migrations/*fleet*` SQL to Neon.
- Do not unset `OILCHANGE_DB`. Desktop must keep working offline on sqlite.
- Do not add S3, extra Neon, or direct AWS. The later full copy is this fleet’s Supabase when that work is asked for.
- Default `go test ./...` must not hit live Neon (integration test skips unless `DATABASE_URL` is already in the process env).

## Out of scope unless the human asks

- Neon Auth, Functions, AI Gateway, Data API
- Using the `uploads` bucket for sqlite file dumps
- Extra Neon, S3, or direct AWS
- Building a full Supabase sqlite-table backup until the human asks (today `oilchange sync` is Oil Desk `fleet_cars` only)
- Migrating Oil Desk reads off Supabase onto Neon
- Nested untracked tree `Fleet-Management/` (implement in **repo root**)
- Creating a second Neon project
