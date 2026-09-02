# oilchange

PDI fleet oil-change binary. Last Reading is computed only in `internal/oil`.
Do not invent miles. HOLD skips the Last Reading write.

```
go build -o oilchange ./cmd/oilchange
go test ./...
```

Toolchain: Go 1.27.1.

GitHub [Fleet-Management](https://github.com/ZacharyTFerguson/Fleet-Management) files live here too:

- OneStep public API how-to: `docs/onestep-api-auth.md` (`main`)
- Location-dwell Node app: `server.js`, `lib/`, `bin/`, `public/` (branch `cursor/important-location-trigger-61d4`)

## Commands

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

## Secrets (never commit)

Paste live values into **`oilchange.env`** in this directory (gitignored). Template: `oilchange.env.example`.

The binary loads `oilchange.env` on startup (or `OILCHANGE_ENV`, `secrets/oilchange.env`, `.env`). Already-set process env wins.

```
OILCHANGE_DB=./oilchange.sqlite
EFLEETS_USERNAME=
EFLEETS_PASSWORD=
EFLEETS_CUST_NUM=583424
EFLEETS_BASE_URL=https://login.efleets.com
EFLEETS_DETAILS_URL=
EFLEETS_MAINT_URL=
EFLEETS_FLEETSUMMARY_URL=
ONESTEP_API_TOKEN=
ONESTEP_API_KEY=
ONE_STEP_FULL_API_KEY=
ONESTEP_API_PRIVATEKEY=
ONESTEP_BASE_URL=https://track.onestepgps.com
DATABASE_URL=
SUPABASE_URL=
SUPABASE_SERVICE_ROLE=
```

Tests never hit live eFleets, OneStep, or Supabase.

HOLD `LOGISTICS_PERSONNEL` (third-spec name `RICH_TYLER_PAIRING`): logistics-personnel names on a punch or OneStep label never create a device↔car link.
