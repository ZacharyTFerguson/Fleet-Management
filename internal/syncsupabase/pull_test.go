package syncsupabase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckPullURL(t *testing.T) {
	if err := checkPullURL("https://chjqcznyxvtjbamttqdj.supabase.co"); err == nil {
		t.Fatal("xray")
	}
	if err := checkPullURL("https://other.supabase.co"); err == nil {
		t.Fatal("wrong project")
	}
	if err := checkPullURL("https://hdtwfdjdvdzdxfdriyzn.supabase.co"); err != nil {
		t.Fatal(err)
	}
	if err := checkPullURL("http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
}

func TestFetchCars(t *testing.T) {
	miles := 100010
	hold := "NO_DEVICE"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "fleet_cars") {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.Header.Get("apikey") == "" || r.Header.Get("Authorization") == "" {
			t.Fatal("missing key headers")
		}
		_ = json.NewEncoder(w).Encode([]CarRow{{
			PDIID:            "PDI-0001",
			EFleetsID:        "27TESTA",
			Nickname:         "VA19",
			LastReadingMiles: &miles,
			HoldReason:       &hold,
		}})
	}))
	defer srv.Close()
	rows, err := FetchCars(t.Context(), Config{URL: srv.URL, AnonKey: "test-anon"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].EFleetsID != "27TESTA" || rows[0].LastReadingMiles == nil {
		t.Fatalf("%+v", rows)
	}
	c := CarFromRow(rows[0])
	if c.LastReadingMiles == nil || *c.LastReadingMiles != 100010 {
		t.Fatalf("from row %+v", c)
	}
}

func TestFetchCarsMissingKey(t *testing.T) {
	_, err := FetchCars(t.Context(), Config{URL: "https://hdtwfdjdvdzdxfdriyzn.supabase.co"})
	if err == nil {
		t.Fatal("expected missing key")
	}
}

func TestFetchDevicesRequiresServiceRole(t *testing.T) {
	_, err := FetchDevices(t.Context(), Config{URL: "https://hdtwfdjdvdzdxfdriyzn.supabase.co", AnonKey: "anon"})
	if err == nil {
		t.Fatal("anon must not fetch devices")
	}
}
