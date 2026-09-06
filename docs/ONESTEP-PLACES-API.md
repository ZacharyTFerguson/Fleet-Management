# OneStep Places / Zones API (Gas Stations only)

Liaison live research for Fleet-Management. **This pipeline is Gas Stations only** (`type_code` `001`, group `Gas_Stations`). Do not create maintenance, shop, or other place types through this code path.

No API keys, PEMs, passwords, or JWTs belong in this file. Resource ids below are public OneStep objects (not credentials); they can change — still look up `Gas_Stations` by `display_name`.

## Auth

Public v3 base:

```text
https://track.onestepgps.com/v3/api/public
```

This fleet signs a short-lived **RS256 JWT** from the account API key + private PEM. Claims are `{access_token, exp}`. Send it as:

```text
Authorization: Bearer <jwt>
```

Without a PEM, fall back to `?api-key=`. Never log the PEM or the JWT. Implementation: `internal/onestep/jwt.go`. Also see [`onestep-api-auth.md`](onestep-api-auth.md).

Portal map (human draw): `https://track.onestepgps.com/v3/ux/map/`

## Proven download (live)

Use **only** these list calls. Paginate with `limit` + `offset`. Do **not** send `belonging_to_groups` or `return_count=true` (live **500**).

| Step | Path | Query | What we keep |
|------|------|-------|----------------|
| 1 | `GET /zone-group` | `limit=50&offset=0` | Group whose `display_name` is `Gas_Stations`. Read `zone_id_list` (~14 on the research day). Observed live group id: `6ldgMVSEPkDtz-81f07-1k` — look it up; do not treat that string as a constant join key. |
| 2 | `GET /zone` | `limit=100&offset=N` | Keep a zone if `zone_id` is in that list **or** `zone_group_id_list` **contains** the group id. |

`GET /marker?limit=&offset=` works. Portal **Places** are often the same objects as zones. Markers are not a substitute for the Gas_Stations zone list.

Do **not** `GET /zone/:id` (live **500**). Do **not** `POST /zone-list-by-ids` (docs path, live **404**). List + filter only.

### Sample zone (A000001)

Observed live `zone_id` `6ldgUl0NN2euKF81f07-1k` (`display_name` Canon label, `zone_type` polygon):

| Field | Where |
|-------|--------|
| `zone_id` | id |
| `display_name` | Canon label (`A000001_001_…`) |
| `zone_type` | `polygon` on this sample |
| `detail.lat_lng.{lat,lng}` | coordinates |
| `detail.custom_fields.address.value` | street address |
| `shape_data.vertices` | flat `[lat,lng,lat,lng,…]` (display / re-review only; do not invent a circle from it) |
| `zone_group_id_list` | group membership |

Wrappers: `result_list` (then `zone_groups` / `data` / `result`). After download, match on **exact Canon label** (`display_name`, case-insensitive). Type segment must be `001`.

## Failed / do-not-use paths (live)

| Path | Live result | Do not use |
|------|-------------|------------|
| `GET /zone/:id` | **500** | Single-zone fetch — use list + filter |
| `GET /zone` with `belonging_to_groups` | **500** | Group filter query |
| `GET /zone` with `return_count=true` | **500** | Count wrapper |
| `POST /zone-list-by-ids` (docs) | **404** | Batch-by-ids |

`DiscoverPlaces` still pings `/zone-group`, `/zone`, and `/marker` for dry-run status only. Bodies are never logged (they can echo credentials).

## Create / update — not proven (portal-first)

Documented writes:

| Method | Path |
|--------|------|
| POST / PUT | `/zone` |
| POST / PUT | `/marker` |
| POST | `/marker-list` |

Proven on this key: **read YES**. PUT update previously returned **auth error 500**. **Create via API is not proven** — portal UI only so far.

Pipeline (no shortcuts):

1. Format the Canon label (`7_3_5_1_1`, type `001`, group `Gas_Stations`).
2. Human review.
3. Third-party map check (Nominatim / OSM default).
4. Re-review vs OneStep GPS (`detail.lat_lng` after a list download).
5. **Portal-first** or gated dry-run until a live POST/PUT is proven. Then one confirm-gated send (`SEND_TO_ONESTEP` + one-time token). No bulk.

`oilchange serve` refuses create/update unless `ONESTEP_WRITE_PROVEN=1`. Dry-run lists the payload, proven GET probes, documented write paths, and `portal_url`. It does **not** POST.

If a later proven write fails 404/405/403, HOLD. Do not invent a zone id. Two SAVE failures → HOLD (Canon rules).

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
