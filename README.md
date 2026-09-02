# oilchange

PDI fleet oil-change binary in **Fleet-Management**. Last Reading is computed only in `internal/oil`.
Do not invent miles. HOLD skips the Last Reading write.

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
| `compute` | Last Reading + HOLD. `[--override-lower]` is the only way to write a lower reading. |
| `oil-done` | `--efleets-id ID --miles N --date YYYY-MM-DD [--location NAME]` |
| `report` | `[--interval 5000] [--due-within N] [--out PATH.csv]` |
| `holds` | open HOLDs |
| `env` | which `oilchange.env` keys loaded (presence only; never prints values) |

Exit: `0` ok, `1` error, `2` compute finished with open HOLDs (report still allowed).

## Secrets (never commit)

Paste live values into **`oilchange.env`** in this directory (gitignored). Template: `oilchange.env.example`.

The binary loads `oilchange.env` on startup (or `OILCHANGE_ENV`, `secrets/oilchange.env`, `.env`). Already-set process env wins.

`EFLEETS_CUST_NUM` has **no** hardcoded default — set it in env for live eFleets sync.

Tests never hit live eFleets, OneStep, or Supabase.

HOLD `LOGISTICS_PERSONNEL` (third-spec name `RICH_TYLER_PAIRING`): logistics-personnel names on a punch or OneStep label never create a device↔car link.

## Accidental twin

See [`docs/ACCIDENTAL-TWIN.md`](docs/ACCIDENTAL-TWIN.md) — do not use `fleetmanager` / `fleetmanagerlocalvscode`.
