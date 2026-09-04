package enterprise

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPickEFleetsPagePrefersExportHost(t *testing.T) {
	page, err := pickEFleetsPage([]cdpTarget{
		{Type: "page", Title: "Mail", URL: "https://mail.example.invalid/", WebSocketDebuggerURL: "ws://127.0.0.1/mail"},
		{Type: "page", Title: "Fleet", URL: "https://login.efleets.com/fleetweb", WebSocketDebuggerURL: "ws://127.0.0.1/e"},
		{Type: "service_worker", URL: "https://login.efleets.com/sw", WebSocketDebuggerURL: "ws://127.0.0.1/sw"},
	}, "https://login.efleets.com/export/DETAILS.csv")
	if err != nil {
		t.Fatal(err)
	}
	if page.WebSocketDebuggerURL != "ws://127.0.0.1/e" {
		t.Fatalf("picked %+v", page)
	}
}

func TestPickEFleetsPageRefusesUnrelatedTabs(t *testing.T) {
	_, err := pickEFleetsPage([]cdpTarget{
		{Type: "page", Title: "Mail", URL: "https://mail.example.invalid/", WebSocketDebuggerURL: "ws://127.0.0.1/mail"},
		{Type: "page", Title: "Docs", URL: "https://docs.example.invalid/", WebSocketDebuggerURL: "ws://127.0.0.1/docs"},
	}, "https://login.efleets.com/export/DETAILS.csv")
	if err == nil || !strings.Contains(err.Error(), "unrelated tab") {
		t.Fatalf("want refuse unrelated, got %v", err)
	}
}

func TestPickEFleetsPageSingleTabIsOk(t *testing.T) {
	page, err := pickEFleetsPage([]cdpTarget{
		{Type: "page", Title: "Portal", URL: "https://intranet.example.invalid/", WebSocketDebuggerURL: "ws://127.0.0.1/one"},
	}, "https://login.efleets.com/export/DETAILS.csv")
	if err != nil {
		t.Fatal(err)
	}
	if page.WebSocketDebuggerURL != "ws://127.0.0.1/one" {
		t.Fatalf("picked %+v", page)
	}
}

func TestCookieAppliesDomainAndLoopbackSecure(t *testing.T) {
	u, _ := url.Parse("https://login.efleets.com/fleetweb/export")
	if !cookieApplies(u, cdpCookie{Name: "JSESSIONID", Value: "x", Domain: ".efleets.com", Path: "/"}) {
		t.Fatal("suffix domain")
	}
	if cookieApplies(u, cdpCookie{Name: "JSESSIONID", Value: "x", Domain: "other.example", Path: "/"}) {
		t.Fatal("foreign domain")
	}
	loop, _ := url.Parse("http://127.0.0.1/export/DETAILS.csv")
	if !cookieApplies(loop, cdpCookie{Name: "JSESSIONID", Value: "sid", Domain: "127.0.0.1", Path: "/", Secure: true}) {
		t.Fatal("loopback http must still apply Secure cookies so httptest can prove reuse")
	}
}

func TestApplyCDPCookiesDoesNotInventAuthorization(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://login.efleets.com/DETAILS.csv", nil)
	applyCDPCookies(req, []cdpCookie{
		{Name: "JSESSIONID", Value: "sid", Domain: "login.efleets.com", Path: "/"},
		{Name: "other", Value: "nope", Domain: "evil.example", Path: "/"},
	})
	if req.Header.Get("Authorization") != "" {
		t.Fatal("must not set Authorization")
	}
	got := req.Header.Get("Cookie")
	if !strings.Contains(got, "JSESSIONID=sid") {
		t.Fatalf("missing session cookie: %q", got)
	}
	if strings.Contains(got, "nope") {
		t.Fatalf("foreign cookie leaked: %q", got)
	}
}

func TestRewriteWSHostUsesCDPListenHost(t *testing.T) {
	got := rewriteWSHost("ws://localhost:9222/devtools/page/1", "http://127.0.0.1:9222")
	if got != "ws://127.0.0.1:9222/devtools/page/1" {
		t.Fatalf("got %q", got)
	}
}

func TestParseCookieResultEmpty(t *testing.T) {
	got, err := parseCookieResult(json.RawMessage("null"))
	if err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
	got, err = parseCookieResult(json.RawMessage(`{"cookies":[{"name":"a","value":"b"}]}`))
	if err != nil || len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("%v %v", got, err)
	}
}
