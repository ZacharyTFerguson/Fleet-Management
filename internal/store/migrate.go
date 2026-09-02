package store

import (
	"database/sql"
	"fmt"
	"strings"

	"oilchange/migrations"
)

// applyMigrations runs schema SQL. SQLite is a test double; RLS is Postgres/fleet-oil only.
func applyMigrations(db *sql.DB, dialect string) error {
	schema, err := migrations.SQL.ReadFile("001_schema.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	sqlText := string(schema)
	if dialect == "sqlite" {
		sqlText = sqliteSchema(sqlText)
	}
	if err := execAll(db, sqlText); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	if dialect == "pgx" {
		rls, err := migrations.SQL.ReadFile("002_rls.sql")
		if err != nil {
			return fmt.Errorf("read rls: %w", err)
		}
		if err := execAll(db, string(rls)); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "already exists") {
				return fmt.Errorf("rls: %w", err)
			}
		}
	}
	return nil
}

// execAll splits statements because the sqlite driver will not run a whole file in one Exec.
func execAll(db *sql.DB, script string) error {
	for _, stmt := range splitSQL(script) {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("%w in %q", err, trimForErr(stmt))
		}
	}
	return nil
}

func splitSQL(s string) []string {
	var out []string
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if strings.HasSuffix(trim, ";") {
			stmt := strings.TrimSpace(b.String())
			b.Reset()
			if stmt != "" {
				out = append(out, stmt)
			}
		}
	}
	if rest := strings.TrimSpace(b.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}

func trimForErr(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// sqliteSchema rewrites Postgres types so OILCHANGE_DB tests do not need Supabase.
func sqliteSchema(s string) string {
	s = strings.ReplaceAll(s, "BIGSERIAL PRIMARY KEY", "INTEGER PRIMARY KEY AUTOINCREMENT")
	s = strings.ReplaceAll(s, "TIMESTAMPTZ", "TEXT")
	s = strings.ReplaceAll(s, "DOUBLE PRECISION", "REAL")
	s = strings.ReplaceAll(s, "BOOLEAN NOT NULL DEFAULT false", "INTEGER NOT NULL DEFAULT 0")
	s = strings.ReplaceAll(s, "BOOLEAN NOT NULL DEFAULT true", "INTEGER NOT NULL DEFAULT 1")
	s = strings.ReplaceAll(s, "BOOLEAN", "INTEGER")
	s = strings.ReplaceAll(s, "DEFAULT now()", "DEFAULT (datetime('now'))")
	s = strings.ReplaceAll(s, "DATE NOT NULL", "TEXT NOT NULL")
	return s
}
