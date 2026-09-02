# OneStepGPS API credentials and usage

Practical guide for calling the One Step GPS (OneStep / OneStepGPS) **public v3 API** for this fleet. No real secrets belong in this repository.

## What you need

| Credential | Required? | Notes |
|------------|-----------|--------|
| **API key** | Yes | Issued by OneStep (commonly via `integration@onestepgps.com`). Agents should load it from env / local settings — never ask the owner to paste it into chat. |
| **Base URL** | Yes (default below) | `https://track.onestepgps.com/v3/api/public` |
| **Client PEM / private key** | No for public API | Working public-API callers authenticate with the API key only. Do not invent a client-cert or “sign your own JWT with a PEM” flow unless OneStep gives you that contract in writing. |
| **Account / portal login** | Portal only | Browser History / Reports use the signed-in PDI Health session on `track.onestepgps.com`. That is separate from the public API key. |

Org research briefs sometimes say “auth is JWT RS256.” Treat that as a description of **OneStep’s token format / portal crypto**, not as “you must load a PEM and mint JWTs yourself.” Pass the API key **as issued**.

### Where the key lives (this org)

- Prefer an env var or a **gitignored** local settings file (oil-change / agent app settings).
- Never commit the key, never paste it into Slack/chat/Drive docs that are broadly shared, never put it in this public repo.
- If a call 401s, look for the existing on-file key and retry — do not stop at the first auth wall and do not request a new key from Zachary unless the stored key is proven revoked.

## Environment variables (recommended)

```bash
# Required
export ONESTEP_API_KEY='YOUR_API_KEY'

# Optional overrides
export ONESTEP_BASE_URL='https://track.onestepgps.com/v3/api/public'

# Only if OneStep later issues a *client* private key for a different auth mode
# (not used by the standard public api-key flow documented here)
export ONESTEP_PEM_PATH='/secure/path/to/onestep-client.pem'
```

Suggested local files (examples only — keep them out of git):

- `.env` / `.env.local` (gitignored)
- `onestep.env` next to a Go binary (gitignored)

Add patterns like `.env`, `*.pem`, `*onestep*secret*` to `.gitignore` before creating any secret files.

## How auth works (public API)

Successful third-party and documented callers use a **query parameter**:

```text
api-key=YOUR_API_KEY
```

Base:

```text
https://track.onestepgps.com/v3/api/public
```

Example device list (proven pattern from public OneStepGPS Go samples):

```text
GET {ONESTEP_BASE_URL}/device?latest_point=true&api-key=YOUR_API_KEY
```

Also used by this fleet for miles-since:

```text
GET {ONESTEP_BASE_URL}/route/drive-stop?...&api-key=YOUR_API_KEY
```

Exact `drive-stop` query parameter names come from OneStep’s live apidoc (signed-in portal / docs OneStep provides with the key). Do not invent params. You will need at least a **`device_id`** and a **time window or since-timestamp** that matches the Enterprise fill second (America/New_York naive times in oil work).

### About PEM files

- A Drive file named `cas.pem` (empty / docker CA placeholder) is **not** the OneStep API credential.
- For the **public API**, you do not need to load a PEM to call `/device` or `/route/drive-stop`.
- If you are ever given a real client private key PEM for a special OneStep contract:
  - Store it outside the repo (`ONESTEP_PEM_PATH`).
  - Permissions `0600`, owner-only.
  - Prefer PKCS#8 or traditional RSA PEM (`BEGIN PRIVATE KEY` / `BEGIN RSA PRIVATE KEY`).
  - Never log the PEM body; never commit it; never base64-dump it into tickets.
  - Validate format with `openssl pkey -in "$ONESTEP_PEM_PATH" -check -noout` (shows validity only).

## Minimal successful requests

### curl — list devices

```bash
curl -sS -G \
  "${ONESTEP_BASE_URL:-https://track.onestepgps.com/v3/api/public}/device" \
  --data-urlencode "latest_point=true" \
  --data-urlencode "api-key=${ONESTEP_API_KEY}"
```

Expect HTTP **200** and JSON with a `result_list` of devices. Each device includes (among other fields):

- `factory_id` — hardware id (pairing key for this fleet)
- `device_id` — portal / History id
- `display_name` — label only (never a join key)

### curl — drive-stop (miles since a timestamp)

```bash
# Replace QUERY… with the live apidoc params for device + time range.
# Always include api-key. Never print the key in CI logs.
curl -sS -G \
  "${ONESTEP_BASE_URL:-https://track.onestepgps.com/v3/api/public}/route/drive-stop" \
  --data-urlencode "api-key=${ONESTEP_API_KEY}" \
  # --data-urlencode "device_id=YOUR_DEVICE_ID" \
  # --data-urlencode "<from/to or since params from apidoc>"
```

Fleet note: this route previously **403**’d for an older key state and later returned **200** for miles-since waves. A 403 is a permissions/key-scope problem, not a reason to fall back to OneStep’s odometer field.

### Go — minimal client sketch

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

func main() {
	key := os.Getenv("ONESTEP_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "ONESTEP_API_KEY is required")
		os.Exit(1)
	}
	base := os.Getenv("ONESTEP_BASE_URL")
	if base == "" {
		base = "https://track.onestepgps.com/v3/api/public"
	}

	u, err := url.Parse(base + "/device")
	if err != nil {
		panic(err)
	}
	q := u.Query()
	q.Set("latest_point", "true")
	q.Set("api-key", key)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%d bytes=%d\n", resp.StatusCode, len(body))
	// Do not fmt.Print the URL after Encode if logs are shared — it contains the key.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
```

## Useful endpoints (fleet context)

| Path | Use |
|------|-----|
| `GET /device` | Inventory; map `factory_id` ↔ `device_id`; optional `latest_point=true` |
| `GET /route/drive-stop` | Miles / drive-stop data since a timestamp (preferred miles-since source when it returns 200) |
| History / reports in the **portal** | Spot-check when GPS went dark while the car still moved; signed-in browser, not the API key |

Related portal (not API key auth):

- History UI: `https://track.onestepgps.com/v3/ux/map/history/single?device_id=…`
- Reports dashboard: `https://track.onestepgps.com/v3/ux/reports/dashboard`

Other public areas mentioned in research briefs (device drives, history, report-export): confirm against live apidoc before coding. Do not invent paths.

## Fleet pairing rules (locked)

1. Pair cars by **`factory_id`** (hardware).
2. Use **`device_id`** for History / drive-stop calls after pairing.
3. Never join on **`display_name`** (or driver nicknames on the tracker label).
4. A car may have more than one device (primary OBD + portable). If devices fight the fuel/odometer band → **HOLD**, do not guess.
5. **Last Reading** for oil work = Enterprise odometer at a known fill second + OneStep **miles driven since that second**. Never use OneStep’s own odometer / mileage field as Last Reading.
6. Prefer trip/GPS / drive-stop distance evidence. Spot-check History every so often; the owner may prefer History over API when the tracker was quiet (example: VA19).

Sheet columns (Automations Copy, when used): headers **OneStep factory id** and **OneStep device id** — resolve by header name, not letter.

## Common failure modes

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| **401** | Missing/wrong `api-key`, key revoked, typo/whitespace | Reload from env/settings; trim spaces; confirm with a bare `/device` call |
| **403** on `/route/drive-stop` | Key lacks route scope (seen historically on this account) | Confirm key with `/device` first; ask OneStep to enable the route or refresh the key — do **not** substitute device odometer |
| **404** on `/v3/api/public` with no path | Hitting the collection root | Call a real resource (`/device`, `/route/drive-stop`, …) |
| Empty **200** on report export | Export id / timing / empty file | Not a substitute for drive-stop miles; retry or use History |
| “Need a key” docs page | Docs gate | Use the on-file key; do not quit |
| Wrong miles | Joined on `display_name` or used odometer field | Re-pair on `factory_id`; sum trip/GPS / drive-stop since fill second |
| PEM parse errors | Wrong file / empty `cas.pem` / docker CA | Public API does not need PEM; if using a client key, fix path and PEM armor headers |

### Logging hygiene

- Prefer passing the key only in memory / `url.Values`, not string-concatenated URLs written to shared logs.
- Redact `api-key` from HAR captures and support tickets.
- Never commit real keys, PEMs, or `.env` files to GitHub.

## Confidence / sources

| Claim | Confidence | Basis |
|-------|------------|--------|
| Base URL `…/v3/api/public` + `api-key` query auth | **High** | Public OneStepGPS Go sample (`LatestPointsGoServer`); third-party `/device` captures; Alvys/Fleetio “request API key” flow |
| `/device` returns `factory_id`, `device_id`, `display_name` | **High** | Public JSON examples; matches this fleet’s sheet U/V columns |
| `/route/drive-stop` is the miles-since call | **High** for path; **medium** for exact query param names | Org LEARNED / Liaison notes (200 after prior 403); exact params = live apidoc |
| Client PEM required for public API | **Low / false for current practice** | No org script or Slack evidence of mTLS/PEM signing; successful pattern is API key only |
| “JWT RS256” | **Medium (interpretive)** | Agent Research/Probe briefs; treat as token format metadata, not a client PEM requirement |

When live apidoc and a live response disagree, trust the response and record the fight.

## Related (out of scope for this public repo)

Oil-change program rules, HOLD codes, and Enterprise field contracts live in **Drive collaboration folders**, not in this public GitHub repo. This document is only the **API credential / call** how-to.
