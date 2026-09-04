// Package webui embeds the static Oil Desk export (web/out) for oilchange serve.
// Rebuild with: cd web && npm ci && npm run build:static
package webui

import "embed"

// Out is the Next.js static export (index.html, devices/, _next/, …).
//
//go:embed all:out
var Out embed.FS
