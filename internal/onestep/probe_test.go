package onestep

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeRequiresAllThreeParams(t *testing.T) {
	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		t.Error("HTTP must not run without device_id/from/to")
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	now := time.Now().UTC()
	if _, err := c.ProbeDriveStop(context.Background(), "", now.Add(-time.Hour), now); err == nil {
		t.Fatal("empty device_id")
	}
	if _, err := c.ProbeDriveStop(context.Background(), "dev1", time.Time{}, now); err == nil {
		t.Fatal("zero from")
	}
	if _, err := c.ProbeDriveStop(context.Background(), "dev1", now, now); err == nil {
		t.Fatal("to not after from")
	}
	if hits != 0 {
		t.Fatalf("HTTP hits=%d", hits)
	}
}

func TestProbeSendsThreeParamsIgnoresOdometer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/api/public/route/drive-stop" {
			t.Errorf("path %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("device_id") != "devA" {
			t.Errorf("device_id %s", q.Get("device_id"))
		}
		if q.Get("factory_id") != "" || q.Get("from") != "" {
			t.Errorf("must not send factory_id/from: %s", r.URL.RawQuery)
		}
		if q.Get("dt_tracker_from") == "" || q.Get("dt_tracker_to") == "" {
			t.Errorf("missing window")
		}
		if q.Get("api-key") != "tok" && r.Header.Get("Authorization") == "" {
			t.Error("expected auth")
		}
		_, _ = w.Write([]byte(`{"odometer":99999,"distance":{"value":12.5,"unit":"mi"},"odometer_from":1,"odometer_to":2}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	res, err := c.ProbeDriveStop(context.Background(), "devA", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if res.Miles != 12.5 {
		t.Fatalf("miles %v (must not use odometer)", res.Miles)
	}
	if res.AuthMode != "api-key-query" {
		t.Fatalf("auth %s", res.AuthMode)
	}
}

func TestProbeMutexSerializesConcurrentCalls(t *testing.T) {
	var current int32
	var max int32
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&current, 1)
		for {
			old := atomic.LoadInt32(&max)
			if c <= old || atomic.CompareAndSwapInt32(&max, old, c) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&n, 1)
		atomic.AddInt32(&current, -1)
		_, _ = w.Write([]byte(`{"distance":{"value":1,"unit":"mi"}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC()
	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.ProbeDriveStop(context.Background(), "devA", from, to)
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
	if n != 16 {
		t.Fatalf("calls %d", n)
	}
	if max != 1 {
		t.Fatalf("in-flight max %d want 1 (mutex)", max)
	}
}
