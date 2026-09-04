package onestep

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseNearAddressJSONFixture(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "onestep", "near_address_rows.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := ParseNearAddressJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits %d", len(hits))
	}
	if hits[0].FactoryID != "3271116717" || hits[0].DeviceID == "" {
		t.Fatalf("factory %+v", hits[0])
	}
	if hits[0].Entity == hits[0].FactoryID {
		t.Fatal("entity must not replace factory_id")
	}
	if !hits[0].HasMiles || hits[0].Miles < 0.19 || hits[0].Miles > 0.21 {
		t.Fatalf("miles %+v", hits[0])
	}
	if !hits[0].HasPos || hits[0].Lat < 40 || hits[0].Lng > -79 {
		t.Fatalf("pos %+v", hits[0])
	}
	if hits[1].FactoryID != "1111111111" {
		t.Fatalf("watch %+v", hits[1])
	}
}

func TestParseNearAddressRequiresFactoryID(t *testing.T) {
	hits, err := ParseNearAddressJSON([]byte(`{"result_list":[{"near_address_device_id":"DEV-ONLY","near_address_entity":"LABEL"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("device_id-only must not be a factory_id: %+v", hits)
	}
}

func TestParseNearAddressNaiveEasternTime(t *testing.T) {
	hits, err := ParseNearAddressJSON([]byte(`{"result_list":[{"near_address_factory_id":"F1","stop_start_time":"2026-06-16 10:14:00"}]}`))
	if err != nil || len(hits) != 1 {
		t.Fatalf("%+v %v", hits, err)
	}
	ny, _ := time.LoadLocation("America/New_York")
	want := time.Date(2026, 6, 16, 10, 14, 0, 0, ny).UTC()
	if !hits[0].From.Equal(want) {
		t.Fatalf("got %s want %s", hits[0].From, want)
	}
}

func TestParseNearAddressIgnoresOdometerOnlyRow(t *testing.T) {
	hits, err := ParseNearAddressJSON([]byte(`{"result_list":[{"odometer":99999,"near_address_entity":"LABEL"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("entity-only row is not a factory_id: %+v", hits)
	}
}

func TestMarshalNearAddressSpecLiveShape(t *testing.T) {
	from := time.Date(2026, 6, 15, 4, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 18, 3, 59, 59, 999000000, time.UTC)
	b, err := marshalNearAddressSpec(NearAddressQuery{
		Address: "203 CRAIGDELL RD, LOWER BURRELL, PA",
		From:    from,
		To:      to,
	})
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	if spec["report_type"] != NearAddressReportType {
		t.Fatalf("type %v", spec["report_type"])
	}
	if spec["all_user_devices"] != true || spec["exclude_inactive_devices"] != true {
		t.Fatalf("device flags %v", spec)
	}
	if spec["time_zone"] != "America/New_York" {
		t.Fatalf("tz %v", spec["time_zone"])
	}
	opts, _ := spec["report_options_near_address"].(map[string]any)
	if opts["search_address_string"] != "203 CRAIGDELL RD, LOWER BURRELL, PA" {
		t.Fatalf("address %v", opts)
	}
	rng, _ := opts["range"].(map[string]any)
	if rng["unit"] != "mi" {
		t.Fatalf("range %v", rng)
	}
	if _, ok := spec["address"]; ok {
		t.Fatal("top-level address is dropped by OneStep; must not send it")
	}
	if spec["datetime_from"] == nil || spec["datetime_to"] == nil {
		t.Fatal("datetimes")
	}
}

func TestNearAddressGeneratePollDownloadMutex(t *testing.T) {
	oldEvery, oldMax := nearAddressPollEvery, nearAddressPollMax
	nearAddressPollEvery = time.Millisecond
	nearAddressPollMax = 2 * time.Second
	defer func() {
		nearAddressPollEvery, nearAddressPollMax = oldEvery, oldMax
	}()
	var current, max, posts, gets int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&current, 1)
		for {
			old := atomic.LoadInt32(&max)
			if c <= old || atomic.CompareAndSwapInt32(&max, old, c) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v3/api/public/report/generate":
			atomic.AddInt32(&posts, 1)
			body, _ := io.ReadAll(r.Body)
			var spec map[string]any
			_ = json.Unmarshal(body, &spec)
			if spec["report_type"] != "near_address" {
				http.Error(w, "bad type", 400)
				return
			}
			opts, _ := spec["report_options_near_address"].(map[string]any)
			if opts == nil || opts["search_address_string"] == "" {
				http.Error(w, "missing near options", 400)
				return
			}
			_, _ = w.Write([]byte(`{"report_generated_id":"job1","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v3/api/public/report-generated/job1":
			atomic.AddInt32(&gets, 1)
			_, _ = w.Write([]byte(`{"report_generated_id":"job1","status":"done","OutputFilePath":"/tmp/out.json"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v3/api/public/report-generated/job1/file":
			atomic.AddInt32(&gets, 1)
			_, _ = w.Write([]byte(`{"result_list":[{"near_address_factory_id":"FACT9","near_address_device_id":"DEV9","distance_from_location":{"value":0.4,"unit":"mi"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	from := time.Date(2026, 6, 15, 4, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 18, 3, 59, 59, 0, time.UTC)
	q := NearAddressQuery{Address: "203 CRAIGDELL RD, LOWER BURRELL, PA", From: from, To: to}

	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hits, job, err := c.NearAddress(context.Background(), q)
			if err != nil {
				errCh <- err
				return
			}
			if job.ID != "job1" || len(hits) != 1 || hits[0].FactoryID != "FACT9" {
				errCh <- errString("hits", hits, job)
				return
			}
			errCh <- nil
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&max); got != 1 {
		t.Fatalf("in-flight max %d want 1 (near_address HTTP must stay mutex-serialized)", got)
	}
	if atomic.LoadInt32(&posts) != n {
		t.Fatalf("posts %d", posts)
	}
}

func TestNearAddressJWTFallbackStaysSerialized(t *testing.T) {
	var current, max, fallback int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "jwt rejected", http.StatusUnauthorized)
			return
		}
		c := atomic.AddInt32(&current, 1)
		for {
			old := atomic.LoadInt32(&max)
			if c <= old || atomic.CompareAndSwapInt32(&max, old, c) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&fallback, 1)
		atomic.AddInt32(&current, -1)
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"report_generated_id":"job2","status":"done"}`))
			return
		}
		_, _ = w.Write([]byte(`{"result_list":[{"near_address_factory_id":"F2","near_address_device_id":"D2"}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	c.PrivateKeyPEM = testPEM(t)
	from := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	to := from.Add(48 * time.Hour)
	q := NearAddressQuery{Address: "1 Main St", From: from, To: to}

	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.NearAddress(context.Background(), q)
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
	if got := atomic.LoadInt32(&max); got != 1 {
		t.Fatalf("fallback in-flight max %d want 1", got)
	}
	if atomic.LoadInt32(&fallback) < n {
		t.Fatalf("fallback hits %d", fallback)
	}
}

func TestLockedGetDoesNotNestLockOnGet(t *testing.T) {
	// get() must stay lock-free so fetchDriveStopBytes can hold mu then call get.
	var inGet int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&inGet, 1)
		_, _ = w.Write([]byte(`{"report_generated_id":"x","status":"done"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	if _, err := c.get(context.Background(), "/v3/api/public/report-generated/x", nil); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&inGet) != 1 {
		t.Fatalf("hits %d", inGet)
	}
}

func errString(msg string, hits []NearAddressHit, job NearAddressJob) error {
	b, _ := json.Marshal(hits)
	return errText(msg + " " + job.ID + " " + string(b))
}

type errText string

func (e errText) Error() string { return string(e) }
