# Live drive-stop probes (2026-09-04)

What “live probe” means here: **one GET** to OneStep `drive-stop` with all three query params. It does **not** refresh Last Reading, does **not** loop, and does **not** invent miles. `oilchange probe-onestep` prints measured `distance` (or a measured 0) and exits.

Command (repo root, secrets from gitignored `oilchange.env`):

```text
oilchange probe-onestep --device-id DEVICE --hours 6,24,48
oilchange probe-onestep --device-id ID1,ID2,ID3 --hours 48
```

Auth tonight: **jwt-rs256** (`Authorization: Bearer` minted RS256 JWS). Not raw token. Empty query was not sent (that is the misleading 403).

## Same vehicle, three windows

Bing-2 / eFleets `26LSZW` / factory `3271116717` / `device_id=6hBKgc-2QbuMHF81f07--k`

| Window | From (UTC) | To (UTC) | Miles | Elapsed | Auth |
|--------|------------|----------|-------|---------|------|
| 6h | 2026-09-03T21:27:50Z | 2026-09-04T03:27:50Z | **0.0000** | 694ms | jwt-rs256 |
| 24h | 2026-09-03T03:27:50Z | 2026-09-04T03:27:50Z | **161.0982** | 617ms | jwt-rs256 |
| 48h | 2026-09-02T03:27:50Z | 2026-09-04T03:27:50Z | **263.6553** | 550ms | jwt-rs256 |

48h **263.6553** matches the earlier smoke on this factory (**263.66**). 6h **0** is a measured empty trip list overnight, not a HOLD and not a guessed odometer.

## Different vehicles, 48h each

| device_id | eFleets | Miles (48h) | Elapsed |
|-----------|---------|-------------|---------|
| `6irN3lPcP_5v8V81f07-1F` | 27QSQG | **293.8496** | 743ms |
| `6irN7uCmPZevJ-81f07-1F` | 285JCZ | **322.1269** | 524ms |
| `6g12RgoZQBmPkV81f07--V` | 292ND8 | **0.0000** | 136ms |

All six calls HTTP-success via the probe command. No 403. No odometer field used. Mutex serializes in-process drive-stop HTTP (unit test: 16 concurrent probes, in-flight max 1).

## How it worked

1. Mint short-lived RS256 JWT from the account PEM + API key (never logged).
2. `GET /v3/api/public/route/drive-stop?device_id=&dt_tracker_from=&dt_tracker_to=&stop_duration=5m0s`
3. Miles from `distance.value` / `drive_stop_list` only. `odometer` / `odometer_from` / `odometer_to` ignored.
4. Probe does **not** call `compute` and does **not** write sqlite Last Reading.

## Do not

- Treat 6h/48h **0.0000** as “broken API.” That is measured zero for that window.
- Treat an empty-query 403 as “API disabled.”
- Join on `display_name`.
- Use `testdata/onestep/map.csv` on the live roster.

## Code

- `internal/onestep/probe.go` — `ProbeDriveStop`
- `internal/onestep/client.go` — `fetchDriveStopWindow` holds `Client.mu`
- `cmd/oilchange/probe_cmd.go` — CLI
- Tests: `internal/onestep/probe_test.go`, `cmd/oilchange/probe_cmd_test.go`
