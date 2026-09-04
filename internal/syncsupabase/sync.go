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
	"net/http"
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
)

// Config is the remote + optional local mirror. Empty URL/key means mock-only.
type Config struct {
	URL         string
	ServiceRole string // PostgREST upsert when set (server-side only)
	SyncSecret  string // x-fleet-sync-token for /functions/v1/fleet-sync when service role unset
	AnonKey     string // SUPABASE_GROK_BUILD_KEY / publishable; SELECT only
	MirrorPath  string // local JSON the web UI can read without secrets
	NoRemote    bool   // skip PostgREST upsert; still write MirrorPath
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
		if c.HoldReason != nil && strings.TrimSpace(*c.HoldReason) != "" {
			row.LastReadingMiles = nil
			row.LastReadingAt = nil
			row.LastReadingSource = nil
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

func refuseXRAY(url string) error {
	lu := strings.ToLower(url)
	if strings.Contains(lu, "xray") || strings.Contains(lu, "chjqcznyxvtjbamttqdj") {
		return fmt.Errorf("refusing XRAY Supabase project for fleet oil data")
	}
	return nil
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
	if !cfg.NoRemote && cfg.URL != "" && (cfg.ServiceRole != "" || cfg.SyncSecret != "") {
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
	return snap, errors.Join(syncErr, mirrorErr)
}

// writeMirrorMu serializes the final replace of cars.json. CreateTemp names
// stay unique per call, but Windows MoveFileEx cannot replace the same dest
// from many goroutines at once (Access is denied). Run() already holds runMu;
// this lock covers writeMirror callers that skip Run.
var writeMirrorMu sync.Mutex

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
	writeMirrorMu.Lock()
	defer writeMirrorMu.Unlock()
	return replaceFile(tmp, path)
}

// replaceFile atomically puts tmp at dest. On Windows, a leftover dest that
// MOVEFILE_REPLACE_EXISTING cannot take is removed and the rename retried.
func replaceFile(tmp, dest string) error {
	err := os.Rename(tmp, dest)
	if err == nil {
		return nil
	}
	if removeErr := os.Remove(dest); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return errors.Join(err, removeErr)
	}
	return os.Rename(tmp, dest)
}

func pushSupabase(ctx context.Context, cfg Config, snap *Snapshot) error {
	base := strings.TrimRight(cfg.URL, "/")
	client := &http.Client{Timeout: 60 * time.Second}

	// Prefer service-role PostgREST when available; else fleet-sync edge function.
	if cfg.ServiceRole != "" {
		if err := upsert(ctx, client, base, cfg.ServiceRole, CarsTable, snap.Cars, "pdi_id"); err != nil {
			return fmt.Errorf("%s: %w", CarsTable, err)
		}
		if len(snap.Cards) > 0 {
			if err := upsert(ctx, client, base, cfg.ServiceRole, CardsTable, snap.Cards, "id"); err != nil {
				return fmt.Errorf("%s: %w", CardsTable, err)
			}
		}
		return nil
	}
	return pushEdgeSync(ctx, client, base, cfg.SyncSecret, snap)
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
	url := base + "/functions/v1/fleet-sync"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
		return fmt.Errorf("fleet-sync HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
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
	url := fmt.Sprintf("%s/rest/v1/%s?on_conflict=%s", base, table, onConflict)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
