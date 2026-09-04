#!/usr/bin/env bash
# Build a static Oil Desk export (web/out) for oilchange serve / go:embed.
# Temporarily parks App Router API routes — they are served by Go instead.
set -euo pipefail
cd "$(dirname "$0")/.."
API_BAK="$(mktemp -d)"
cleanup() {
  if [[ -d "$API_BAK/api" ]]; then
    rm -rf src/app/api
    mv "$API_BAK/api" src/app/api
  fi
  rmdir "$API_BAK" 2>/dev/null || true
}
trap cleanup EXIT
if [[ -d src/app/api ]]; then
  mv src/app/api "$API_BAK/api"
fi
export OILCHANGE_STATIC=1
rm -rf out
npx next build
