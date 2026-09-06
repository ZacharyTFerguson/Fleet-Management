package desk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"oilchange/internal/app"
	"oilchange/internal/deskauth"
	"oilchange/internal/store"
)

// DeskAPI is login, status, secrets, and the gas-station marker pipeline.
type DeskAPI struct {
	Auth     *deskauth.Manager
	Login    func(ctx context.Context, user, pass string) (username string, bootstrap bool, err error)
	Session  func(ctx context.Context) (users int, bootstrap bool, err error)
	Status   func(ctx context.Context) (app.StatusReport, error)
	Secrets  func(ctx context.Context) ([]app.SecretView, error)
	PutSecret func(ctx context.Context, key, value string) (app.SecretView, error)
	Markers  func(ctx context.Context) (app.MarkerList, error)
	Pull     func(ctx context.Context) (app.MarkerList, error)
	Geocode  func(ctx context.Context, id string) (*store.MarkerJob, error)
	Review   func(ctx context.Context, id, action, notes string) (*store.MarkerJob, error)
	DryRun   func(ctx context.Context, id string) (map[string]any, error)
	Send     func(ctx context.Context, id, phrase, token string) (*store.MarkerJob, error)
	Zone     func(ctx context.Context, id, phrase, token string) (*store.MarkerJob, error)
	Confirm  func(ctx context.Context, id string) (string, error)
}

func mountDeskAPI(mux *http.ServeMux, api *DeskAPI) {
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		serveLogin(w, r, api)
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		serveLogout(w, r, api)
	})
	mux.HandleFunc("/api/auth/session", func(w http.ResponseWriter, r *http.Request) {
		serveSession(w, r, api)
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireDesk(w, r, api) {
			return
		}
		serveStatus(w, r, api)
	})
	mux.HandleFunc("/api/secrets", func(w http.ResponseWriter, r *http.Request) {
		if !requireDesk(w, r, api) {
			return
		}
		serveSecrets(w, r, api)
	})
	mux.HandleFunc("/api/markers", func(w http.ResponseWriter, r *http.Request) {
		if !requireDesk(w, r, api) {
			return
		}
		serveMarkers(w, r, api)
	})
	mux.HandleFunc("/api/markers/", func(w http.ResponseWriter, r *http.Request) {
		if !requireDesk(w, r, api) {
			return
		}
		serveMarkerAction(w, r, api)
	})
}

func requireDesk(w http.ResponseWriter, r *http.Request, api *DeskAPI) bool {
	jsonHeaders(w)
	if api == nil || api.Auth == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "desk login API off — set OILCHANGE_DB"})
		return false
	}
	if _, ok := api.Auth.UserFromRequest(r); ok {
		return true
	}
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "login required", "login": "/login/"})
	return false
}

func jsonHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}

func serveLogin(w http.ResponseWriter, r *http.Request, api *DeskAPI) {
	jsonHeaders(w)
	if api == nil || api.Login == nil || api.Auth == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "login unavailable"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err := json.Unmarshal(b, &body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}
	user, bootstrap, err := api.Login(r.Context(), strings.TrimSpace(body.Username), body.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	api.Auth.SetCookie(w, r, user)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "username": user, "bootstrap": bootstrap})
}

func serveLogout(w http.ResponseWriter, r *http.Request, api *DeskAPI) {
	jsonHeaders(w)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api != nil && api.Auth != nil {
		api.Auth.ClearCookie(w)
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func serveSession(w http.ResponseWriter, r *http.Request, api *DeskAPI) {
	jsonHeaders(w)
	out := map[string]any{"ok": false}
	if api != nil && api.Session != nil {
		n, boot, err := api.Session(r.Context())
		if err == nil {
			out["users"] = n
			out["bootstrap"] = boot
		}
	}
	if api != nil && api.Auth != nil {
		if u, ok := api.Auth.UserFromRequest(r); ok {
			out["ok"] = true
			out["username"] = u
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func serveStatus(w http.ResponseWriter, r *http.Request, api *DeskAPI) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.Status == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "status unavailable"})
		return
	}
	rep, err := api.Status(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if u, ok := api.Auth.UserFromRequest(r); ok {
		rep.User = u
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rep)
}

func serveSecrets(w http.ResponseWriter, r *http.Request, api *DeskAPI) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if api.Secrets == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "secrets unavailable"})
			return
		}
		list, err := api.Secrets(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"fields": list, "note": "Values are never returned in full after save."})
	case http.MethodPut, http.MethodPost:
		if api.PutSecret == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err := json.Unmarshal(b, &body); err != nil || body.Key == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "key required"})
			return
		}
		view, err := api.PutSecret(r.Context(), body.Key, body.Value)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func serveMarkers(w http.ResponseWriter, r *http.Request, api *DeskAPI) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if api.Markers == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		list, err := api.Markers(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(list)
	case http.MethodPost:
		if api.Pull == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		list, err := api.Pull(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(list)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func serveMarkerAction(w http.ResponseWriter, r *http.Request, api *DeskAPI) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/markers/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	id, action := parts[0], parts[1]
	var body struct {
		Action        string `json:"action"`
		Notes         string `json:"notes"`
		Confirm       string `json:"confirm"`
		ConfirmToken  string `json:"confirm_token"`
	}
	b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	_ = json.Unmarshal(b, &body)
	switch action {
	case "geocode":
		j, err := api.Geocode(r.Context(), id)
		writeJob(w, j, err)
	case "review":
		act := body.Action
		if act == "" {
			act = "approve"
		}
		j, err := api.Review(r.Context(), id, act, body.Notes)
		if err != nil {
			writeJob(w, nil, err)
			return
		}
		tok, _ := api.Confirm(r.Context(), id)
		_ = json.NewEncoder(w).Encode(map[string]any{"job": j, "confirm_token": tok})
	case "dry-run":
		out, err := api.DryRun(r.Context(), id)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	case "send":
		j, err := api.Send(r.Context(), id, body.Confirm, body.ConfirmToken)
		writeJob(w, j, err)
	case "zone":
		j, err := api.Zone(r.Context(), id, body.Confirm, body.ConfirmToken)
		writeJob(w, j, err)
	default:
		http.NotFound(w, r)
	}
}

func writeJob(w http.ResponseWriter, j *store.MarkerJob, err error) {
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(j)
}
