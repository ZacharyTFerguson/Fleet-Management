#!/usr/bin/env bash
# Quick Cloudflare Tunnel in front of local Oil Desk (default http://127.0.0.1:4739).
# Prints a public https://*.trycloudflare.com URL. No named-tunnel account required.
set -euo pipefail

TARGET="${1:-http://127.0.0.1:4739}"
BIN="${CLOUDFLARED_BIN:-}"

install_cloudflared() {
  local dest="${HOME}/.local/bin/cloudflared"
  mkdir -p "$(dirname "$dest")"
  echo "cloudflared not found; installing linux-amd64 binary to $dest"
  curl -fsSL -o "$dest" \
    https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64
  chmod +x "$dest"
  echo "$dest"
}

if [[ -z "$BIN" ]]; then
  if command -v cloudflared >/dev/null 2>&1; then
    BIN="$(command -v cloudflared)"
  elif [[ -x "${HOME}/.local/bin/cloudflared" ]]; then
    BIN="${HOME}/.local/bin/cloudflared"
  else
    BIN="$(install_cloudflared)"
  fi
fi

echo "=== Cloudflare quick tunnel ==="
echo "local:  $TARGET"
echo "bin:    $BIN"
echo "Look for https://*.trycloudflare.com in the log below."
echo
exec "$BIN" tunnel --no-autoupdate --url "$TARGET"
