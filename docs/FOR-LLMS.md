# For LLMs arriving in this repo

Read this first, then [`docs/collab/OIL-LOCKS.md`](collab/OIL-LOCKS.md). Do not weaken those locks to “make the desk look finished.”

## What this is

**oilchange** is a Go CLI (`./cmd/oilchange`) plus an embedded **Oil Desk** UI. It tracks oil-change due miles, last oil, GPS boxes, and fuel-card ↔ vehicle evidence for one fleet.

It is not a generic fleet SaaS. Desktop is Go only: `go build -o oilchange ./cmd/oilchange` embeds the static UI (`web/out`). `oilchange serve` hosts Oil Desk + `/api/cars` on `127.0.0.1:4739`. No npm on the machine that runs the desk.

Two puzzles, not one:

1. **Last Reading** — trusted miles so oil interval (when the next change is due) is honest.
2. **Card ↔ vehicle** — which car was actually at the pump when a fuel card swiped.

## Goal

**Last Reading** is Enterprise odometer at a trusted fill or shop second, plus OneStep **drive-stop miles since that second**. The formula lives only in `internal/oil`. Do not invent Last Reading from GPS box odometer (`odometer` / `odometer_from` / `odometer_to` / device mileage).

**Card ↔ vehicle** is GPS-at-the-pump, not the Enterprise DETAILS “Vehicle” column. That column is last-write-wins and is often the wrong car. Call the swipe by the GPS-named vehicle that sat at that station.

## What it does

```
eFleets / OneStep / compute  →  SQLite (OILCHANGE_DB)     ← daily driver (works offline)
                                    ├─ oilchange sync     → Oil Desk remote `fleet_cars` + web/data/cars.json
                                    └─ oilchange backup-neon → Neon Postgres (AWS-hosted backup only)
```

SQLite (`OILCHANGE_DB`) is the working store for ingest, compute, and serve. Neon is a durable copy of those sqlite tables — not the daily driver. Neon runs on AWS; do **not** add S3, RDS, a second Neon, or any other direct-AWS backup. The Oil Desk remote is this fleet’s Supabase `fleet_cars` table. **Later**, that same Supabase project becomes a **full** sqlite-table backup so an AWS/Neon outage is not the only copy. Today `sync` is still desk cars/holds, not that full backup. Command cheat-sheet: [`README.md`](../README.md).

| Cmd | Role |
|---|---|
| `sync-enterprise` | Ingest eFleets roster, fuel DETAILS, optional shop ROs. Seeds last oil. Does **not** compute Last Reading. Live portal if `EFLEETS_*` is set; otherwise `--vehicles` / `--fuel-details` file drop. |
| `sync-onestep` | Devices + drive-stop miles-since after the trusted fill second. Join on `factory_id` only. |
| `devices sync` / `list` | Durable `onestep_devices` registry (`factory_id` PK, `device_id` history id, `display_name` label only). Does not fetch miles. |
| `probe-onestep` | One-shot live drive-stop GET. Prints measured miles. Does **not** write Last Reading. |
| `compute` | Last Reading + HOLD. `[--override-lower]` is the only way to write a lower reading. |
| `oil-done` | Record an oil change (`--efleets-id` `--miles` `--date`). |
| `serve` | Oil Desk UI + `/api/cars` from the embedded export. `[--addr]` `[--mirror]`. |
| `cards rebuild` | Score swipes into `card_pairings`. GPS-first stop windows from OneStep unless `--no-gps` (rematch from `data/runtime/gps-stops.json`). Never writes Last Reading. |
| `cards split` | GPS eras: which vehicle each card was in over time (`SPLIT` when one card sat in two cars). |
| `cards call` | What to call each swipe (GPS-named car, not the DETAILS Vehicle column). Default: only rows where GPS ≠ Enterprise. `--all` prints every GPS-named swipe. |
| `cards suspect` / `trace` / `pairings` | Wrong-car intel. Pairings are evidence, not Enterprise last-write-wins. |
| `report` / `holds` | Due list / open HOLDs. |
| `sync` | Push sqlite cars/holds to Oil Desk `fleet_cars` + refresh `web/data/cars.json`. Best-effort Neon backup after success. |
| `backup-neon` | Copy sqlite oilchange tables into Neon. Alias: `backup`. |
| `env` | Which `oilchange.env` keys loaded. Presence only; never prints values. |

Exit: `0` ok, `1` error, `2` compute finished with open HOLDs (report still allowed).

### GPS-first card matching

`cards rebuild` uses OneStep **stop** windows (drive-stop list, no odometer). A swipe is assigned when exactly one GPS-linked car was in a short stop at that swipe time/place. Overnight sits are ignored. Two cars stopped → no vote. Station lat/lng come from exclusive GPS sits; later swipes can match the car at that pump even when another box is sitting elsewhere. Enterprise Vehicle is not the join.

None of the `cards *` commands write Last Reading.

## Hard locks (do not regress)

If a step would violate one of these, stop and HOLD. Full card: [`docs/collab/OIL-LOCKS.md`](collab/OIL-LOCKS.md).

1. **Last Reading only in `internal/oil`.** Trusted fill/shop odo + stored drive-stop miles-since. Nowhere else.
2. **Never OneStep odometer.** Ignore `odometer` / `odometer_from` / `odometer_to` / device mileage as Last Reading.
3. **HOLD skips the Last Reading write.** Do not seed a number to clear a HOLD. `WriteLastReading` must not “finish the desk.”
4. **Join GPS on `factory_id` only.** `device_id` is History / drive-stop identity. **`display_name` is never a join key.** Never guess an unpaired `factory_id`.
5. **Never invent miles.** A missing GPS sum is `NO_DRIVESTOP`, not zero. A measured empty trip list **is** stored as 0 on purpose — that is not a guess.
6. **SQLite is the daily driver.** Neon is the AWS-hosted backup. Oil Desk remote is this fleet’s Supabase `fleet_cars`. That Supabase project is the planned second **full** backup (not a new AWS product). Do not unset `OILCHANGE_DB` so desktop can keep working offline.
7. **Never write fleet oil to the other/wrong Supabase project** (the in-repo name is XRAY). Refuse it as `DATABASE_URL` too (also refuse `-pooler` and `supabase.co` as Neon). Exact project lock: OIL-LOCKS + README. Do not copy that lock from chat; copy it from those files.
8. Live device map is gitignored `data/runtime/onestep-map.csv`. Fixture `testdata/onestep/map.csv` is not the fleet.
9. **Never ask a human to log into eFleets in chat.** Secrets belong in gitignored `oilchange.env` or Cloud Agent secrets. File-drop CSVs do not need a portal login.

ExpectedBand (90 mph) rejects punches. It does not invent Last Reading.

Open HOLD codes operators see include `NO_DEVICE`, `NO_DRIVESTOP`, `NO_TRUSTED_FILL`, `LOWER_READING_REFUSED`, `LOGISTICS_PERSONNEL` (logistics-personnel names on a punch or OneStep label never create a device↔car link).

## Secrets and live portals

Secrets live in gitignored **`oilchange.env`** (template: `oilchange.env.example`) or **Cloud Agent secrets**. Never paste credentials, PEMs, JWTs, portal passwords, customer/portal numbers, or connection strings into chat, PRs, or git.

- `oilchange env` prints which keys loaded. It never prints values. Keep it that way.
- Set `EFLEETS_CUST_NUM` in that env. Do **not** hardcode a portal/customer number in source or docs.
- Already-open Chrome session (CDP) is documented separately; do not type a password for that path, and do not ask anyone to sign in via chat.
- Tests never hit live eFleets, OneStep, or Supabase.

Do not commit `oilchange.env`, sqlite files, `data/runtime/`, PEMs, or `web/data/cards.json`.

## Read next (do not duplicate here)

| Doc | When |
|---|---|
| [`README.md`](../README.md) | Commands, desktop run, env table |
| [`docs/collab/OIL-LOCKS.md`](collab/OIL-LOCKS.md) | Product locks |
| [`docs/CHROME-SESSION-ENTERPRISE.md`](CHROME-SESSION-ENTERPRISE.md) | Attach to an already-logged-in eFleets tab (no password in env for that path) |
| [`docs/NEON-FOR-LLMS.md`](NEON-FOR-LLMS.md) | Neon is already the sqlite backup — keep using it; do not recreate the project |
| [`docs/onestep-api-auth.md`](onestep-api-auth.md) | Public v3 auth and drive-stop query names |
| [`docs/collab/README.md`](collab/README.md) | Collaborator notes for other sessions |
| [`docs/ACCIDENTAL-TWIN.md`](ACCIDENTAL-TWIN.md) | Wrong GitHub repo — do not use it |

Canonical source tree is this repo root (`cmd/oilchange`, `internal/`, `migrations/`). Ignore a nested untracked `Fleet-Management/` copy unless you are syncing that tree on purpose.
