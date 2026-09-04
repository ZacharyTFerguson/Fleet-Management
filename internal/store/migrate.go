package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"oilchange/migrations"
)

// applyMigrations runs 001_schema then any later *.sql except 002_rls (pgx-only).
// Later migrations are dup-column / already-exists tolerant for reopen + enriched CREATE.
func applyMigrations(db *sql.DB, dialect string) error {
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
		var runErr error
		if name == "001_schema.sql" {
			runErr = execAll(db, sqlText)
		} else {
			runErr = execAllIgnoreDup(db, sqlText)
		}
		if runErr != nil {
			return fmt.Errorf("%s: %w", name, runErr)
		}
	}
	if dialect == "pgx" {
		rls, err := migrations.SQL.ReadFile("002_rls.sql")
		if err != nil {
			return fmt.Errorf("read rls: %w", err)
		}
		if err := execAllIgnoreRLS(db, string(rls)); err != nil {
			return fmt.Errorf("rls: %w", err)
		}
	}
	return nil
}

func migrationSQLNames() ([]string, error) {
	ents, err := migrations.SQL.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var extra []string
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		if name == "001_schema.sql" || name == "002_rls.sql" {
			continue
		}
		extra = append(extra, name)
	}
	sort.Strings(extra)
	return append([]string{"001_schema.sql"}, extra...), nil
}

func execAll(db *sql.DB, script string) error {
	for _, stmt := range splitSQL(script) {
		if _, err := db.Exec(stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "already exists") {
				continue
			}
			return fmt.Errorf("%w in %q", err, trimForErr(stmt))
		}
	}
	return nil
}

func execAllIgnoreDup(db *sql.DB, script string) error {
	for _, stmt := range splitSQL(script) {
		if _, err := db.Exec(stmt); err != nil {
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

// execAllIgnoreRLS applies 002_rls.sql. Neon has no Supabase anon/authenticated
// roles; skip those policy statements so ENABLE ROW LEVEL SECURITY still runs.
func execAllIgnoreRLS(db *sql.DB, script string) error {
	for _, stmt := range splitSQL(script) {
		if _, err := db.Exec(stmt); err != nil {
			if rlsIgnorable(err) {
				continue
			}
			return fmt.Errorf("%w in %q", err, trimForErr(stmt))
		}
	}
	return nil
}

func rlsIgnorable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "already exists") {
		return true
	}
	if strings.Contains(msg, "does not exist") && (strings.Contains(msg, "role") || strings.Contains(msg, "user")) {
		return true
	}
	return strings.Contains(msg, "42704")
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
