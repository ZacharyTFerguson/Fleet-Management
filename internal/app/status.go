package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"oilchange/internal/onestep"
	"oilchange/internal/store"
)

// StatusReport is connection health. It never includes DSNs, keys, PEMs, or passwords.
type StatusReport struct {
	SQLite   EndpointStatus `json:"sqlite"`
	Neon     EndpointStatus `json:"neon"`
	Supabase EndpointStatus `json:"supabase"`
	OneStep  EndpointStatus `json:"onestep"`
	Owners   DataOwners     `json:"owners"`
	User     string         `json:"user,omitempty"`
	At       string         `json:"at"`
}

type EndpointStatus struct {
	OK      bool   `json:"ok"`
	Role    string `json:"role"`
	Detail  string `json:"detail,omitempty"`
	Project string `json:"project,omitempty"`
	Error   string `json:"error,omitempty"`
	Write   string `json:"write,omitempty"`
}

type DataOwners struct {
	WorkingStore string `json:"working_store"`
	DeskPublish  string `json:"desk_publish"`
	Backup       string `json:"backup"`
	Note         string `json:"note"`
}

func (a *App) Status(ctx context.Context) (StatusReport, error) {
	rep := StatusReport{
		At: time.Now().UTC().Format(time.RFC3339),
		Owners: DataOwners{
			WorkingStore: "SQLite (OILCHANGE_DB) — ingest, compute, serve, vault",
			DeskPublish:  "Supabase ZacharyTFerguson's Project (hdtwfdjdvdzdxfdriyzn) fleet_cars + later full copy",
			Backup:       "Neon Fleet_Management_Neon / Fleet_Manage_Oil (unpooled DATABASE_URL)",
			Note:         "Never XRAY (chjqcznyxvtjbamttqdj). Vault secrets stay on sqlite only.",
		},
	}
	rep.SQLite = a.pingSQLite(ctx)
	rep.Neon = a.pingNeon(ctx)
	rep.Supabase = a.pingSupabase(ctx)
	rep.OneStep = a.pingOneStep(ctx)
	if a.Store != nil {
		_ = a.Store.InsertHeartbeat(ctx, "sqlite", rep.SQLite.OK, rep.SQLite.Detail)
		_ = a.Store.InsertHeartbeat(ctx, "neon", rep.Neon.OK, rep.Neon.Detail)
		_ = a.Store.InsertHeartbeat(ctx, "supabase", rep.Supabase.OK, rep.Supabase.Detail)
	}
	return rep, nil
}

func (a *App) pingSQLite(ctx context.Context) EndpointStatus {
	st := EndpointStatus{Role: "working store"}
	if a.Store == nil {
		st.Error = "no sqlite store (set OILCHANGE_DB)"
		return st
	}
	cars, err := a.Store.CountTable(ctx, "cars")
	if err != nil {
		st.Error = err.Error()
		return st
	}
	stations, _ := a.Store.CountTable(ctx, "gas_stations")
	placeN, _ := a.Store.CountTable(ctx, "places")
	ledgerN, _ := a.Store.CountTable(ctx, "mileage_ledger")
	st.OK = true
	st.Detail = fmt.Sprintf("cars=%d gas_stations=%d places=%d ledger=%d", cars, stations, placeN, ledgerN)
	st.Write = "ok (local)"
	return st
}

func (a *App) pingNeon(ctx context.Context) EndpointStatus {
	st := EndpointStatus{Role: "backup", Project: "Fleet_Management_Neon / Fleet_Manage_Oil"}
	dsn := strings.TrimSpace(a.Cfg.DatabaseURL)
	if dsn == "" {
		dsn = a.SecretPlain(ctx, "neon_database_url")
	}
	if dsn == "" {
		st.Error = "DATABASE_URL missing (Secrets page or oilchange.env, unpooled)"
		return st
	}
	if err := validateNeonBackupURL(dsn); err != nil {
		st.Error = err.Error()
		return st
	}
	dest, err := store.Open("pgx", dsn)
	if err != nil {
		st.Error = "open failed (credentials not printed)"
		return st
	}
	defer dest.Close()
	n, err := dest.CountTable(ctx, "cars")
	if err != nil {
		st.Error = "query failed"
		return st
	}
	st.OK = true
	st.Detail = fmt.Sprintf("cars=%d", n)
	st.Write = "backup-neon path"
	return st
}

func (a *App) pingSupabase(ctx context.Context) EndpointStatus {
	st := EndpointStatus{Role: "Oil Desk publish", Project: "hdtwfdjdvdzdxfdriyzn"}
	base := strings.TrimSpace(a.Cfg.SupabaseURL)
	if base == "" {
		base = a.SecretPlain(ctx, "supabase_url")
	}
	key := strings.TrimSpace(a.Cfg.SupabaseAnonKey)
	if key == "" {
		key = a.SecretPlain(ctx, "supabase_anon_key")
	}
	if base == "" || key == "" {
		st.Error = "SUPABASE_URL or publishable key missing"
		return st
	}
	if strings.Contains(strings.ToLower(base), "chjqcznyxvtjbamttqdj") {
		st.Error = "refusing XRAY project"
		return st
	}
	u := strings.TrimRight(base, "/") + "/rest/v1/fleet_cars?select=efleets_id&limit=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		st.Error = "request build failed"
		return st
	}
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		st.Error = "unreachable"
		return st
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if res.StatusCode >= 300 {
		st.Error = fmt.Sprintf("HTTP %d", res.StatusCode)
		return st
	}
	var rows []map[string]any
	_ = json.Unmarshal(b, &rows)
	st.OK = true
	st.Detail = fmt.Sprintf("fleet_cars readable (sample_rows=%d)", len(rows))
	write := "not configured"
	if a.Cfg.ServiceRole != "" || a.Cfg.SyncSecret != "" || a.SecretPlain(ctx, "supabase_service_role") != "" || a.SecretPlain(ctx, "supabase_sync_secret") != "" {
		write = "sync / service role present (server-side)"
	}
	st.Write = write
	return st
}

func (a *App) pingOneStep(ctx context.Context) EndpointStatus {
	st := EndpointStatus{Role: "GPS / places"}
	c := a.oneStepClient()
	if c == nil || c.Token == "" {
		st.Error = "OneStep API key not set"
		return st
	}
	st.Detail = "auth=" + c.AuthMode()
	q := url.Values{}
	q.Set("latest_point", "true")
	q.Set("limit", "1")
	_, err := c.GetPublic(ctx, "/v3/api/public/device", q)
	if err != nil {
		st.Error = "device ping failed (redacted)"
		return st
	}
	st.OK = true
	if onestep.WriteProven() {
		st.Write = "places send is confirm-gated (ONESTEP_WRITE_PROVEN)"
	} else {
		st.Write = "places create not proven — portal-first"
	}
	return st
}
