package migrations

import "embed"

// SQL is schema for fleet-oil. Comments mark each table ready for that Supabase project.
//
//go:embed *.sql
var SQL embed.FS
