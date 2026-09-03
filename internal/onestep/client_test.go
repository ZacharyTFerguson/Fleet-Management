package onestep

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestDriveStopSumsMilesIgnoresOdometerJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/api/public/route/drive-stop" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("factory_id") != "FACT1" {
			t.Errorf("factory_id %s", r.URL.Query().Get("factory_id"))
		}
		_, _ = w.Write([]byte(`{\"odometer\":999999,\"stops\":[{\"miles\":3.2,\"odometer\":111},{\"distance\":1.3}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	n, err := c.DriveStopMiles(context.Background(), "FACT1", time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if n < 4.4 || n > 4.6 {
		t.Fatalf("sum %v (must not use odometer 999999)", n)
	}
}


func TestDriveStopRejectsRowsWithoutDistance(t *testing.T) {
	_, err := sumDriveStop([]byte(`{\"stops\":[{\"odometer\":999999}]}`))
	if err == nil {
		t.Fatal("device odometer must not become zero drive-stop miles")
	}
	n, err := sumDriveStop([]byte(`{\"stops\":[],\"odometer\":999999}`))
	if err != nil || n != 0 {
		t.Fatalf("empty measured trip list: miles=%v err=%v", n, err)
	}
}

func TestDriveStopRejectsNonObjectRows(t *testing.T) {
	if _, err := sumDriveStop([]byte(`{\"stops\":[null]}`)); err == nil {
		t.Fatal("null stop row must not become zero miles")
	}
}

func TestParseDevicesResultList(t *testing.T) {
	devs, err := parseDevices([]byte(`{\"result_list\":[{\"factory_id\":\"FACT1\",\"device_id\":\"DEV1\",\"display_name\":\"VA19\",\"odometer\":50}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].FactoryID != "FACT1" || devs[0].DeviceID != "DEV1" {
		t.Fatalf("%+v", devs)
	}
}

func TestJWTAuthHeaderNotRawKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	var sawAuth string
	var sawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[{\"factory_id\":\"FACT1\",\"device_id\":\"DEV1\"}]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "raw-api-key-must-not-appear")
	c.PrivateKeyPEM = string(pemBytes)
	c.HTTP = srv.Client()
	if _, err := c.ListDevices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sawAuth, "Bearer eyJ") {
		t.Fatalf("auth %q", sawAuth)
	}
	if strings.Contains(sawAuth, "raw-api-key-must-not-appear") || strings.Contains(sawQuery, "raw-api-key-must-not-appear") {
		t.Fatal("raw api key leaked")
	}
}

func TestAPIKeyQueryWhenNoPEM(t *testing.T) {
	var sawQuery string
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[{\"factory_id\":\"FACT1\",\"device_id\":\"DEV1\"}]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "query-key")
	c.HTTP = srv.Client()
	if _, err := c.ListDevices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sawAuth != "" {
		t.Fatalf("auth %q", sawAuth)
	}
	if !strings.Contains(sawQuery, "api-key=query-key") {
		t.Fatalf("query %q", sawQuery)
	}
}

func TestListDevicesFactoryID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{\"factory_id\":\"FACT1\",\"device_id\":\"DEV1\",\"display_name\":\"VA19\",\"odometer\":50}]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	devs, err := c.ListDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].FactoryID != "FACT1" {
		t.Fatalf("%+v", devs)
	}
}

func TestMapCSVIgnoresLogisticsPersonnelLink(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "onestep", "map.csv")
	devs, err := LoadMapCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		if d.FactoryID == "TYLERBOX" {
			t.Fatal("tyler box must not appear as a linkable device")
		}
		if d.FactoryID == "FACT1" && (d.LinkedCarEFleetsID == nil || *d.LinkedCarEFleetsID != "27TESTA") {
			t.Fatalf("FACT1 %+v", d)
		}
	}
}

func TestLinkByFactoryIDNotDisplayName(t *testing.T) {
	m := map[string]string{"FACT1": "27TESTA"}
	d := LinkByFactoryID(model.OneStepDevice{FactoryID: "NOPE", DisplayName: "27TESTA"}, m)
	if d.LinkedCarEFleetsID != nil {
		t.Fatal("must not join display_name 27TESTA")
	}
	d = LinkByFactoryID(model.OneStepDevice{FactoryID: "FACT1", DisplayName: "ignored-name"}, m)
	if d.LinkedCarEFleetsID == nil || *d.LinkedCarEFleetsID != "27TESTA" {
		t.Fatalf("factory_id join %+v", d)
	}
}

func TestHTTPErrorBodyRedactsAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied api-key=super-secret-token reflected", http.StatusUnauthorized)
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "super-secret-token")
	client.HTTP = srv.Client()
	_, err := client.ListDevices(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("leaked api key in error: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("expected redaction marker: %v", err)
	}
}
