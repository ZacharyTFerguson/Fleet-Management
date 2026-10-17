package syncsupabase

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestWriteMirrorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cars.json"
	cars := []CarRow{{
		PDIID:     "PDI-0001",
		EFleetsID: "27TESTA",
		Nickname:  "Alpha",
		Plate:     "ABC123",
		Region:    "VA",
	}}
	cfg := Config{MirrorPath: path}
	out, err := Run(t.Context(), cfg, cars, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "mock-mirror" {
		t.Fatalf("source %s", out.Source)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Cars) != 1 || got.Cars[0].EFleetsID != "27TESTA" {
		t.Fatalf("mirror %#v", got.Cars)
	}
	if got.SyncedAt.IsZero() {
		t.Fatal("synced_at missing")
	}
}

func TestConcurrentRunSerializesMirror(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cars.json"
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cars := []CarRow{{
				PDIID:     "PDI-0001",
				EFleetsID: "27TESTA",
				Nickname:  "Alpha",
			}}
			_, err := Run(t.Context(), Config{MirrorPath: path}, cars, nil, nil)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("torn mirror: %v", err)
	}
	if len(got.Cars) != 1 {
		t.Fatalf("mirror cars %d", len(got.Cars))
	}
}

func TestConcurrentWriteMirrorUsesIndependentTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cars.json"
	const n = 32
	start := make(chan struct{})
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- writeMirror(path, &Snapshot{
				Source: "mock-mirror",
				Cars:   []CarRow{{PDIID: "PDI-0001", EFleetsID: "CAR"}},
			})
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("torn mirror: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "cars.json" {
		t.Fatalf("temporary files leaked: %+v", entries)
	}
}

func TestRemoteFailureStillRefreshesMirror(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "remote unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	path := t.TempDir() + "/cars.json"
	out, err := Run(t.Context(), Config{
		URL:         srv.URL,
		ServiceRole: "test-key",
		MirrorPath:  path,
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "CAR1"}}, nil, nil)
	if err == nil {
		t.Fatal("expected remote error")
	}
	if out == nil || out.Source != "mock-mirror" {
		t.Fatalf("fallback snapshot: %+v", out)
	}
	b, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Cars) != 1 || got.Cars[0].EFleetsID != "CAR1" {
		t.Fatalf("fallback mirror: %+v", got)
	}
}

func TestRefuseXRAY(t *testing.T) {
	if err := refuseXRAY("https://chjqcznyxvtjbamttqdj.supabase.co"); err == nil {
		t.Fatal("expected XRAY refuse")
	}
	if err := refuseXRAY("https://hdtwfdjdvdzdxfdriyzn.supabase.co"); err != nil {
		t.Fatal(err)
	}
	if CarsTable != "fleet_cars" || CardsTable != "fleet_cards" {
		t.Fatalf("unexpected tables %s %s", CarsTable, CardsTable)
	}
}

func TestCarsForRemoteUpsertOmitsNullHoldWhenNeverComputed(t *testing.T) {
	rows, err := carsForRemoteUpsert([]CarRow{{
		PDIID:     "PDI-0001",
		EFleetsID: "27VA15",
		Nickname:  "VA15",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	if _, ok := rows[0]["hold_reason"]; ok {
		t.Fatalf("roster-only push must omit hold_reason, got %#v", rows[0])
	}
	if rows[0]["efleets_id"] != "27VA15" || rows[0]["pdi_id"] != "PDI-0001" {
		t.Fatalf("identity must stay: %#v", rows[0])
	}
}

func TestCarsForRemoteUpsertClearsHoldAfterSuccessfulCompute(t *testing.T) {
	read := 180312
	rows, err := carsForRemoteUpsert([]CarRow{{
		PDIID:            "PDI-0001",
		EFleetsID:        "26LSZW",
		LastReadingMiles: &read,
	}})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := rows[0]["hold_reason"]
	if !ok || v != nil {
		t.Fatalf("successful compute must send hold_reason null to clear remote HOLD, got %v ok=%v", v, ok)
	}
	if rows[0]["last_reading_miles"] != float64(read) {
		t.Fatalf("known last reading must be sent: %#v", rows[0]["last_reading_miles"])
	}
}

func TestCarsForRemoteUpsertKeepsHoldReason(t *testing.T) {
	hold := "NO_DRIVESTOP"
	oil := 179598
	rows, err := carsForRemoteUpsert([]CarRow{{
		PDIID:        "PDI-0001",
		EFleetsID:    "26LSZW",
		LastOilMiles: &oil,
		HoldReason:   &hold,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["hold_reason"] != hold {
		t.Fatalf("HOLD must be sent: %#v", rows[0]["hold_reason"])
	}
}

func TestPushOmitsNullHoldInPostgRESTBody(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	path := t.TempDir() + "/cars.json"
	_, err := Run(t.Context(), Config{
		URL:         srv.URL,
		ServiceRole: "test-key",
		MirrorPath:  path,
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "27VA15", Nickname: "VA15"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"hold_reason":null`) {
		t.Fatalf("PostgREST upsert must not send hold_reason null for never-computed sqlite: %s", body)
	}
	if !strings.Contains(string(body), `"efleets_id":"27VA15"`) {
		t.Fatalf("identity missing from upsert: %s", body)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"hold_reason": null`) && !strings.Contains(string(b), `"hold_reason":null`) {
		t.Fatalf("local cars.json mirror must still include hold_reason: %s", b)
	}
}

func TestPushSendsNullHoldAfterSuccessfulCompute(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	read := 180312
	_, err := Run(t.Context(), Config{
		URL:         srv.URL,
		ServiceRole: "test-key",
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "26LSZW", LastReadingMiles: &read}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"hold_reason":null`) {
		t.Fatalf("PostgREST upsert must send hold_reason null after compute cleared HOLD: %s", body)
	}
}
