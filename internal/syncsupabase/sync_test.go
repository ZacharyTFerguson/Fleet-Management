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

	"oilchange/internal/model"
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

func TestCarsForRemoteUpsertOmitsBlankIdentity(t *testing.T) {
	rows, err := carsForRemoteUpsert([]CarRow{{
		PDIID:     "PDI-0001",
		EFleetsID: "27VA15",
		Nickname:  "VA15",
		VIN:       "",
		Plate:     "-",
		Region:    "  ",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	if _, ok := rows[0]["vin"]; ok {
		t.Fatalf("blank VIN must be omitted so merge-duplicates cannot wipe remote: %#v", rows[0])
	}
	if _, ok := rows[0]["plate"]; ok {
		t.Fatalf("placeholder plate must be omitted: %#v", rows[0])
	}
	if _, ok := rows[0]["region"]; ok {
		t.Fatalf("blank region must be omitted: %#v", rows[0])
	}
	if rows[0]["nickname"] != "VA15" || rows[0]["efleets_id"] != "27VA15" || rows[0]["pdi_id"] != "PDI-0001" {
		t.Fatalf("known identity must stay: %#v", rows[0])
	}
}

func TestCarsForRemoteUpsertKeepsRealVIN(t *testing.T) {
	rows, err := carsForRemoteUpsert([]CarRow{{
		PDIID:     "PDI-0001",
		EFleetsID: "26LSZW",
		Nickname:  "Bing-2",
		VIN:       "4S3GTAB63P3706918",
		Plate:     "ABC1234",
		Region:    "VA",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["vin"] != "4S3GTAB63P3706918" {
		t.Fatalf("real VIN must be sent: %#v", rows[0]["vin"])
	}
	if rows[0]["plate"] != "ABC1234" || rows[0]["region"] != "VA" || rows[0]["nickname"] != "Bing-2" {
		t.Fatalf("real identity must be sent: %#v", rows[0])
	}
}

func TestFromCarsMirrorKeepsBlankVIN(t *testing.T) {
	rows := FromCars([]model.Car{{
		PDIID:     "PDI-0001",
		EFleetsID: "27VA15",
		Nickname:  "VA15",
	}})
	if len(rows) != 1 || rows[0].VIN != "" {
		t.Fatalf("mirror FromCars must still carry blank VIN: %+v", rows)
	}
}

func TestPushOmitsBlankVINInPostgRESTBody(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	_, err := Run(t.Context(), Config{
		URL:         srv.URL,
		ServiceRole: "test-key",
	}, []CarRow{{PDIID: "PDI-0001", EFleetsID: "27VA15", Nickname: "VA15", VIN: "", Plate: "-"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"vin":""`) {
		t.Fatalf("PostgREST upsert must not send empty vin: %s", body)
	}
	if strings.Contains(string(body), `"plate":"-"`) || strings.Contains(string(body), `"plate":""`) {
		t.Fatalf("PostgREST upsert must not send blank/placeholder plate: %s", body)
	}
	if !strings.Contains(string(body), `"efleets_id":"27VA15"`) {
		t.Fatalf("identity missing from upsert: %s", body)
	}
	if !strings.Contains(string(body), `"nickname":"VA15"`) {
		t.Fatalf("known nickname missing from upsert: %s", body)
	}
}
