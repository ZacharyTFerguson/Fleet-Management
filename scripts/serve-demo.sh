#!/usr/bin/env bash
# File-drop roster + richest DETAILS available + shop ROs → SQLite → Oil Desk.
# Live OneStep (JWT Bearer) for device pairing + drive-stop miles when a token is present.
# Does not invent miles. HOLD stays HOLD without a real drive-stop result.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ADDR="${OILCHANGE_SERVE_ADDR:-127.0.0.1:4739}"
DB="${OILCHANGE_DB:-$ROOT/oilchange.sqlite}"
MIRROR="${FLEET_MIRROR_PATH:-$ROOT/data/runtime/cars.json}"
BIN="${OILCHANGE_BIN:-$ROOT/bin/oilchange}"

VEHICLES="${OILCHANGE_VEHICLES:-$ROOT/testdata/enterprise/fleetsummary_live.csv}"
SHOP="${OILCHANGE_SHOP_RO:-$ROOT/testdata/enterprise/maintenance.csv}"
if [[ -f "$ROOT/data/runtime/enterprise/DETAILS_583424_30-Days.csv" ]]; then
  FUEL="${OILCHANGE_FUEL_DETAILS:-$ROOT/data/runtime/enterprise/DETAILS_583424_30-Days.csv}"
  export OILCHANGE_FUEL_SOURCE="${OILCHANGE_FUEL_SOURCE:-Drive DETAILS_583424_30-Days.csv (FergFam export, file-drop)}"
else
  FUEL="${OILCHANGE_FUEL_DETAILS:-$ROOT/testdata/enterprise/details_live.csv}"
  export OILCHANGE_FUEL_SOURCE="${OILCHANGE_FUEL_SOURCE:-testdata/enterprise/details_live.csv}"
fi
export OILCHANGE_MAINT_SOURCE="${OILCHANGE_MAINT_SOURCE:-testdata/enterprise/maintenance.csv (shop RO fixture; live EFLEETS_MAINT_URL missing)}"
export OILCHANGE_DB="$DB"

echo "=== Oil Desk (file-drop + live OneStep when keyed) ==="
echo "sqlite:  $DB"
echo "fuel:    $FUEL"
echo "shop RO: $SHOP"
echo "listen:  http://$ADDR/records/"
echo

if [[ ! -x "$BIN" ]]; then
  echo "building $BIN"
  mkdir -p "$(dirname "$BIN")"
  go build -o "$BIN" ./cmd/oilchange
fi

mkdir -p "$(dirname "$MIRROR")"

"$BIN" sync-enterprise --vehicles "$VEHICLES" --fuel-details "$FUEL" --shop-ro "$SHOP"

if "$BIN" env | grep -q 'onestep auth: jwt-rs256\|onestep auth: api-key'; then
  echo "OneStep: devices sync (live /device, pair factory_id / exact VIN — not display_name)"
  "$BIN" devices sync || echo "devices sync failed (desk still serves file-drop records)"
  echo "OneStep: drive-stop miles-since after trusted fill (distance only; may take a few minutes)"
  set +e
  "$BIN" sync-onestep
  onestep_rc=$?
  set -e
  if [[ "$onestep_rc" -ne 0 ]]; then
    echo "sync-onestep exited $onestep_rc — HOLDs stay HOLD; no invented miles"
  fi
fi

set +e
"$BIN" compute
compute_rc=$?
set -e
if [[ "$compute_rc" -ne 0 && "$compute_rc" -ne 2 ]]; then
  echo "oilchange compute failed (exit $compute_rc)" >&2
  exit "$compute_rc"
fi

"$BIN" sync --no-remote --mirror "$MIRROR"
echo
echo "Open http://$ADDR/records/  (fuel + shop RO + OneStep). Oil Desk: http://$ADDR/"
echo "Tunnel: scripts/cloudflare-tunnel.sh"
exec "$BIN" serve --addr "$ADDR" --mirror "$MIRROR"
