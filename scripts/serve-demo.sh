#!/usr/bin/env bash
# Offline Oil Desk demo: testdata CSVs → SQLite → HTTP on 127.0.0.1:4739.
# Does not call live eFleets, OneStep, Neon, or Supabase.
# Does not invent miles. compute may exit 2 (open HOLDs) without drive-stop miles.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ADDR="${OILCHANGE_SERVE_ADDR:-127.0.0.1:4739}"
DB="${OILCHANGE_DB:-$ROOT/oilchange.sqlite}"
MIRROR="${FLEET_MIRROR_PATH:-$ROOT/data/runtime/cars.json}"
BIN="${OILCHANGE_BIN:-$ROOT/bin/oilchange}"

VEHICLES="${OILCHANGE_VEHICLES:-$ROOT/testdata/enterprise/fleetsummary_live.csv}"
FUEL="${OILCHANGE_FUEL_DETAILS:-$ROOT/testdata/enterprise/details_live.csv}"
SHOP="${OILCHANGE_SHOP_RO:-$ROOT/testdata/enterprise/maintenance.csv}"

export OILCHANGE_DB="$DB"

echo "=== Oil Desk DEMO (offline / file-drop testdata) ==="
echo "sqlite:  $DB"
echo "mirror:  $MIRROR  (not web/data/cars.json — committed mirror stays put)"
echo "listen:  http://$ADDR"
echo "Last Reading stays HOLD without OneStep drive-stop miles. No invented miles."
echo

if [[ ! -x "$BIN" ]]; then
  echo "building $BIN"
  mkdir -p "$(dirname "$BIN")"
  go build -o "$BIN" ./cmd/oilchange
fi

mkdir -p "$(dirname "$MIRROR")"

"$BIN" sync-enterprise --vehicles "$VEHICLES" --fuel-details "$FUEL" --shop-ro "$SHOP"
# exit 2 = open HOLDs; expected without OneStep. Last Reading is not written on HOLD.
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
echo "Oil Desk DEMO labeled source=mock-mirror. Open http://$ADDR"
echo "Tunnel: scripts/cloudflare-tunnel.sh"
exec "$BIN" serve --addr "$ADDR" --mirror "$MIRROR"
