package onestep

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseRetryAfterSecondsAndDate(t *testing.T) {
	if ParseRetryAfter("12") != 12*time.Second {
		t.Fatalf("seconds")
	}
	if ParseRetryAfter("0") != 0 || ParseRetryAfter("") != 0 {
		t.Fatal("zero")
	}
	future := time.Now().UTC().Add(5 * time.Second).Format(http.TimeFormat)
	d := ParseRetryAfter(future)
	if d < 3*time.Second || d > 8*time.Second {
		t.Fatalf("http-date %s", d)
	}
}

func TestGetReturnsRetryAfterOn429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	_, err := c.GetPublic(t.Context(), "/v3/api/public/route/drive-stop", nil)
	if err == nil {
		t.Fatal("expected 429")
	}
	if RetryAfterOf(err) != 7*time.Second {
		t.Fatalf("retry-after %s err=%v", RetryAfterOf(err), err)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != 429 {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("%v", err)
	}
}
