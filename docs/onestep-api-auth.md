# OneStepGPS API credentials and usage

Practical guide for calling the One Step GPS (OneStep / OneStepGPS) **public v3 API** for this fleet. No real secrets belong in this repository.

This account does **not** authenticate with `?api-key=` or a raw Bearer API key. Those notes (generic OneStep samples, older Cursor guide text) are wrong here.

## What you need

| Credential | Required? | Notes |
|------------|-----------|--------|
| **API token** (`access_token` claim) | Yes | Issued by OneStep. Load from env / local settings — never ask the owner to paste it into chat. |
| **Account PEM** (RSA private key) | Yes | Signs a short-lived RS256 JWS. Without the PEM, do not call the API. |
| **Base URL** | Yes (default below) | Host `https://track.onestepgps.com` (paths are under `/v3/api/public`). |
| **Account / portal login** | Portal only | Browser History / Reports use the signed-in session on `track.onestepgps.com`. That is a spot-check, not the miles source. |

### Auth (this account)

Mint an RS256 JWS whose payload is `{ "access_token": "<token>", "exp": <unix> }`, signed with the account PEM. Send it as:

```text
Authorization: Bearer <jwt>
```

- Not raw `Authorization: Bearer <api-key>`.
- Not `?api-key=` on the query string.
- Sign a fresh token per request (short `exp`, a few minutes at most).
- Never commit the token or PEM. Never log the JWT, PEM body, or token.

### Where credentials live (this org)

- Prefer env vars or a **gitignored** local settings file (`oilchange.env`).
- Never commit the token or PEM, never paste them into Slack/chat/Drive docs that are broadly shared, never put them in this public repo.
- Cloud Agent secret names used by the Go binary: token `OneStepAPIKEYTobeSigned` (also `ONESTEP_API_TOKEN` / `ONESTEP_API_KEY`); PEM `OneStepAPIKEY` (also `ONESTEP_API_PRIVATEKEY`) or `ONESTEP_PEM_PATH`.

## Environment variables (recommended)

```bash
# Required — both. Token alone is not enough for this account.
export ONESTEP_API_TOKEN='YOUR_API_TOKEN'
export ONESTEP_API_PRIVATEKEY='-----BEGIN PRIVATE KEY-----
...
-----END PRIVATE KEY-----'
# Or a file path instead of inlining the PEM:
# export ONESTEP_PEM_PATH='/secure/path/to/onestep-client.pem'

# Optional override (host or host + /v3/api/public)
export ONESTEP_BASE_URL='https://track.onestepgps.com'
```

Suggested local files (examples only — keep them out of git):

- `oilchange.env` (gitignored)
- `.env` / `.env.local` (gitignored)

Add patterns like `.env`, `oilchange.env`, `*.pem` to `.gitignore` before creating any secret files.

## How auth works (this fleet)

```text
GET {ONESTEP_BASE_URL}/v3/api/public/device?latest_point=true
Authorization: Bearer <rs256-jws>
```

Miles-since (when the route is permitted):

```text
GET {ONESTEP_BASE_URL}/v3/api/public/route/drive-stop?...
Authorization: Bearer <rs256-jws>
```

Exact `drive-stop` query parameter names come from a proven 200 / live apidoc. Do not invent params. You will need a **`device_id`** (after a `factory_id` pair) and a **time window after the trusted fill second**.

### Live status (this account)

| Call | Status | What to do |
|------|--------|------------|
| `GET /device` | **200** | Inventory. Pair by `factory_id`. `device_id` is History identity. Drop any odometer JSON. |
| `GET /route/drive-stop` | **403** (current) | **HOLD** (`NO_DRIVESTOP`). Spot-check History. Do **not** invent miles. Do **not** use OneStep’s own odometer as Last Reading. A 200 on `/device` is not permission to substitute odo. |

### About PEM files

- A Drive file named `cas.pem` (empty / docker CA placeholder) is **not** the OneStep API credential.
- This account **does** need the real client private key PEM to mint the JWS.
- Store it outside the repo (`ONESTEP_PEM_PATH` or gitignored `oilchange.env`).
- Permissions `0600`, owner-only.
- Prefer PKCS#8 or traditional RSA PEM (`BEGIN PRIVATE KEY` / `BEGIN RSA PRIVATE KEY`).
- Never log the PEM body; never commit it; never base64-dump it into tickets.
- Validate format with `openssl pkey -in "$ONESTEP_PEM_PATH" -check -noout` (shows validity only).

## Minimal successful requests

### curl — list devices (JWT, not api-key)

```bash
# jwt is a short-lived RS256 JWS {access_token, exp} signed with the account PEM.
# Do not print $jwt in CI logs.
curl -sS -G \
  "${ONESTEP_BASE_URL:-https://track.onestepgps.com}/v3/api/public/device" \
  --data-urlencode "latest_point=true" \
  -H "Authorization: Bearer ${jwt}"
```

Expect HTTP **200** and JSON with a `result_list` (or equivalent) of devices. Each device includes (among other fields):

- `factory_id` — hardware id (pairing key for this fleet)
- `device_id` — portal / History id
- `display_name` — label only (never a join key)

### curl — drive-stop

Same `Authorization: Bearer <jwt>` header. Do **not** add `api-key`. If the response is **403**, HOLD and use History as a spot-check only — never invent miles and never use device odometer.

## Useful endpoints (fleet context)

| Path | Use |
|------|-----|
| `GET /device` | Inventory; map `factory_id` ↔ `device_id`; optional `latest_point=true`. Proven **200** on this account. |
| `GET /route/drive-stop` | Miles since a timestamp **when it returns 200**. Current live is **403** → HOLD / History. |
| History / reports in the **portal** | Spot-check when GPS went dark or drive-stop is 403. Not the Last Reading source. |

Related portal (not API token auth):

- History UI: `https://track.onestepgps.com/v3/ux/map/history/single?device_id=…`
- Reports dashboard: `https://track.onestepgps.com/v3/ux/reports/dashboard`

Other public areas (device drives, history, report-export): confirm against live apidoc before coding. Do not invent paths.

## Fleet pairing rules (locked)

1. Pair cars by **`factory_id`** (hardware).
2. Use **`device_id`** for History / drive-stop calls after pairing.
3. Never join on **`display_name`** (or driver nicknames on the tracker label).
4. A car may have more than one device (primary OBD + portable). If live boxes disagree on miles → **HOLD**, do not guess.
5. **Last Reading** for oil work = Enterprise odometer at a known fill second + OneStep **miles driven since that second**. Never use OneStep’s own odometer / mileage field as Last Reading.
6. Prefer drive-stop distance when the route returns 200. On **403**, HOLD and spot-check History. Do not invent a trip-miles field.

Sheet columns (Automations Copy, when used): headers **OneStep factory id** and **OneStep device id** — resolve by header name, not letter.

## Common failure modes

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| **401** | Missing/wrong token, bad PEM, expired JWS, or someone sent `?api-key=` / raw Bearer | Reload token + PEM; mint a fresh RS256 JWS; do not fall back to query-key |
| **403** on `/route/drive-stop` | Route not permitted on this account (current live) | Confirm `/device` is 200; **HOLD**; History spot-check; do **not** substitute device odometer |
| **404** on `/v3/api/public` with no path | Hitting the collection root | Call a real resource (`/device`, `/route/drive-stop`, …) |
| Empty **200** on report export | Export id / timing / empty file | Not a substitute for drive-stop miles; retry or use History |
| Wrong miles | Joined on `display_name` or used odometer field | Re-pair on `factory_id`; never use OneStep odo as Last Reading |
| PEM parse errors | Wrong file / empty `cas.pem` / docker CA | Fix path and PEM armor headers; this account cannot call without a real PEM |

### Logging hygiene

- Prefer passing the JWT only in the `Authorization` header, not string-concatenated URLs written to shared logs.
- Redact `Authorization`, JWTs, PEM bodies, and tokens from HAR captures and support tickets.
- Never commit real keys, PEMs, or `.env` files to GitHub.

## Confidence / sources

| Claim | Confidence | Basis |
|-------|------------|--------|
| This account: RS256 JWS `{access_token, exp}` + `Authorization: Bearer <jwt>` | **High** | Live fleet lock 2026-09-04; Go client `internal/onestep` |
| `?api-key=` or raw Bearer for this account | **False** | Same lock; generic OneStep samples do not apply here |
| `/device` 200 with `factory_id`, `device_id`, `display_name` | **High** | Live 200 on this account |
| `/route/drive-stop` 403 on this account | **High** (current) | Live 403 → HOLD / History; do not invent miles |
| Client PEM required | **High** for this account | Token + PEM both required |

When live apidoc and a live response disagree, trust the response and record the fight.

## Related (out of scope for this public repo)

Oil-change program rules, HOLD codes, and Enterprise field contracts live in **Drive collaboration folders**, not in this public GitHub repo. This document is only the **API credential / call** how-to.
