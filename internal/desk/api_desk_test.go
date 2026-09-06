package desk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oilchange/internal/app"
	"oilchange/internal/desk"
	"oilchange/internal/deskauth"
	"oilchange/internal/vault"
)

func deskHandler(t *testing.T, api *desk.DeskAPI) http.Handler {
	t.Helper()
	dir := t.TempDir()
	web := filepath.Join(dir, "out")
	_ = os.MkdirAll(web, 0o755)
	_ = os.WriteFile(filepath.Join(web, "index.html"), []byte("ok"), 0o644)
	h, err := desk.Handler(desk.Options{WebDir: web, MirrorPath: filepath.Join(dir, "cars.json"), Desk: api})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSecretsAndStatusRequireLogin(t *testing.T) {
	m := deskauth.NewManager([]byte("test-key"))
	h := deskHandler(t, &desk.DeskAPI{
		Auth: m,
		Secrets: func(ctx context.Context) ([]app.SecretView, error) {
			return []app.SecretView{{Record: vault.Record{Key: "onestep_api_key", Set: true, Mask: "••••abcd", Bytes: 12}}}, nil
		},
		Status: func(ctx context.Context) (app.StatusReport, error) {
			return app.StatusReport{SQLite: app.EndpointStatus{OK: true, Detail: "cars=1"}}, nil
		},
	})
	for _, path := range []string{"/api/secrets", "/api/status", "/api/markers", "/api/boxscore"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s status %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "super-secret") {
			t.Fatal("secret leaked")
		}
	}
	rec := httptest.NewRecorder()
	m.SetCookie(rec, httptest.NewRequest(http.MethodGet, "/", nil), "zach")
	req := httptest.NewRequest(http.MethodGet, "/api/secrets", nil)
	req.Header.Set("Cookie", rec.Result().Header.Get("Set-Cookie"))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("authed %d %s", rec2.Code, rec2.Body.String())
	}
	if strings.Contains(rec2.Body.String(), "super-secret") {
		t.Fatal("echo")
	}
	var wrap map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
}
