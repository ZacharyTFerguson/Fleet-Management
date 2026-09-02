# Fleet-Management

Records **important locations**: places where a vehicle sits still long enough to matter.

This automation run received an empty trigger body — no vehicle was named and no GPS ping was sent — so no live unit was written to the catalog. The recorder is ready for the next ping.

## Sit rule

A ping is treated as sitting when speed is missing or at most **3 mph**.

If that vehicle stays inside **100 meters** for **15 minutes**, the place is stored. A silent gap longer than **60 minutes** closes the sit. Later sits by the same vehicle within **150 meters** update the same location and increment the visit count.

## Run

Node 20+, no npm packages.

```bash
npm test
npm start
```

Open http://127.0.0.1:3000

- Human: the page lists recorded places and can send pings
- JSON: `GET /api/v1/important-locations` and `GET /api/v1/important-locations?vehicle=`
- Ingest: `POST /api/v1/pings` with `{ "vehicleId", "lat", "lng", "recordedAt", "speedMph" }`

## CLI (automation / webhook)

```bash
node bin/record-location.js --vehicle DEMO-1 --lat 40.7128 --lng -74.006 --speed 0 --at 2026-09-02T12:00:00.000Z
```

Or pipe JSON on stdin. An empty body exits 0 and reports `recorded: false` (`missing_vehicle` or `no_vehicle_payload`). It does not invent a location.

Catalog file: `data/important-locations.json` (override with `--file` or `IMPORTANT_LOCATIONS_FILE`).

## What this repo does not do

- Does not copy live eFleets inventory, plates, VINs, or customer GPS into git
- Does not write oil-tracker sheet columns G/H or U/V
- Does not send email
