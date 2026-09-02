package syncsupabase

import (
	"encoding/json"
	"os"
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
