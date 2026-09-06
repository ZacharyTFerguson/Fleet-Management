package deskauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPasswordHashAndSession(t *testing.T) {
	h, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct-horse") || CheckPassword(h, "wrong") {
		t.Fatal("bcrypt")
	}
	if h == "correct-horse" {
		t.Fatal("stored plaintext")
	}
	m := NewManager([]byte("test-session-key-32-bytes-long!!"))
	tok := m.Sign("zach", time.Now().Add(time.Hour))
	u, ok := m.Verify(tok)
	if !ok || u != "zach" {
		t.Fatalf("verify %q %v", u, ok)
	}
	if _, ok := m.Verify(tok + "x"); ok {
		t.Fatal("tamper")
	}
	expired := m.Sign("zach", time.Now().Add(-time.Minute))
	if _, ok := m.Verify(expired); ok {
		t.Fatal("expired")
	}
}

func TestCookieRoundTrip(t *testing.T) {
	m := NewManager([]byte("cookie-key"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	m.SetCookie(rec, req, "desk")
	res := rec.Result()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Cookie", res.Header.Get("Set-Cookie"))
	u, ok := m.UserFromRequest(req2)
	if !ok || u != "desk" {
		t.Fatalf("user %q %v", u, ok)
	}
}
