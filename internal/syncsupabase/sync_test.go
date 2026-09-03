package syncsupabase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestValidateFleetTargetAllowlistAndXRAY(t *testing.T) {
	t.Parallel()
	ok := []string{
		"https://hdtwfdjdvdzdxfdriyzn.supabase.co",
		"https://HDTWFDJDVDZDXFDRIYZN.supabase.co/rest/v1/fleet_cars",
		"https://hdtwfdjdvdzdxfdriyzn.supabase.co:443",
		"https://hdtwfdjdvdzdxfdriyzn.supabase.co.",
		"http://127.0.0.1:54321",
		"http://localhost:54321",
		"http://[::1]:54321",
	}
	for _, raw := range ok {
		if err := refuseXRAY(raw); err != nil {
			t.Fatalf("allow %q: %v", raw, err)
		}
	}

	blocked := []string{
		"https://chjqcznyxvtjbamttqdj.supabase.co",
		"https://CHJQCZNYXVTJBAMTTQDJ.SUPABASE.CO/rest/v1/fleet_cars",
		"http://chjqcznyxvtjbamttqdj.supabase.co",
		"https://xray.supabase.co",
		"https://example.supabase.co/?ref=chjqcznyxvtjbamttqdj",
		"https://otherprojectref12345.supabase.co",
		"http://hdtwfdjdvdzdxfdriyzn.supabase.co",
		"https://evil.example",
		"https://user:super-secret-service-role-token@hdtwfdjdvdzdxfdriyzn.supabase.co",
		"ftp://hdtwfdjdvdzdxfdriyzn.supabase.co",
	}
	for _, raw := range blocked {
		if err := refuseXRAY(raw); err == nil {
			t.Fatalf("expected refuse for %q", raw)
		}
	}
}

func TestRunRefusesXRAYCaseVariantsWithoutNetwork(t *testing.T) {
	path := t.TempDir() + "/cars.json"
	out, err := Run(t.Context(), Config{
		URL:         "https://CHJQCZNYXVTJBAMTTQDJ.supabase.co",
		ServiceRole: "super-secret-service-role-token",
		MirrorPath:  path,
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "CAR1"}}, nil, nil)
	if err == nil {
		t.Fatal("expected XRAY refuse")
	}
	if !strings.Contains(err.Error(), "XRAY") {
		t.Fatalf("want XRAY in error, got %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-service-role-token") {
		t.Fatal("leaked service role")
	}
	if out == nil || out.Source != "mock-mirror" {
		t.Fatalf("fallback snapshot: %+v", out)
	}
}

func TestRunRefusesOtherSupabaseProject(t *testing.T) {
	_, err := Run(t.Context(), Config{
		URL:         "https://abcdefghijklmnopqr.supabase.co",
		ServiceRole: "super-secret-service-role-token",
	}, []CarRow{{PDIID: "PDI-0001"}}, nil, nil)
	if err == nil {
		t.Fatal("expected non-fleet refuse")
	}
	if strings.Contains(err.Error(), "super-secret-service-role-token") {
		t.Fatal("leaked service role")
	}
}

func TestRedirectToXRAYRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://CHJQCZNYXVTJBAMTTQDJ.supabase.co/rest/v1/fleet_cars", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	path := t.TempDir() + "/cars.json"
	_, err := Run(t.Context(), Config{
		URL:         srv.URL,
		ServiceRole: "super-secret-service-role-token",
		MirrorPath:  path,
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "CAR1"}}, nil, nil)
	if err == nil {
		t.Fatal("expected redirect refuse")
	}
	if !strings.Contains(err.Error(), "XRAY") {
		t.Fatalf("want XRAY refuse, got %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-service-role-token") {
		t.Fatal("leaked service role")
	}
}

func TestRedirectToNonFleetHostRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/steal", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	_, err := Run(t.Context(), Config{
		URL:        srv.URL,
		SyncSecret: "super-secret-sync-token-value",
		MirrorPath: t.TempDir() + "/cars.json",
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "CAR1"}}, nil, nil)
	if err == nil {
		t.Fatal("expected redirect refuse")
	}
	if strings.Contains(err.Error(), "super-secret-sync-token-value") {
		t.Fatal("leaked sync token")
	}
}

func TestServiceRoleRedactedFromHTTPError(t *testing.T) {
	const secret = "super-secret-service-role-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid apikey "+secret+" also "+url.QueryEscape(secret), http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := Run(t.Context(), Config{
		URL:         srv.URL,
		ServiceRole: secret,
		MirrorPath:  t.TempDir() + "/cars.json",
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "CAR1"}}, nil, nil)
	if err == nil {
		t.Fatal("expected remote error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("leaked service role")
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("expected redaction marker: %v", err)
	}
}

func TestSyncSecretRedactedFromEdgeHTTPError(t *testing.T) {
	const secret = "super-secret-sync-token-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "fleet-sync") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "bad token "+secret, http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := Run(t.Context(), Config{
		URL:        srv.URL,
		SyncSecret: secret,
		MirrorPath: t.TempDir() + "/cars.json",
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "CAR1"}}, nil, nil)
	if err == nil {
		t.Fatal("expected remote error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("leaked sync token")
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("expected redaction marker: %v", err)
	}
}
