# Fleet-Management

Helpers and docs for PDI fleet automation: the `oilchange` Last Reading binary, and a Node recorder for important sit-still locations.

## Docs

- [OneStepGPS API credentials and usage](docs/onestep-api-auth.md) — how to use the API key (and related credentials) against `track.onestepgps.com` without committing secrets.
- [Important locations (sit rule)](docs/LOCATION-TRIGGER-README.md) — dwell thresholds, HTTP/CLI ingest, and catalog file.

## oilchange

PDI fleet oil-change binary. Last Reading is computed only in `internal/oil`.
Do not invent miles. HOLD skips the Last Reading write.

```
go build -o oilchange ./cmd/oilchange
go test ./...
```

Toolchain: Go 1.27.1.

OneStep live auth: short-lived RS256 JWT when a PEM is present in gitignored `oilchange.env`; otherwise `api-key` query (see `docs/onestep-api-auth.md`).

### Commands

| Cmd | What |
|---|---|
| `sync-enterprise` | Live eFleets if `EFLEETS_*` is set, or `--vehicles` `--fuel-details` `[--shop-ro]` `[--mileage-history]`. Oil/lube shop ROs seed last oil. Does not compute Last Reading. |
| `sync-onestep` | OneStep API devices + drive-stop miles-since after the trusted fill second. Join `factory_id` only. Optional `--map PATH`. |
| `compute` | Last Reading + HOLD. `[--override-lower]` is the only way to write a lower reading. |
| `oil-done` | `--efleets-id ID --miles N --date YYYY-MM-DD [--location NAME]` |
| `report` | `[--interval 5000] [--due-within N] [--out PATH.csv]` |
| `holds` | open HOLDs |
| `env` | which `oilchange.env` keys loaded (presence only; never prints values) |

Exit: `0` ok, `1` error, `2` compute finished with open HOLDs (report still allowed).

### Secrets (never commit)

Paste live values into **`oilchange.env`** in this directory (gitignored). Template: `oilchange.env.example`.

The binary loads `oilchange.env` on startup (or `OILCHANGE_ENV`, `secrets/oilchange.env`, `.env`). Already-set process env wins.

Tests never hit live eFleets, OneStep, or Supabase.

HOLD `LOGISTICS_PERSONNEL` (third-spec name `RICH_TYLER_PAIRING`): logistics-personnel names on a punch or OneStep label never create a device↔car link.

## Important locations

Records places where a vehicle sits still long enough to matter (`server.js`, `lib/`, `bin/`, `public/`).

Sit rule (summary): speed missing or ≤ **3 mph**, inside **100 m** for **15 minutes** → store the place. A **60-minute** silent gap closes the sit; later sits within **150 m** update the same place.

```bash
npm test
npm start
```

Open http://127.0.0.1:3000 — or ingest via `POST /api/v1/pings` / `node bin/record-location.js`. Empty payloads return `recorded: false` and do not invent a location.

Full detail: [docs/LOCATION-TRIGGER-README.md](docs/LOCATION-TRIGGER-README.md).
