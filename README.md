# Fleet-Management

Helpers and docs for PDI fleet automation, plus the `oilchange` Last Reading binary.

## Docs

- [OneStepGPS API credentials and usage](docs/onestep-api-auth.md) — how to use the API key (and related credentials) against `track.onestepgps.com` without committing secrets.

## oilchange

PDI fleet oil-change binary. Last Reading is computed only in `internal/oil`.
Do not invent miles. HOLD skips the Last Reading write.

```
go build -o oilchange ./cmd/oilchange
go test ./...
```

Toolchain: Go 1.27.1.

Also in this tree: location-dwell Node app (`server.js`, `lib/`, `bin/`, `public/`).

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
