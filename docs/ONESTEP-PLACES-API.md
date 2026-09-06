# OneStep Places / Zones API (Gas Stations only)

Research note for Fleet-Management. **This pipeline is Gas Stations only** (`type_code` `001`, group `Gas_Stations`). Do not create maintenance, shop, or other place types through this code path.

No account IDs, API keys, PEMs, or passwords belong in this file.

## What we already know (high confidence)

Public v3 base (this fleet):

```text
https://track.onestepgps.com/v3/api/public
```

Auth (same as devices / drive-stop — see [`onestep-api-auth.md`](onestep-api-auth.md)):

| Mode | When | How |
|------|------|-----|
| `api-key` query | No PEM | `?api-key=` |
| RS256 JWT Bearer | PEM present in vault / env | `Authorization: Bearer <jwt>` |

Proven public resources used elsewhere in this repo:

| Path | Role |
|------|------|
| `GET /device` | Inventory (`factory_id`, `device_id`, `display_name`) |
| `GET /route/drive-stop` | Miles / stop windows. **Only** source of OneStep miles. Never invent miles. |
| `POST /generate-reports` type `near_address` | Near-address hunt (download often 404; rows come from drive-stop) |

Official apidoc (`https://track.onestepgps.com/v3/apidoc/`) is **login-gated**. Community OneStepGPS samples document `/device` and `/route/drive-stop` only.

## Download / list locations (what we tried)

`oilchange serve` dry-run and `Client.DiscoverPlaces` GET these candidates (limit=50). Status codes are recorded; response bodies are not logged (they can echo credentials).

| Path | Expected use | Confidence |
|------|----------------|------------|
| `GET /v3/api/public/zone` | List geofence zones | Probe live; treat 200+`result_list` as success |
| `GET /v3/api/public/zones` | Plural alias | Same |
| `GET /v3/api/public/marker` | Map markers / POIs | Same |
| `GET /v3/api/public/markers` | Plural alias | Same |
| `GET /v3/api/public/place` | Places catalog | Same |
| `GET /v3/api/public/places` | Plural alias | Same |
| `GET /v3/api/public/geofence` | Legacy geofence name | Same |
| `GET /v3/api/public/geofences` | Plural alias | Same |
| `GET /v3/api/public/poi` | Points of interest | Same |
| `GET /v3/api/public/user-place` | User-owned places | Same |
| `GET /v3/api/public/important-location` | Portal “important location” | Same |

Typical query params (same family as `/device`):

- `api-key` (when not using JWT)
- `limit` (50–500)

### Response shape (when a list parses)

Wrappers tried, in order: `result_list`, `zones`, `markers`, `places`, `geofences`, `data`, `result`, `items`, or a bare array.

Per item we keep:

| Field | Aliases |
|-------|---------|
| `id` | `zone_id`, `marker_id`, `place_id` |
| `name` | `display_name`, `label` |
| `address` | `street_address` |
| `lat` / `lng` | `latitude` / `longitude` / `location.lat` |
| `group` | `group_name`, `folder` |

Gas-station matching after create is **exact Canon label** on `name` (case-insensitive). Example: `A000001_001_SHELL_A_A`.

## Can the public API create zones?

**Not documented as a supported public write.** Portal UX (draw on map → Save) is the proven create path. This repo still POSTs one reviewed gas-station payload at a time to:

| Create try | Body (scaffold) |
|------------|-----------------|
| `POST /marker`, `/markers`, `/place`, `/places` | `{name, address, lat, lng, group:"Gas_Stations", type:"gas"}` |
| `POST /zone`, `/zones`, `/geofence`, `/geofences` | circle, `radius` 25 m, `prefer: canopy_pad` |

If every POST is 404/405/403, **say so in the job `last_error` and HOLD**. Do not invent a zone id. Do not bulk-import. Two SAVE failures → HOLD (Canon rules).

Dry-run (`POST /api/markers/{id}/dry-run`) lists probe statuses and the payload **without sending**.

Live send requires:

1. Human review / approve
2. Third-party geocode (Nominatim default; Mapbox/Google if a key is on Secrets)
3. Confirm phrase `SEND_TO_ONESTEP` plus the one-time `confirm_token`

## Auth for portal vs API

| Credential | Places download | Portal draw |
|------------|-----------------|-------------|
| API key | Yes | No |
| Portal username/password | No (not a public-API substitute) | Yes (browser session) |
| JWT PEM | Optional on public API | n/a |

Store all of the above on the Secrets page (server vault). Never commit them.

## Miles lock (do not violate)

Places / zones / markers **must not** write Last Reading. If a workflow needs miles, call `GET /route/drive-stop` with `device_id`, `dt_tracker_from`, `dt_tracker_to`. Never invent OneStep miles.

## Group naming

Use **`Gas_Stations`**. Canon Place label stays `GeneralCode_Type_Branding_TopTier_TopTierGrade` (`7_3_5_1_1`). Type segment is always `001` on this pipeline.

## Live probe log

Re-run from a desk session (no secrets in the output):

1. Sign in → Status (OneStep ping)
2. Stations → pull → approve one row → Dry-run

Record HTTP statuses from `list_probes` into a PR note. Update the table above when a path is proven 200 with a parseable list.
