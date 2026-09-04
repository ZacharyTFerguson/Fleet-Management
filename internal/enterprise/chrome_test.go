package enterprise

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

const detailsCSV = "Unit,Odometer\nVA1,100000\n"
const loginHTML = `<html><title>Client Login</title><form><input type="password" name="password"></form></html>`

type cdpScript struct {
	pageURL   string
	pageTitle string
	wsPath    string
	cookies   []cdpCookie
	pages     []cdpTarget
	noList bool
}

func startCDPServer(t *testing.T, script cdpScript) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	if script.wsPath == "" {
		script.wsPath = "/devtools/page/1"
	}
	if script.pageURL == "" {
		script.pageURL = "https://login.efleets.com/fleetweb"
	}
	if script.pageTitle == "" {
		script.pageTitle = "eFleets"
	}

	mux := http.NewServeMux()
	var srv *httptest.Server

	wsURL := func(r *http.Request, path string) string {
		return "ws://" + r.Host + path
	}

	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"Browser":              "Chrome/test",
			"webSocketDebuggerUrl": wsURL(r, "/devtools/browser"),
		})
	})
	list := func(w http.ResponseWriter, r *http.Request) {
		if script.noList {
			http.Error(w, "no list", http.StatusNotFound)
			return
		}
		pages := script.pages
		if pages == nil {
			pages = []cdpTarget{{
				ID:                   "1",
				Type:                 "page",
				Title:                script.pageTitle,
				URL:                  script.pageURL,
				WebSocketDebuggerURL: wsURL(r, script.wsPath),
			}}
		} else {
			for i := range pages {
				if pages[i].WebSocketDebuggerURL == "" && pages[i].Type == "page" {
					pages[i].WebSocketDebuggerURL = wsURL(r, script.wsPath)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(pages)
	}
	mux.HandleFunc("/json/list", list)
	mux.HandleFunc("/json", list)

	handleWS := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var msg struct {
				ID     int64          `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Method {
			case "Network.enable":
				_ = conn.WriteJSON(map[string]any{"id": msg.ID, "result": map[string]any{}})
			case "Network.getCookies", "Network.getAllCookies", "Storage.getCookies":
				_ = conn.WriteJSON(map[string]any{"id": msg.ID, "result": map[string]any{"cookies": script.cookies}})
			default:
				_ = conn.WriteJSON(map[string]any{"id": msg.ID, "error": map[string]any{"message": "unexpected " + msg.Method}})
			}
		}
	}
	mux.HandleFunc("/devtools/page/1", handleWS)
	mux.HandleFunc("/devtools/browser", handleWS)

	mux.HandleFunc("/export/DETAILS.csv", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("must not send a password / Authorization header")
		}
		if strings.Contains(strings.ToLower(r.Header.Get("Cookie")), "password=") {
			t.Error("must not send a password cookie")
		}
		if !strings.Contains(r.Header.Get("Cookie"), "JSESSIONID=sid") {
			_, _ = w.Write([]byte(loginHTML))
			return
		}
		_, _ = w.Write([]byte(detailsCSV))
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestNewChromeSessionAdapterRequiresCDP(t *testing.T) {
	_, err := NewChromeSessionAdapter("", "https://example.invalid/DETAILS.csv", "", "")
	if err == nil || !strings.Contains(err.Error(), "EFLEETS_CDP_URL") {
		t.Fatalf("want CDP required, got %v", err)
	}
}

func TestChromeSessionFetchReusesCDPCookies(t *testing.T) {
	srv := startCDPServer(t, cdpScript{
		cookies: []cdpCookie{{
			Name:   "JSESSIONID",
			Value:  "sid",
			Domain: "127.0.0.1",
			Path:   "/",
		}},
	})
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
	if string(b) != detailsCSV {
		t.Fatalf("must return server DETAILS bytes, not invent miles: %q", b)
	}
}

func TestChromeSessionFetchLoginHTMLIsHardError(t *testing.T) {
	srv := startCDPServer(t, cdpScript{
		pageURL: "https://login.efleets.com/fleetweb/login",
		cookies: nil, // attached, but not logged in
	})
	ad, err := NewChromeSessionAdapter(srv.URL, srv.URL+"/export/DETAILS.csv", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ad.Client = srv.Client()
	_, _, err = ad.Fetch(context.Background(), ReportFuelDetails)
	if err == nil || !strings.Contains(err.Error(), "login page") || !strings.Contains(err.Error(), "no HTTP login fallback") {
		t.Fatalf("want login-page hard error, got %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "username") || strings.Contains(err.Error(), "EFLEETS_PASSWORD") {
		t.Fatalf("must not suggest password typing: %v", err)
	}
}

func TestChromeSessionFetchRefusesMenuHuntWhenURLMissing(t *testing.T) {
	srv := startCDPServer(t, cdpScript{})
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

func TestChromeSessionFetchBrowserStorageCookiesFallback(t *testing.T) {
	srv := startCDPServer(t, cdpScript{
		noList: true,
		cookies: []cdpCookie{{
			Name:   "JSESSIONID",
			Value:  "sid",
			Domain: "127.0.0.1",
			Path:   "/",
		}},
	})
	ad, err := NewChromeSessionAdapter(srv.URL, srv.URL+"/export/DETAILS.csv", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ad.Client = srv.Client()
	b, _, err := ad.Fetch(context.Background(), ReportFuelDetails)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != detailsCSV {
		t.Fatalf("browser-level cookies must still yield DETAILS bytes: %q", b)
	}
}

func TestChromeSessionFetchWSAttachFailureIsHardError(t *testing.T) {
	srv := startCDPServer(t, cdpScript{})
	ad, err := NewChromeSessionAdapter(srv.URL, srv.URL+"/export/DETAILS.csv", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ad.Client = srv.Client()
	ad.Dial = func(context.Context, string) (CDPConn, error) {
		return nil, errTestDial
	}
	_, _, err = ad.Fetch(context.Background(), ReportFuelDetails)
	if err == nil || !strings.Contains(err.Error(), "dial refused") {
		t.Fatalf("want WS attach hard error, got %v", err)
	}
}

var errTestDial = errString("CDP WebSocket attach failed (no password typing): dial refused")

type errString string

func (e errString) Error() string { return string(e) }

func TestChromeSessionHasReportDoesNotDial(t *testing.T) {
	ad := &ChromeSessionAdapter{CDPURL: "http://127.0.0.1:9", DetailsURL: "https://example.invalid/DETAILS.csv"}
	if !ad.HasReport(ReportFuelDetails) {
		t.Fatal("DETAILS URL is set")
	}
	if ad.HasReport(ReportShopRO) {
		t.Fatal("no MAINT URL")
	}
}

func TestChromeSessionFetchUsesCapturedURLNotPassword(t *testing.T) {
	srv := startCDPServer(t, cdpScript{
		cookies: []cdpCookie{{
			Name:   "JSESSIONID",
			Value:  "sid",
			Domain: "127.0.0.1",
			Path:   "/",
		}},
	})
	ad, err := NewChromeSessionAdapter(srv.URL, srv.URL+"/export/DETAILS.csv", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ad.Client = srv.Client()
	b, _, err := ad.Fetch(context.Background(), ReportFuelDetails)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "VA1") {
		t.Fatalf("body %q", b)
	}
}
