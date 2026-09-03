// Package desk serves the static Oil Desk UI and /api/cars for desktop
// (no Node/npm required). Never targets XRAY.
package desk

import (
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
	Addr       string // e.g. 127.0.0.1:4739
	WebDir     string // optional on-disk export; empty → embedded web/out
	MirrorPath string // cars.json written by oilchange sync
}

// Handler returns the mux that serves static UI + /api/cars.
func Handler(opts Options) (http.Handler, error) {
	mirror := opts.MirrorPath
	if mirror == "" {
		mirror = filepath.Join("web", "data", "cars.json")
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
	fmt.Fprintf(os.Stderr, "oilchange serve: Oil Desk at http://%s (mirror %s)\n", addr, opts.MirrorPath)
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
