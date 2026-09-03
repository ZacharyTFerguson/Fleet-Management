// Package syncsupabase pushes local oilchange SQLite state to the fleet Supabase
// project (PostgREST or fleet-sync edge function) or a local JSON mirror when
// credentials are absent. Never targets XRAY. Service role / sync token are
// server-side only.
package syncsupabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"oilchange/internal/model"
)

// Shared-project table names (ZacharyTFerguson's Project). Prefixed so reception/Users stay intact.
const (
	CarsTable  = "fleet_cars"
	CardsTable = "fleet_cards"

	// fleetProjectRef is the only Supabase project oil/fleet data may target.
	fleetProjectRef = "hdtwfdjdvdzdxfdriyzn"
	xrayProjectRef  = "chjqcznyxvtjbamttqdj"
	fleetHost       = fleetProjectRef + ".supabase.co"
)

// Config is the remote + optional local mirror. Empty URL/key means mock-only.
type Config struct {
	URL         string
	ServiceRole string // PostgREST upsert when set (server-side only)
	SyncSecret  string // x-fleet-sync-token for /functions/v1/fleet-sync when service role unset
	MirrorPath  string // local JSON the web UI can read without secrets
}

// Snapshot is the durable fleet surface for the cars list UI.
type Snapshot struct {
	SyncedAt time.Time `json:"synced_at"`
	Source   string    `json:"source"` // "supabase" | "mock-mirror"
	Cars     []CarRow  `json:"cars"`
	Holds    []HoldRow `json:"holds,omitempty"`
	Cards    []CardRow `json:"cards,omitempty"`
}

// CarRow is the list-view shape (opaque pdi_id, never embeds region in the key).
type CarRow struct {
	PDIID             string  `json:"pdi_id"`
	EFleetsID         string  `json:"efleets_id"`
	Nickname          string  `json:"nickname"`
	Plate             string  `json:"plate"`
	VIN               string  `json:"vin"`
	Region            string  `json:"region"`
	LastOilMiles      *int    `json:"last_oil_miles"`
	LastOilDate       *string `json:"last_oil_date"`
	LastReadingMiles  *int    `json:"last_reading_miles"`
	LastReadingAt     *string `json:"last_reading_at"`
	LastReadingSource *string `json:"last_reading_source"`
	HoldReason        *string `json:"hold_reason"`
	IntervalMiles     int     `json:"interval_miles"`
}

// HoldRow is an open HOLD — UI must not treat last_reading as current odo beside it.
type HoldRow struct {
	EFleetsID string `json:"efleets_id"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
	At        string `json:"at"`
}

// CardRow is a fuel card summary for optional secondary panels.
type CardRow struct {
	ID                   string  `json:"id"`
	CompanyVehicleNumber string  `json:"company_vehicle_number"`
	LinkedCarEFleetsID   *string `json:"linked_car_efleets_id"`
	Notes                string  `json:"notes"`
}

// FromCars maps store cars into the sync/list shape.
func FromCars(cars []model.Car) []CarRow {
	out := make([]CarRow, 0, len(cars))
	for _, c := range cars {
		row := CarRow{
			PDIID:             c.PDIID,
			EFleetsID:         c.EFleetsID,
			Nickname:          c.Nickname,
			Plate:             c.Plate,
			VIN:               c.VIN,
			Region:            c.Region,
			LastOilMiles:      c.LastOilMiles,
			LastReadingMiles:  c.LastReadingMiles,
			LastReadingSource: c.LastReadingSource,
			HoldReason:        c.HoldReason,
			IntervalMiles:     c.IntervalMiles,
		}
		if c.LastOilDate != nil {
			s := c.LastOilDate.UTC().Format(time.RFC3339)
			row.LastOilDate = &s
		}
		if c.LastReadingAt != nil {
			s := c.LastReadingAt.UTC().Format(time.RFC3339)
			row.LastReadingAt = &s
		}
		out = append(out, row)
	}
	return out
}

// FromHolds maps open HOLD events.
func FromHolds(hs []model.HoldEvent) []HoldRow {
	out := make([]HoldRow, 0, len(hs))
	for _, h := range hs {
		out = append(out, HoldRow{
			EFleetsID: h.EFleetsID,
			Reason:    h.Reason,
			Detail:    h.Detail,
			At:        h.At.UTC().Format(time.RFC3339),
		})
	}
	return out
}

// FromCards maps fuel cards.
func FromCards(cs []model.Card) []CardRow {
	out := make([]CardRow, 0, len(cs))
	for _, c := range cs {
		out = append(out, CardRow{
			ID:                   c.ID,
			CompanyVehicleNumber: c.CompanyVehicleNumber,
			LinkedCarEFleetsID:   c.LinkedCarEFleetsID,
			Notes:                c.Notes,
		})
	}
	return out
}

// refuseXRAY is the remote-target gate: allowlist the fleet project (or loopback
// for tests) and deny XRAY / any other host, including case variants and
// credentialed URLs.
func refuseXRAY(raw string) error {
	return validateFleetTarget(raw)
}

func validateFleetTarget(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("refusing empty Supabase URL")
	}
	lowered := strings.ToLower(raw)
	if strings.Contains(lowered, "xray") || strings.Contains(lowered, xrayProjectRef) {
		return fmt.Errorf("refusing XRAY Supabase project for fleet oil data")
	}
	u, err := parseHTTPURL(raw)
	if err != nil {
		return fmt.Errorf("refusing invalid Supabase URL")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if strings.Contains(host, "xray") || strings.Contains(host, xrayProjectRef) {
		return fmt.Errorf("refusing XRAY Supabase project for fleet oil data")
	}
	if u.User != nil {
		return fmt.Errorf("refusing URL with embedded credentials")
	}
	if isLoopbackHost(host) {
		return nil
	}
	if host == fleetHost {
		if !strings.EqualFold(u.Scheme, "https") {
			return fmt.Errorf("refusing non-https fleet Supabase URL")
		}
		return nil
	}
	return fmt.Errorf("refusing non-fleet Supabase host for fleet oil data")
}

func parseHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		u, err = url.Parse("https://" + strings.TrimPrefix(raw, "//"))
		if err != nil {
			return nil, err
		}
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported scheme")
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("missing host")
	}
	return u, nil
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func fleetHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL != nil {
				if err := validateFleetTarget(req.URL.String()); err != nil {
					return err
				}
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
}

func redactSecrets(msg string, secrets ...string) string {
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if s == "" || len(s) < 6 {
			continue
		}
		msg = strings.ReplaceAll(msg, s, "[redacted]")
		msg = strings.ReplaceAll(msg, url.QueryEscape(s), "[redacted]")
		msg = strings.ReplaceAll(msg, url.PathEscape(s), "[redacted]")
	}
	return msg
}

func redactErr(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := redactSecrets(err.Error(), secrets...)
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

// runMu serializes Run. Concurrent oilchange sync --interval ticks and a
// one-shot CLI/web refresh share the same cars.json tmp+rename path and
// PostgREST upserts; without this lock the mirror can tear and fleet_cars
// batches can overlap. Never targets XRAY.
var runMu sync.Mutex

// Run upserts cars (and related rows) to Supabase when configured, and always
// refreshes the local mirror so the web UI can run without secrets.
func Run(ctx context.Context, cfg Config, cars []CarRow, holds []HoldRow, cards []CardRow) (*Snapshot, error) {
	runMu.Lock()
	defer runMu.Unlock()
	snap := &Snapshot{
		SyncedAt: time.Now().UTC(),
		Source:   "mock-mirror",
		Cars:     cars,
		Holds:    holds,
		Cards:    cards,
	}
	var syncErr error
	if cfg.URL != "" && (cfg.ServiceRole != "" || cfg.SyncSecret != "") {
		if err := refuseXRAY(cfg.URL); err != nil {
			syncErr = err
		} else if err := pushSupabase(ctx, cfg, snap); err != nil {
			syncErr = err
		} else {
			snap.Source = "supabase"
		}
	}
	var mirrorErr error
	if cfg.MirrorPath != "" {
		mirrorErr = writeMirror(cfg.MirrorPath, snap)
	}
	return snap, redactErr(errors.Join(syncErr, mirrorErr), cfg.ServiceRole, cfg.SyncSecret)
}

func writeMirror(path string, snap *Snapshot) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err := f.Chmod(0o644); err != nil {
		return errors.Join(err, f.Close())
	}
	if _, err := f.Write(b); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func pushSupabase(ctx context.Context, cfg Config, snap *Snapshot) error {
	base := strings.TrimRight(cfg.URL, "/")
	client := fleetHTTPClient()

	// Prefer service-role PostgREST when available; else fleet-sync edge function.
	var err error
	if cfg.ServiceRole != "" {
		if err = upsert(ctx, client, base, cfg.ServiceRole, CarsTable, snap.Cars, "pdi_id"); err != nil {
			err = fmt.Errorf("%s: %w", CarsTable, err)
		} else if len(snap.Cards) > 0 {
			if err = upsert(ctx, client, base, cfg.ServiceRole, CardsTable, snap.Cards, "id"); err != nil {
				err = fmt.Errorf("%s: %w", CardsTable, err)
			}
		}
	} else {
		err = pushEdgeSync(ctx, client, base, cfg.SyncSecret, snap)
	}
	return redactErr(err, cfg.ServiceRole, cfg.SyncSecret)
}

func pushEdgeSync(ctx context.Context, client *http.Client, base, secret string, snap *Snapshot) error {
	payload := map[string]any{"cars": snap.Cars}
	if len(snap.Cards) > 0 {
		payload["cards"] = snap.Cards
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := base + "/functions/v1/fleet-sync"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-fleet-sync-token", secret)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		body := redactSecrets(strings.TrimSpace(string(b)), secret)
		return fmt.Errorf("fleet-sync HTTP %d: %s", res.StatusCode, body)
	}
	return nil
}

func upsert(ctx context.Context, client *http.Client, base, key, table string, rows any, onConflict string) error {
	body, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	if string(body) == "null" || string(body) == "[]" {
		return nil
	}
	endpoint := fmt.Sprintf("%s/rest/v1/%s?on_conflict=%s", base, table, onConflict)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=merge-duplicates,return=minimal")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		body := redactSecrets(strings.TrimSpace(string(b)), key)
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, body)
	}
	return nil
}
