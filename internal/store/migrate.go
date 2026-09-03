package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"oilchange/migrations"
)

// applyMigrations runs schema and additive migrations before Postgres-only RLS.
func applyMigrations(ctx context.Context, db *sql.DB, dialect string) error {
	names, err := migrationSQLNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		raw, err := migrations.SQL.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		sqlText := string(raw)
		if dialect == "sqlite" {
			sqlText = sqliteSchema(sqlText)
		}
		run := execAll
		if name != "001_schema.sql" {
			run = execAllIgnoreDup
		}
		if err := run(ctx, db, sqlText); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if dialect == "pgx" {
		rls, err := migrations.SQL.ReadFile("002_rls.sql")
		if err != nil {
			return fmt.Errorf("read rls: %w", err)
		}
		if err := execAll(ctx, db, string(rls)); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "already exists") {
				return fmt.Errorf("rls: %w", err)
			}
		}
	}
	return nil
}

func migrationSQLNames() ([]string, error) {
	entries, err := migrations.SQL.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var extra []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") || name == "001_schema.sql" || name == "002_rls.sql" {
			continue
		}
		extra = append(extra, name)
	}
	sort.Strings(extra)
	return append([]string{"001_schema.sql"}, extra...), nil
}

func execAll(ctx context.Context, db *sql.DB, script string) error {
	for _, stmt := range splitSQL(script) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "already exists") {
				continue
			}
			return fmt.Errorf("%w in %q", err, trimForErr(stmt))
		}
	}
	return nil
}

func execAllIgnoreDup(ctx context.Context, db *sql.DB, script string) error {
	for _, stmt := range splitSQL(script) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "duplicate column") ||
				strings.Contains(msg, "already exists") ||
				strings.Contains(msg, "already exist") {
				continue
			}
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
	s = strings.ReplaceAll(s, "now()", "datetime('now')")
	s = strings.ReplaceAll(s, "DATE NOT NULL", "TEXT NOT NULL")
	return s
}
