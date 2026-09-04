package config

import (
	"fmt"
	"os"
	"strings"
)

// Config is process env. Secrets stay in the environment; nothing here writes them to disk.
type Config struct {
	DatabaseURL       string
	SQLitePath        string
	SupabaseURL       string
	ServiceRole       string
	SyncSecret        string // SUPABASE_SYNC_SECRET for fleet-sync edge function
	SupabaseAnonKey   string // SUPABASE_GROK_BUILD_KEY / SUPABASE_ANON_KEY (publishable; SELECT only)
	OneStepToken      string
	OneStepPrivateKey string
	OneStepPublicKey  string
	OneStepBase       string
	EFleetsUser       string
	EFleetsPass       string
	EFleetsCust       string
	EFleetsBase       string
	EFleetsDetails    string
	EFleetsMaint      string
	EFleetsFleet      string
}

// Load reads oilchange.env then process env. Empty secrets are allowed so tests and file-drop sync do not need a live account.
func Load() Config {
	if err := loadDotEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "oilchange.env: %v\n", err)
	}
	base := getenv("ONESTEP_BASE_URL", "https://track.onestepgps.com")
	return Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		SQLitePath:      getenv("OILCHANGE_DB", ""),
		SupabaseURL:     os.Getenv("SUPABASE_URL"),
		ServiceRole:     os.Getenv("SUPABASE_SERVICE_ROLE"),
		SyncSecret:      os.Getenv("SUPABASE_SYNC_SECRET"),
		SupabaseAnonKey: firstEnv("SUPABASE_GROK_BUILD_KEY", "SUPABASE_ANON_KEY"),
		// Cloud Agent secret names: OneStepAPIKEYTobeSigned (API key) + OneStepAPIKEY (PEM for JWS).
		OneStepToken: firstEnv("ONESTEP_API_TOKEN", "ONESTEP_API_KEY", "ONE_STEP_FULL_API_KEY", "OneStepAPIKEYTobeSigned"),
		OneStepPrivateKey: firstNonEmpty(
			firstEnv("ONESTEP_API_PRIVATEKEY", "ONESTEP_API_PRIVATE_KEY", "OneStepAPIKEY"),
			readEnvFile("ONESTEP_PEM_PATH"),
			readEnvFile("ONESTEP_JWT_PEM_PATH"),
		),
		OneStepPublicKey: firstEnv("ONESTEP_API_PUBLIC_KEY", "ONESTEP_API_PUBLICKEY"),
		OneStepBase:      strings.TrimRight(base, "/"),
		// Cloud Agent secret names match OneStep style (EFleetsUsername) plus oilchange.env names.
		EFleetsUser:    firstEnv("EFLEETS_USERNAME", "EFLEETS_USER", "EFleetsUsername", "EFleetsUser"),
		EFleetsPass:    firstEnv("EFLEETS_PASSWORD", "EFLEETS_PASS", "EFleetsPassword", "EFleetsPass"),
		EFleetsCust:    firstEnv("EFLEETS_CUST_NUM", "EFLEETS_CUSTOMER", "EFleetsCustNum"),
		EFleetsBase:    getenv("EFLEETS_BASE_URL", "https://login.efleets.com"),
		EFleetsDetails: firstEnv("EFLEETS_DETAILS_URL", "EFleetsDetailsURL"),
		EFleetsMaint:   firstEnv("EFLEETS_MAINT_URL", "EFleetsMaintURL"),
		EFleetsFleet:   firstEnv("EFLEETS_FLEETSUMMARY_URL", "EFleetsFleetSummaryURL"),
	}
}

// EFleetsSecretsHint is the only place live portal creds belong. Do not ask for them in chat.
const EFleetsSecretsHint = "set EFLEETS_USERNAME, EFLEETS_PASSWORD, and EFLEETS_CUST_NUM in gitignored oilchange.env or Cloud Agent secrets (aliases EFleetsUsername / EFleetsPassword / EFleetsCustNum). Never paste them into chat."

// firstEnv is the first non-empty env value so oilchange.env names from OneStep's note still load.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// readEnvFile loads a PEM from ONESTEP_PEM_PATH (Cursor guide). Empty path is a no-op.
func readEnvFile(envKey string) string {
	p := strings.TrimSpace(os.Getenv(envKey))
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "oilchange.env: %s: %v\n", envKey, err)
		return ""
	}
	return string(b)
}

// DSN picks sqlite when OILCHANGE_DB is set so unit tests never touch live Supabase.
func (c Config) DSN() (driver, dsn string, err error) {
	if c.SQLitePath != "" {
		return "sqlite", c.SQLitePath, nil
	}
	if c.DatabaseURL != "" {
		return "pgx", c.DatabaseURL, nil
	}
	return "", "", fmt.Errorf("set OILCHANGE_DB or DATABASE_URL")
}

// getenv keeps defaults like OneStep base URL out of committed secrets files.
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// EnvReport describes loaded secrets without printing values. Used by `oilchange env`.
func (c Config) EnvReport() []string {
	line := func(name, v string, extra string) string {
		s := name + ": missing"
		if strings.TrimSpace(v) != "" {
			s = fmt.Sprintf("%s: set (%d bytes)", name, len(v))
		}
		if extra != "" {
			s += " " + extra
		}
		return s
	}
	page := func(u string) string {
		if strings.TrimSpace(u) == "" {
			return ""
		}
		lu := strings.ToLower(u)
		if strings.Contains(lu, "fueltab=") || strings.Contains(lu, "maintenancetab=") || strings.HasSuffix(lu, "/dashboard") || strings.Contains(lu, "/fuel?") {
			return "portal page, not a CSV export"
		}
		if strings.HasSuffix(lu, ".csv") || strings.Contains(lu, "export") {
			return "looks like an export URL"
		}
		return "URL set"
	}
	pemKind := func(v, begin string) string {
		if strings.Contains(v, begin) {
			return "PEM"
		}
		return "not PEM armor"
	}
	return []string{
		line("OILCHANGE_DB", c.SQLitePath, ""),
		line("DATABASE_URL", c.DatabaseURL, "Neon backup (unpooled pgx); DSN stays sqlite when OILCHANGE_DB is set"),
		line("SUPABASE_URL", c.SupabaseURL, "ZacharyTFerguson's Project (fleet_*); never XRAY"),
		line("SUPABASE_GROK_BUILD_KEY", c.SupabaseAnonKey, "publishable/anon SELECT on fleet_cars (alias SUPABASE_ANON_KEY)"),
		line("SUPABASE_SERVICE_ROLE", c.ServiceRole, "server-side PostgREST sync only"),
		line("SUPABASE_SYNC_SECRET", c.SyncSecret, "fleet-sync edge token when service role unset"),
		line("EFLEETS_USERNAME", c.EFleetsUser, "(EFLEETS_USER / EFleetsUsername; oilchange.env or Cloud Agent secrets — never chat)"),
		line("EFLEETS_PASSWORD", c.EFleetsPass, "(EFLEETS_PASS / EFleetsPassword; never printed)"),
		line("EFLEETS_CUST_NUM", c.EFleetsCust, "(EFLEETS_CUSTOMER / EFleetsCustNum)"),
		line("EFLEETS_DETAILS_URL", c.EFleetsDetails, page(c.EFleetsDetails)),
		line("EFLEETS_MAINT_URL", c.EFleetsMaint, page(c.EFleetsMaint)),
		line("EFLEETS_FLEETSUMMARY_URL", c.EFleetsFleet, page(c.EFleetsFleet)),
		line("ONESTEP_API_KEY", c.OneStepToken, "(ONE_STEP_FULL_API_KEY / ONESTEP_API_TOKEN / ONESTEP_API_KEY)"),
		line("ONESTEP_API_PRIVATEKEY", c.OneStepPrivateKey, pemKind(c.OneStepPrivateKey, "BEGIN PRIVATE KEY")),
		line("ONESTEP_API_PUBLIC_KEY", c.OneStepPublicKey, pemKind(c.OneStepPublicKey, "BEGIN PUBLIC KEY")),
		line("ONESTEP_BASE_URL", c.OneStepBase, ""),
	}
}
