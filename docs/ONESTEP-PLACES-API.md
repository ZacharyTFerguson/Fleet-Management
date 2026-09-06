# OneStep Places / Zones API (Gas Stations only)

Research note for Fleet-Management. **This pipeline is Gas Stations only** (`type_code` `001`, group `Gas_Stations`). Do not create maintenance, shop, or other place types through this code path.

No account IDs, API keys, PEMs, passwords, or JWTs belong in this file.

## Auth

Public v3 base:

```text
https://track.onestepgps.com/v3/api/public
```

| Mode | When | How |
|------|------|-----|
| RS256 JWT Bearer | PEM present in vault / env | `Authorization: Bearer <jwt>` signed from API key + PEM |
| `api-key` query | No PEM | `?api-key=` |

Never log the PEM or the JWT. Same auth as devices / drive-stop — see [`onestep-api-auth.md`](onestep-api-auth.md).

Portal map (human draw): `https://track.onestepgps.com/v3/ux/map/`

## Proven download (live)

Use **only** these list calls. Paginate with `limit` + `offset`. Do **not** send `belonging_to_groups` or `return_count=true` (live **500**).

| Step | Path | Query | What we keep |
|------|------|-------|----------------|
| 1 | `GET /zone-group` | `limit=50&offset=0` | Group whose `display_name` is `Gas_Stations`. Read `zone_id_list`. Historical live group id is an opaque string — do not hardcode it; look it up. |
| 2 | `GET /zone` | `limit=100&offset=N` | Keep a zone if `zone_id` is in that list **or** `zone_group_id_list` contains the group id. |

`GET /marker?limit=&offset=` also works for markers. It is not a substitute for the Gas_Stations zone list.

### Fields we parse (sample zone)

| Field | Where |
|-------|--------|
| `zone_id` | id |
| `display_name` | Canon label (`A000001_001_SHELL_A_A`) |
| `detail.lat_lng.{lat,lng}` | coordinates |
| `detail.custom_fields.address.value` | street address |
| `shape_data.vertices` | polygon (display only; we do not invent a circle from it) |
| `zone_group_id_list` | group membership |

Wrappers: `result_list` (then `zone_groups` / `data` / `result`). Gas-station matching after download is **exact Canon label** on `display_name` (case-insensitive).

## Failed / do-not-use paths (live)

| Path | Live result | Do not use |
|------|-------------|------------|
| `GET /zone/:id` | **500** | Single-zone fetch |
| `GET /zone` with `belonging_to_groups` | **500** | Group filter query |
| `GET /zone` with `return_count=true` | **500** | Count wrapper |
| `POST /zone-list-by-ids` (docs) | **404** | Batch-by-ids |

Older candidate probes (`/places`, `/geofences`, `/poi`, `/user-place`, `/important-location`) are not the proven Gas_Stations download. `DiscoverPlaces` still pings `/zone-group`, `/zone`, and `/marker` for dry-run status only. Bodies are never logged.

## Create / update — not proven (portal-first)

Documented `POST` / `PUT` zone and marker exist. **Create via API is not proven** on this fleet key. `PUT` update has historically returned **auth error 500**.

Until a live POST/PUT succeeds on this key:

1. Review the station, geocode, dry-run.
2. **Open the portal** and draw the Gas_Stations marker / canopy zone there.
3. Re-download (`GET /zone-group` + `/zone`) to attach the live `zone_id`.
4. Do not set `ONESTEP_WRITE_PROVEN=1` until that write is proven.

`oilchange serve` refuses `POST` create unless `ONESTEP_WRITE_PROVEN=1`. Dry-run still lists payload + list probes and includes `portal_url`. Send remains confirm-gated (`SEND_TO_ONESTEP` + one-time token). No bulk.

If a proven write later fails 404/405/403, put the job on HOLD. Do not invent a zone id. Two SAVE failures → HOLD (Canon rules).

## Auth for portal vs API

| Credential | Places download | Portal draw |
|------------|-----------------|-------------|
| API key (+ optional JWT PEM) | Yes | No |
| Portal username/password | No (not a public-API substitute) | Yes (browser session) |

Store credentials on the Secrets page (server vault). Never commit them.

## Miles lock (do not violate)

Places / zones / markers **must not** write Last Reading. If a workflow needs miles, call `GET /route/drive-stop` with `device_id`, `dt_tracker_from`, `dt_tracker_to`. Never invent OneStep miles. Never use device odometer as a base.

## Mileage box score (related lock)

Expected at a gas card transaction = last **good maintenance** odometer + drive-stop miles since that maintenance timestamp. Recorded = **gas card transaction** odometer + punch time. OneStep odometer is never the base.

## Group naming

Use **`Gas_Stations`**. Canon Place label stays `GeneralCode_Type_Branding_TopTier_TopTierGrade` (`7_3_5_1_1`). Type segment is always `001` on this pipeline.
