package desk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestCardsAPIFromMirror(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "out")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cards := filepath.Join(dir, "cards.json")
	if err := os.WriteFile(cards, []byte(`{"source":"card-swipes","stats":{"cards":2,"unknown":1},"unknown":[{"kind":"suspect","card_id":"CARD-MIX-99","best_car":"27VA15"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := desk.Handler(desk.Options{WebDir: web, MirrorPath: filepath.Join(dir, "cars.json"), CardsPath: cards})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/cards", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CARD-MIX-99") {
		t.Fatalf("body %s", rec.Body.String())
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

func TestVINFromFileGETMissing(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "out")
	_ = os.MkdirAll(web, 0o755)
	_ = os.WriteFile(filepath.Join(web, "index.html"), []byte("x"), 0o644)
	missing := filepath.Join(dir, "device-information.json")
	h, err := desk.Handler(desk.Options{
		WebDir:                web,
		MirrorPath:            filepath.Join(dir, "cars.json"),
		DeviceInformationPath: missing,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/devices/vin-from-file", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var got desk.VINFromFileResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Exists || got.Path != missing {
		t.Fatalf("%+v", got)
	}
}

func TestVINFromFilePOSTAppliesSavedJSON(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "out")
	_ = os.MkdirAll(web, 0o755)
	_ = os.WriteFile(filepath.Join(web, "index.html"), []byte("x"), 0o644)
	info := filepath.Join(dir, "device-information.json")
	var posted int
	h, err := desk.Handler(desk.Options{
		WebDir:                web,
		MirrorPath:            filepath.Join(dir, "cars.json"),
		DeviceInformationPath: info,
		ApplyDeviceInformation: func(ctx context.Context) (desk.VINFromFileResult, error) {
			posted++
			if ctx == nil {
				t.Fatal("nil context")
			}
			return desk.VINFromFileResult{
				Path:     info,
				Exists:   true,
				Parsed:   2,
				Upserted: 2,
				Linked:   1,
				Links:    []desk.VINFromFileLink{{FactoryID: "FACTVIN", DeviceID: "DEVVIN", VIN: "1HGCM82633A004352", EFleetsID: "27TESTA"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/devices/vin-from-file", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if posted != 1 {
		t.Fatalf("posted %d", posted)
	}
	if !strings.Contains(rec.Body.String(), "FACTVIN") || !strings.Contains(rec.Body.String(), `"asked": 0`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestVINFromFilePOSTMissingFile(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "out")
	_ = os.MkdirAll(web, 0o755)
	_ = os.WriteFile(filepath.Join(web, "index.html"), []byte("x"), 0o644)
	info := filepath.Join(dir, "missing-device-information.json")
	h, err := desk.Handler(desk.Options{
		WebDir:                web,
		MirrorPath:            filepath.Join(dir, "cars.json"),
		DeviceInformationPath: info,
		ApplyDeviceInformation: func(ctx context.Context) (desk.VINFromFileResult, error) {
			return desk.VINFromFileResult{Path: info}, fmt.Errorf("saved OneStep device information not found at %s — drop the Device Information JSON there (OneStep cooling down; do not GET /device)", info)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/devices/vin-from-file", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestVINFromFilePOSTWithoutStore(t *testing.T) {
	dir := t.TempDir()
	web := filepath.Join(dir, "out")
	_ = os.MkdirAll(web, 0o755)
	_ = os.WriteFile(filepath.Join(web, "index.html"), []byte("x"), 0o644)
	h, err := desk.Handler(desk.Options{WebDir: web, MirrorPath: filepath.Join(dir, "cars.json")})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/devices/vin-from-file", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
