# OneStepGPS API credentials and usage

Practical guide for calling the One Step GPS (OneStep / OneStepGPS) **public v3 API** for this fleet. No real secrets belong in this repository.

This account does **not** authenticate with `?api-key=` or a raw Bearer API key. Generic OneStep samples that show those are wrong here.

## What you need

| Credential | Required? | Notes |
|------------|-----------|--------|
| **API token** (`access_token` claim) | Yes | Issued by OneStep. Load from env / local settings — never ask the owner to paste it into chat. |
| **Account PEM** (RSA private key) | Yes | Signs a short-lived RS256 JWS. Raw token without the PEM → **401 JWS required**. |
| **Base URL** | Yes (default below) | Host `https://track.onestepgps.com` (paths are under `/v3/api/public`). |
| **Account / portal login** | Portal only | Browser History / Reports are a **spot-check**, not the miles source. |

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

Miles-since (`drive-stop` **is enabled**):

```text
GET {ONESTEP_BASE_URL}/v3/api/public/route/drive-stop?device_id=…&dt_tracker_from=…&dt_tracker_to=…
Authorization: Bearer <rs256-jws>
```

Required query (proven 200): **`device_id`**, **`dt_tracker_from`**, **`dt_tracker_to`**. Pair the car by `factory_id` first; History identity is `device_id`. Never join on `display_name`.

Tracker stamps are naive **America/New_York** `YYYY-MM-DD HH:MM:SS`. `from` is the trusted fill second; `to` is now.

### Live status (this account)

| Call | Status | What to do |
|------|--------|------------|
| `GET /device` | **200** | Inventory. Pair by `factory_id`. `device_id` is History identity. Drop any odometer JSON. |
| `GET /route/drive-stop` **with** the three required query params | **200** | Miles-since. Use **`distance`** / **`drive_stop_list`**. Proven: ~594.9 mi API vs ~595 History. Earlier NJ12 proof: 268.49 API vs 268 History. History UI is spot-check only. |
| `GET /route/drive-stop` **empty / missing query** | Misleading **403** “access denied” | **Not** “API not enabled.” Send `device_id`, `dt_tracker_from`, `dt_tracker_to`. Do **not** invent miles. Do **not** use OneStep’s own odometer as Last Reading. |
| Any call with raw token (no JWS) | **401** | JWS required. Mint RS256 `{access_token, exp}`. |

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

Expect HTTP **200** and JSON with a `result_list` of devices. Each device includes (among other fields):

- `factory_id` — hardware id (pairing key for this fleet)
- `device_id` — portal / History id
- `display_name` — label only (never a join key)

### curl — drive-stop (miles since a timestamp)

```bash
# Required query. Empty/missing query looks like 403 access denied — that is not “API not enabled.”
curl -sS -G \
  "${ONESTEP_BASE_URL:-https://track.onestepgps.com}/v3/api/public/route/drive-stop" \
  --data-urlencode "device_id=${DEVICE_ID}" \
  --data-urlencode "dt_tracker_from=${FROM}" \
  --data-urlencode "dt_tracker_to=${TO}" \
  -H "Authorization: Bearer ${jwt}"
```

Use JSON **`distance`** / **`drive_stop_list`**. Drop any odometer field. Never use OneStep’s own odometer as Last Reading.

## Useful endpoints (fleet context)

| Path | Use |
|------|-----|
| `GET /device` | Inventory; map `factory_id` ↔ `device_id`; optional `latest_point=true` |
| `GET /route/drive-stop` | Miles-since. Required: `device_id`, `dt_tracker_from`, `dt_tracker_to`. Read `distance` / `drive_stop_list`. |
| History / reports in the **portal** | Spot-check only (example: ~594.9 API vs ~595 History). Never the stored miles source. |

Related portal (not API key auth):

- History UI: `https://track.onestepgps.com/v3/ux/map/history/single?device_id=…`
- Reports dashboard: `https://track.onestepgps.com/v3/ux/reports/dashboard`

## Fleet pairing rules (locked)

1. Pair cars by **`factory_id`** (hardware).
2. Use **`device_id`** for History / drive-stop calls after pairing.
3. Never join on **`display_name`** (or driver nicknames on the tracker label).
4. A car may have more than one device (primary OBD + portable). If devices fight the fuel/odometer band → **HOLD**, do not guess.
5. **Last Reading** for oil work = Enterprise odometer at a known fill second + OneStep **miles driven since that second**. Never use OneStep’s own odometer / mileage field as Last Reading.
6. Prefer drive-stop **`distance`**. Spot-check History; do not store History UI miles.

Sheet columns (Automations Copy, when used): headers **OneStep factory id** and **OneStep device id** — resolve by header name, not letter.

## Common failure modes

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| **401** | Raw token / missing JWS | Mint RS256 `{access_token, exp}` with the account PEM. Do not send `?api-key=`. |
| **403** on `/route/drive-stop` | Empty or missing `device_id` / `dt_tracker_from` / `dt_tracker_to` | **Misleading “access denied.”** The route is enabled. Fill the three query params. Do **not** treat this as “API not enabled.” Do **not** substitute device odometer. |
| **404** on `/v3/api/public` with no path | Hitting the collection root | Call a real resource (`/device`, `/route/drive-stop`, …) |
| Empty **200** on report export | Export id / timing / empty file | Not a substitute for drive-stop miles; retry or use History as a spot-check |
| Wrong miles | Joined on `display_name` or used odometer field | Re-pair on `factory_id`; use `distance` / `drive_stop_list` since the fill second |
| PEM parse errors | Wrong file / empty `cas.pem` / docker CA | Fix path and PEM armor headers; this account needs the real client key |

### Logging hygiene

- Prefer passing credentials only in memory, not string-concatenated URLs written to shared logs.
- Redact `Authorization` and any leftover `api-key` from HAR captures and support tickets.
- Never commit real keys, PEMs, or `.env` files to GitHub.

## Confidence / sources

| Claim | Confidence | Basis |
|-------|------------|--------|
| Auth is RS256 JWS `{access_token, exp}` as `Authorization: Bearer` | **High** | This account: raw token → 401 JWS required |
| `/device` returns `factory_id`, `device_id`, `display_name` | **High** | Live `/device` 200 |
| `/route/drive-stop` is enabled with `device_id` + `dt_tracker_from` + `dt_tracker_to` | **High** | Liaison proven 200 (~594.9 mi vs History ~595). Empty query → misleading 403. |
| Miles fields are `distance` / `drive_stop_list` | **High** | Same proven 200. Do not invent a trip-miles alias. |
| Client PEM required | **High** for this account | 401 without JWS |

When live apidoc and a live response disagree, trust the response and record the fight.

## Related (out of scope for this public repo)

Oil-change program rules, HOLD codes, and Enterprise field contracts live in **Drive collaboration folders**, not in this public GitHub repo. This document is only the **API credential / call** how-to.
