package onestep

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestDriveStopSumsMilesIgnoresOdometerJSON(t *testing.T) {
	from := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 4, 7, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/api/public/route/drive-stop" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("device_id") != "DEV1" {
			t.Errorf("device_id %s", q.Get("device_id"))
		}
		if q.Get("factory_id") != "" {
			t.Errorf("factory_id must not be a drive-stop query: %s", q.Get("factory_id"))
		}
		if q.Get("from") != "" {
			t.Errorf("from is not the proven param: %s", q.Get("from"))
		}
		if q.Get("dt_tracker_from") != FormatTrackerTime(from) {
			t.Errorf("dt_tracker_from %s", q.Get("dt_tracker_from"))
		}
		if q.Get("dt_tracker_to") != FormatTrackerTime(to) {
			t.Errorf("dt_tracker_to %s", q.Get("dt_tracker_to"))
		}
		_, _ = w.Write([]byte(`{"odometer":999999,"drive_stop_list":[{"distance":3.2,"odometer":111},{"distance":1.3}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	c.Now = func() time.Time { return to }
	n, err := c.DriveStopMilesFor(context.Background(), model.OneStepDevice{FactoryID: "FACT1", DeviceID: "DEV1"}, from)
	if err != nil {
		t.Fatal(err)
	}
	if n < 4.4 || n > 4.6 {
		t.Fatalf("sum %v (must not use odometer 999999)", n)
	}
}

func TestDriveStopRejectsRowsWithoutDistance(t *testing.T) {
	_, err := sumDriveStop([]byte(`{"stops":[{"odometer":999999}]}`))
	if err == nil {
		t.Fatal("device odometer must not become zero drive-stop miles")
	}
	n, err := sumDriveStop([]byte(`{"stops":[],"odometer":999999}`))
	if err != nil || n != 0 {
		t.Fatalf("empty measured trip list: miles=%v err=%v", n, err)
	}
}

func TestDriveStopRejectsMalformedMiles(t *testing.T) {
	for _, body := range []string{
		`{"stops":[{"miles":-1}]}`,
		`{"stops":[{"miles":"NaN"}]}`,
		`{"stops":[{"distance":"Inf"}]}`,
		`{"stops":[{"distance_miles":"-Inf"}]}`,
		`{"stops":[{"miles":1},{"distance":"NaN"}]}`,
	} {
		if n, err := sumDriveStop([]byte(body)); err == nil {
			t.Errorf("sumDriveStop(%s) = %v, want error", body, n)
		}
	}
}

func TestDriveStopRejectsNonObjectRows(t *testing.T) {
	if _, err := sumDriveStop([]byte(`{"stops":[null]}`)); err == nil {
		t.Fatal("null stop row must not become zero miles")
	}
}

func TestDriveStopRootDistanceIgnoresOdometer(t *testing.T) {
	n, err := sumDriveStop([]byte(`{"distance":594.9,"odometer":999999}`))
	if err != nil {
		t.Fatal(err)
	}
	if n < 594.8 || n > 595.0 {
		t.Fatalf("distance %v (must not use odometer 999999)", n)
	}
	if n, err := sumDriveStop([]byte(`{"odometer":123456}`)); err == nil {
		t.Fatalf("odometer-only must not become miles=%v", n)
	}
}

func TestDriveStopListPreferredOverRootDistance(t *testing.T) {
	n, err := sumDriveStop([]byte(`{"distance":1,"odometer":999999,"drive_stop_list":[{"distance":300.4},{"distance":294.5}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if n < 594.8 || n > 595.0 {
		t.Fatalf("list sum %v", n)
	}
}

func TestDriveStopMissingDeviceIDFailsLocally(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "access denied", http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	_, err := c.DriveStopMiles(context.Background(), "", time.Unix(0, 0).UTC())
	if err == nil {
		t.Fatal("empty device_id must fail before HTTP")
	}
	if hits != 0 {
		t.Fatalf("must not dial drive-stop with an empty query, hits=%d", hits)
	}
	if !strings.Contains(err.Error(), "device_id is required") {
		t.Fatalf("err %v", err)
	}
}

func TestDriveStop403IsMissingQueryNotDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"odometer":999999,"error":"access denied"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	n, err := c.DriveStopMiles(context.Background(), "DEV1", time.Unix(0, 0).UTC())
	if err == nil || n != 0 {
		t.Fatalf("403 must not become miles=%v err=%v", n, err)
	}
	if !strings.Contains(err.Error(), "drive-stop is enabled") {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "not enabled") {
		t.Fatalf("must not treat 403 as API-not-enabled: %v", err)
	}
	if strings.Contains(err.Error(), "999999") {
		t.Fatalf("must not surface odo on 403: %v", err)
	}
}

func TestDriveStop401IsJWSRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "raw token", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	_, err := c.DriveStopMiles(context.Background(), "DEV1", time.Unix(0, 0).UTC())
	if err == nil {
		t.Fatal("expected 401")
	}
	if !strings.Contains(err.Error(), "JWS required") {
		t.Fatalf("err %v", err)
	}
}

func TestDriveStopGolden594UsesDistanceNotOdo(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	body, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "testdata", "onestep", "drive-stop-200.json"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := sumDriveStop(body)
	if err != nil {
		t.Fatal(err)
	}
	if n < 594.8 || n > 595.0 {
		t.Fatalf("golden miles %v (must be drive_stop_list distance, not odo 999999)", n)
	}
}

func TestParseDevicesResultList(t *testing.T) {
	devs, err := parseDevices([]byte(`{"result_list":[{"factory_id":"FACT1","device_id":"DEV1","display_name":"VA19","odometer":50}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].FactoryID != "FACT1" || devs[0].DeviceID != "DEV1" {
		t.Fatalf("%+v", devs)
	}
}

func TestParseDevicesDoesNotPromoteGenericID(t *testing.T) {
	devs, err := parseDevices([]byte(`[{"id":"history-device-id","display_name":"VA19"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 0 {
		t.Fatalf("generic id became factory_id: %+v", devs)
	}
}

func TestParseDevicesMarksInactiveAndDeadNonLive(t *testing.T) {
	devs, err := parseDevices([]byte(`[
		{"factory_id":"INACTIVE","active":false},
		{"factory_id":"DEAD","active":true,"dead":true},
		{"factory_id":"LIVE"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 3 {
		t.Fatalf("devices %+v", devs)
	}
	if !devs[0].Dead {
		t.Fatalf("inactive device parsed as live: %+v", devs[0])
	}
	if !devs[1].Dead {
		t.Fatalf("dead device parsed as live: %+v", devs[1])
	}
	if devs[2].Dead {
		t.Fatalf("missing active flag should default live: %+v", devs[2])
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
		_, _ = w.Write([]byte(`[{"factory_id":"FACT1","device_id":"DEV1"}]`))
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
		_, _ = w.Write([]byte(`[{"factory_id":"FACT1","device_id":"DEV1"}]`))
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
		_, _ = w.Write([]byte(`[{"factory_id":"FACT1","device_id":"DEV1","display_name":"VA19","odometer":50}]`))
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

func TestListDevicesSkipsIDOnlyEndpoint(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v3/api/public/device-info":
			_, _ = w.Write([]byte(`[{"id":"history-only"}]`))
		case "/v3/api/public/device":
			_, _ = w.Write([]byte(`[{"factory_id":"FACTORY","device_id":"DEVICE"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "token")
	client.HTTP = srv.Client()
	devs, err := client.ListDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].FactoryID != "FACTORY" {
		t.Fatalf("devices %+v", devs)
	}
	if got := strings.Join(paths, ","); got != "/v3/api/public/device-info,/v3/api/public/device" {
		t.Fatalf("paths %s", got)
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

func TestHTTPErrorBodyRedactsJWT(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	var sawJWT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		sawJWT = strings.TrimPrefix(auth, "Bearer ")
		http.Error(w, "invalid token "+sawJWT, http.StatusUnauthorized)
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "raw-api-key-must-not-appear")
	client.PrivateKeyPEM = string(pemBytes)
	client.HTTP = srv.Client()
	_, err = client.ListDevices(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if sawJWT == "" || !strings.HasPrefix(sawJWT, "eyJ") {
		t.Fatalf("expected JWT auth, got %q", sawJWT)
	}
	if strings.Contains(err.Error(), sawJWT) {
		t.Fatalf("leaked JWT in error: %v", err)
	}
	if strings.Contains(err.Error(), "raw-api-key-must-not-appear") {
		t.Fatalf("leaked api key in error: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("expected redaction marker: %v", err)
	}
}

func TestHTTPErrorRedactsURLEncodedAuthAndQuery(t *testing.T) {
	token := "secret key+/="
	encoded := url.QueryEscape(token)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied "+encoded+" at https://example.test/device?foo=bar&api-key="+encoded, http.StatusUnauthorized)
	}))
	defer srv.Close()
	client := NewClient(srv.URL, token)
	client.HTTP = srv.Client()
	_, err := client.ListDevices(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	for _, leaked := range []string{token, encoded, "foo=bar", "api-key="} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("expected redaction marker: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTransportURLErrorDoesNotLeakRequestQuery(t *testing.T) {
	token := "transport key+/="
	client := NewClient("https://example.test", token)
	client.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed for " + req.URL.String())
	})}
	_, err := client.ListDevices(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	for _, leaked := range []string{token, url.QueryEscape(token), "api-key=", "latest_point="} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("transport error leaked %q: %v", leaked, err)
		}
	}
}

func TestAuthenticatedRedirectIsNotFollowed(t *testing.T) {
	token := "redirect-secret"
	sinkHits := 0
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinkHits++
		if r.Header.Get("Authorization") != "" || r.URL.Query().Get("api-key") != "" {
			t.Errorf("redirect target received auth: header=%q query=%q", r.Header.Get("Authorization"), r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"factory_id":"STOLEN"}]`))
	}))
	defer sink.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL+"/capture?api-key="+url.QueryEscape(token), http.StatusFound)
	}))
	defer source.Close()

	client := NewClient(source.URL, token)
	client.HTTP = source.Client()
	_, err := client.ListDevices(context.Background())
	if err == nil {
		t.Fatal("expected redirect refusal")
	}
	if sinkHits != 0 {
		t.Fatalf("followed authenticated redirect %d times", sinkHits)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "api-key=") {
		t.Fatalf("redirect error leaked auth: %v", err)
	}
}
