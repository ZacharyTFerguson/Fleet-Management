package onestep

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oilchange/internal/model"
)

func testPEM(t *testing.T) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// Live auth is jwt-rs256 and drive-stop answered HTTP 500/401 often enough that
// driveStopMiles falls back to an api-key client. That fallback is a fresh
// *Client with its own zero mutex, so the documented "drive-stop HTTP is
// mutex-serialized" lock only covers the request that failed, not the one that
// actually fetches miles. SyncOneStep over 146 boxes then fans out unserialized.
func TestDriveStopJWTFallbackStaysSerialized(t *testing.T) {
	var current, max, fallbackHits int32
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
		atomic.AddInt32(&fallbackHits, 1)
		atomic.AddInt32(&current, -1)
		_, _ = w.Write([]byte(`{"distance":{"value":1,"unit":"mi"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	c.PrivateKeyPEM = testPEM(t)
	since := time.Now().UTC().Add(-time.Hour)
	dev := model.OneStepDevice{FactoryID: "FACT1", DeviceID: "DEV1"}

	const n = 16
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.DriveStopMilesFor(context.Background(), dev, since)
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
	if got := atomic.LoadInt32(&fallbackHits); got != n {
		t.Fatalf("fallback calls %d want %d", got, n)
	}
	if got := atomic.LoadInt32(&max); got != 1 {
		t.Fatalf("api-key fallback in-flight max %d want 1 (drive-stop HTTP must stay mutex-serialized)", got)
	}
}
