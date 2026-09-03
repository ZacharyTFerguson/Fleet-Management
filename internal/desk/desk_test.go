package desk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/desk"
	"oilchange/internal/syncsupabase"
)

func TestCarsAPIFromMirror(t *testing.T) {
	dir := t.TempDir()
	mirror := filepath.Join(dir, "cars.json")
	miles := 12000
	snap := syncsupabase.Snapshot{
		SyncedAt: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
		Source:   "mock-mirror",
		Cars: []syncsupabase.CarRow{{
			PDIID:            "PDI-0001",
			EFleetsID:        "ABC",
			Nickname:         "CT1",
			LastReadingMiles: &miles,
			IntervalMiles:    5000,
		}},
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mirror, b, 0o644); err != nil {
		t.Fatal(err)
	}

	// Empty web dir with a tiny index so Handler still mounts static.
	web := filepath.Join(dir, "out")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	h, err := desk.Handler(desk.Options{WebDir: web, MirrorPath: mirror})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cars", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var got syncsupabase.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "mock-mirror" || len(got.Cars) != 1 || got.Cars[0].Nickname != "CT1" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("index status %d", rec2.Code)
	}
}

func TestCarsAPIMissingMirror(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "out")
	_ = os.MkdirAll(web, 0o755)
	_ = os.WriteFile(filepath.Join(web, "index.html"), []byte("x"), 0o644)
	h, err := desk.Handler(desk.Options{
		WebDir:     web,
		MirrorPath: filepath.Join(dir, "missing.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/cars", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got syncsupabase.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "mock-seed" || len(got.Cars) != 0 {
		t.Fatalf("got %+v", got)
	}
}
