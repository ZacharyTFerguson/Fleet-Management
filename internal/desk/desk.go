// Package desk serves the static Oil Desk UI and /api/cars for desktop
// (no Node/npm required). Never targets XRAY.
package desk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"oilchange/internal/syncsupabase"
	webui "oilchange/web"
)

// Options configures the local Oil Desk HTTP server.
type Options struct {
	Addr                   string // e.g. 127.0.0.1:4739
	WebDir                 string // optional on-disk export; empty → embedded web/out
	MirrorPath             string // cars.json written by oilchange sync
	CardsPath              string // cards.json from oilchange cards rebuild
	DeviceInformationPath  string // gitignored Device Information JSON (file apply; no live HTTP)
	ApplyDeviceInformation func(ctx context.Context) (VINFromFileResult, error)
	History                *HistoryAPI
}

// VINFromFileLink is one factory_id → Enterprise car from the saved JSON.
type VINFromFileLink struct {
	FactoryID string `json:"factory_id"`
	DeviceID  string `json:"device_id"`
	VIN       string `json:"vin"`
	EFleetsID string `json:"efleets_id"`
}

// VINFromFileResult is GET (exists) or POST (apply) for /api/devices/vin-from-file.
type VINFromFileResult struct {
	Path               string            `json:"path"`
	Exists             bool              `json:"exists"`
	Parsed             int               `json:"parsed,omitempty"`
	Upserted           int               `json:"upserted,omitempty"`
	Asked              int               `json:"asked"`
	Linked             int               `json:"linked"`
	Already            int               `json:"already"`
	NoVIN              int               `json:"no_vin"`
	NoRoster           int               `json:"no_roster"`
	SkippedExistingMap int               `json:"skipped_existing_map"`
	Links              []VINFromFileLink `json:"links,omitempty"`
	Error              string            `json:"error,omitempty"`
}

// Handler returns the mux that serves static UI + /api/cars.
func Handler(opts Options) (http.Handler, error) {
	mirror := opts.MirrorPath
	if mirror == "" {
		mirror = filepath.Join("web", "data", "cars.json")
	}
	cardsPath := opts.CardsPath
	if cardsPath == "" {
		cardsPath = filepath.Join(filepath.Dir(mirror), "cards.json")
	}
	static, err := staticFS(opts.WebDir)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cars", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveCars(w, r, mirror)
	})
	mux.HandleFunc("/api/cards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveJSONFile(w, r, cardsPath)
	})
	mux.HandleFunc("/api/devices/vin-from-file", func(w http.ResponseWriter, r *http.Request) {
		serveVINFromFile(w, r, opts)
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		serveHistory(w, r, opts.History)
	})
	mux.HandleFunc("/api/history/assign", func(w http.ResponseWriter, r *http.Request) {
		serveHistoryAssign(w, r, opts.History)
	})
	mux.HandleFunc("/api/history/events", func(w http.ResponseWriter, r *http.Request) {
		serveHistoryEvents(w, r, opts.History)
	})
	mux.HandleFunc("/api/devices/evidence", func(w http.ResponseWriter, r *http.Request) {
		serveDeviceEvidence(w, r, opts.History)
	})
	mux.HandleFunc("/api/devices/probe", func(w http.ResponseWriter, r *http.Request) {
		serveDeviceProbe(w, r, opts.History)
	})
	mux.Handle("/", spaFileServer(static))
	return mux, nil
}

// ListenAndServe starts the desk server until the listener fails.
func ListenAndServe(opts Options) error {
	h, err := Handler(opts)
	if err != nil {
		return err
	}
	addr := opts.Addr
	if addr == "" {
		addr = "127.0.0.1:4739"
	}
	fmt.Fprintf(os.Stderr, "oilchange serve: Oil Desk at http://%s (mirror %s cards %s)\n", addr, opts.MirrorPath, opts.CardsPath)
	return http.ListenAndServe(addr, h)
}

func staticFS(webDir string) (fs.FS, error) {
	if webDir != "" {
		abs, err := filepath.Abs(webDir)
		if err != nil {
			return nil, err
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("web dir %s: %w", abs, err)
		}
		return os.DirFS(abs), nil
	}
	sub, err := fs.Sub(webui.Out, "out")
	if err != nil {
		return nil, fmt.Errorf("embedded web/out: %w (rebuild with cd web && npm run build:static)", err)
	}
	return sub, nil
}

func serveVINFromFile(w http.ResponseWriter, r *http.Request, opts Options) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	path := strings.TrimSpace(opts.DeviceInformationPath)
	if path == "" {
		path = filepath.Join("data", "runtime", "device-information.json")
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_, err := os.Stat(path)
		out := VINFromFileResult{Path: path, Exists: err == nil}
		if r.Method == http.MethodHead {
			if out.Exists {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodPost:
		if opts.ApplyDeviceInformation == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "oilchange serve has no sqlite store — set OILCHANGE_DB to apply saved Device Information",
			})
			return
		}
		res, err := opts.ApplyDeviceInformation(r.Context())
		if err != nil {
			code := http.StatusInternalServerError
			if os.IsNotExist(err) || strings.Contains(err.Error(), "not found") {
				code = http.StatusNotFound
			}
			w.WriteHeader(code)
			msg := err.Error()
			if res.Error != "" {
				msg = res.Error
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "path": path})
			return
		}
		res.Path = path
		res.Exists = true
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func serveJSONFile(w http.ResponseWriter, r *http.Request, path string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write([]byte(`{"synced_at":"` + time.Now().UTC().Format(time.RFC3339) + `","source":"mock-seed","stats":{},"unknown":[],"stations":[],"cars_without_card":[]}`))
			}
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(b)
}

func serveCars(w http.ResponseWriter, r *http.Request, mirror string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	snap, err := readMirror(mirror)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			snap = &syncsupabase.Snapshot{
				SyncedAt: time.Now().UTC(),
				Source:   "mock-seed",
				Cars:     []syncsupabase.CarRow{},
			}
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(snap)
}

func readMirror(path string) (*syncsupabase.Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var snap syncsupabase.Snapshot
	dec := json.NewDecoder(f)
	if err := dec.Decode(&snap); err != nil {
		return nil, err
	}
	if snap.Source == "" {
		snap.Source = "mock-mirror"
	}
	if snap.Cars == nil {
		snap.Cars = []syncsupabase.CarRow{}
	}
	return &snap, nil
}

// spaFileServer serves the static export. Next trailingSlash pages live at
// path/index.html; bare .html files are also tried for robustness.
func spaFileServer(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		upath := path.Clean(r.URL.Path)
		if upath == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Try exact path, then as directory index, then .html
		candidates := []string{
			strings.TrimPrefix(upath, "/"),
			strings.TrimPrefix(path.Join(upath, "index.html"), "/"),
			strings.TrimPrefix(upath, "/") + ".html",
		}
		for _, c := range candidates {
			if c == "" || strings.HasSuffix(c, "/") {
				continue
			}
			if f, err := root.Open(c); err == nil {
				_ = f.Close()
				if strings.HasSuffix(c, "index.html") && !strings.HasSuffix(r.URL.Path, "/") && !strings.HasSuffix(r.URL.Path, "index.html") {
					// Keep trailing-slash URLs consistent with Next export.
					r2 := r.Clone(r.Context())
					r2.URL.Path = strings.TrimSuffix(upath, "/") + "/"
					fileServer.ServeHTTP(w, r2)
					return
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Fall through to FileServer (may 404) — do not rewrite to index for missing assets.
		fileServer.ServeHTTP(w, r)
	})
}

// WriteHealth is a tiny helper for tests.
func WriteHealth(w io.Writer) {
	_, _ = io.WriteString(w, "ok\n")
}
