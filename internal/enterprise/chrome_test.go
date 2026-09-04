package enterprise

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewChromeSessionAdapterRequiresCDP(t *testing.T) {
	_, err := NewChromeSessionAdapter("", "https://example.invalid/DETAILS.csv", "", "")
	if err == nil || !strings.Contains(err.Error(), "EFLEETS_CDP_URL") {
		t.Fatalf("want CDP required, got %v", err)
	}
}

func TestChromeSessionFetchUsesCapturedURLNotPassword(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Browser":"Chrome/test"}`))
	})
	mux.HandleFunc("/export/DETAILS.csv", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("must not send a password header")
		}
		_, _ = w.Write([]byte("Unit,Odometer\nVA1,100000\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ad, err := NewChromeSessionAdapter(srv.URL, srv.URL+"/export/DETAILS.csv", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ad.Client = srv.Client()
	b, name, err := ad.Fetch(context.Background(), ReportFuelDetails)
	if err != nil {
		t.Fatal(err)
	}
	if name != "DETAILS.csv" {
		t.Fatalf("name %q", name)
	}
	if !strings.Contains(string(b), "VA1") {
		t.Fatalf("body %q", b)
	}
}

func TestChromeSessionFetchRefusesMenuHuntWhenURLMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Browser":"Chrome/test"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ad, err := NewChromeSessionAdapter(srv.URL, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ad.Client = srv.Client()
	_, _, err = ad.Fetch(context.Background(), ReportFuelDetails)
	if err == nil || !strings.Contains(err.Error(), "DevTools Network") {
		t.Fatalf("want Network-capture error, got %v", err)
	}
}

func TestChromeSessionFetchMissingChromeDoesNotInvent(t *testing.T) {
	ad, err := NewChromeSessionAdapter("http://127.0.0.1:9", "https://example.invalid/DETAILS.csv", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ad.Client = &http.Client{}
	_, _, err = ad.Fetch(context.Background(), ReportFuelDetails)
	if err == nil || !strings.Contains(err.Error(), "Chrome session missing") {
		t.Fatalf("want missing session, got %v", err)
	}
}

func TestChromeSessionHasReportDoesNotDial(t *testing.T) {
	ad := &ChromeSessionAdapter{CDPURL: "http://127.0.0.1:9", DetailsURL: "https://example.invalid/DETAILS.csv"}
	if !ad.HasReport(ReportFuelDetails) {
		t.Fatal("DETAILS URL is set")
	}
	if ad.HasReport(ReportShopRO) {
		t.Fatal("no MAINT URL")
	}
}
