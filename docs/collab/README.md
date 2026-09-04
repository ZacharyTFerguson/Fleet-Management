# Oilchange collab

This folder is for **other Grok / agent sessions working in the same repo**, not a handoff and not a review queue. Read `STATUS.md`, then keep going. If you change something that another session will trip over, update STATUS.

Workspace: `C:\Users\Zacha\go\local9_3_2026` (repo root). Ignore nested `Fleet-Management/` unless you are syncing that tree on purpose.

## How to work here

- Collaborator, not reviewer. Do the next real step.
- Secrets stay in gitignored `oilchange.env` or Cloud Agent secrets. Never paste keys, PEMs, JWTs, eFleets passwords, or Neon URLs into chat, git, or these files. Do not ask Zachary to log into eFleets in chat.
- Do not invent miles. Last Reading is only `internal/oil`: Enterprise odo at the trusted fill second + stored OneStep **drive-stop miles-since**. HOLD skips the Last Reading write.
- OneStep join is **`factory_id` only**. `display_name` / nickname is never a join key. `device_id` is History / drive-stop identity.
- Live device map is `data/runtime/onestep-map.csv` (gitignored). Do **not** use `testdata/onestep/map.csv` on the live roster.
- SQLite (`OILCHANGE_DB`) is the daily driver. Neon is the backup. Supabase `fleet_cars` is the Oil Desk remote today; later a second **full copy** if Neon is down. Do not add S3, extra Neon, or direct AWS. `--no-remote` when you must not push null Last Reading.

## Locked oil rules

| Do | Do not |
|---|---|
| Pair cars on `factory_id` | Join on `display_name` |
| Call drive-stop with `device_id` (`oilchange probe-onestep` is a one-shot GET; see DRIVE-STOP-LIVE-PROBES.md) | Treat OneStep odometer / `odometer_from` / `odometer_to` as Last Reading |
| Persist measured miles, including a measured **0** | Seed Last Reading to clear a HOLD |
| HOLD `NO_DEVICE` / `NO_DRIVESTOP` / `NO_TRUSTED_FILL` | Guess a reading so the desk looks populated |
| `oilchange sync --mirror --no-remote` to refresh `web/data/cars.json` | `sync --mirror` with write creds while Last Reading is still null unless that push is intended |

## OneStep drive-stop (live, 2026-09-03)

Wrong params (`factory_id` + `from`) HTTP **500**. Working public v3 query:

```
GET /v3/api/public/route/drive-stop
  device_id
  dt_tracker_from   RFC3339 UTC (trusted fill second)
  dt_tracker_to     RFC3339 UTC (now)
  stop_duration     5m0s
  api-key           (or JWT RS256 if PEM is loaded — device list and drive-stop both work)
```

Do **not** send `return_points` for miles-since (map-UI payload; hung fleet sync). Miles live on `distance.value` (+ `unit`); ignore odometer objects.

Code: `internal/onestep/client.go`. Ingest: `oilchange sync-onestep --map data/runtime/onestep-map.csv`.

## Nearby Address (unknown cards)

Fill-day ±1 (provider swipe, not bank posting), 1 mile, watch `factory_id` until exclusive. Live spec: `docs/collab/NEAR-ADDRESS.md`. CLI: `oilchange cards nearby`. Targeted follow-up: `oilchange cards watch --live --persist` (newest 10 fills, watched boxes only, 35s pace).

## Talk to the session that wrote this

Point the other agent at this folder and `STATUS.md`. If you are Grok Bot / another Grok in this workspace, treat STATUS as shared working memory: what is true now, what is blocked, what the next step is. Update it when you finish a step.
