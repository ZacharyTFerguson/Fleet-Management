package enterprise

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChromeSessionAdapter reuses an already-open Chrome eFleets tab via CDP.
// It never types a password and never walks portal menus. Export bytes come
// from Network-captured EFLEETS_*_URL only, with cookies pulled over the
// CDP WebSocket. A login-page body is a hard error — not a password fallback.
type ChromeSessionAdapter struct {
	CDPURL     string
	DetailsURL string
	MaintURL   string
	FleetURL   string
	// Client is shared by the CDP HTTP probe and the export GET so tests stay on httptest.
	Client *http.Client
	// Dial opens the DevTools WebSocket. Tests inject a fake; live uses DialCDP.
	Dial func(ctx context.Context, wsURL string) (CDPConn, error)
}

// NewChromeSessionAdapter requires EFLEETS_CDP_URL so we never fall back to
// password HTTP just because a Chrome flag was forgotten.
func NewChromeSessionAdapter(cdp string, details, maint, fleet string) (*ChromeSessionAdapter, error) {
	if strings.TrimSpace(cdp) == "" {
		return nil, fmt.Errorf("EFLEETS_CDP_URL is required (Chrome remote-debugging URL; no password typing)")
	}
	return &ChromeSessionAdapter{
		CDPURL:     strings.TrimRight(strings.TrimSpace(cdp), "/"),
		DetailsURL: details,
		MaintURL:   maint,
		FleetURL:   fleet,
		Client:     &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// HasReport is true when the matching Network-captured URL is set, so
// SyncEnterprise can fetch without probing Chrome during unit tests.
func (a *ChromeSessionAdapter) HasReport(kind ReportKind) bool {
	_, _, err := exportURL(kind, a.DetailsURL, a.MaintURL, a.FleetURL)
	return err == nil
}

func (a *ChromeSessionAdapter) httpClient() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return &http.Client{Timeout: 120 * time.Second}
}

// Fetch attaches to the open Chrome, reuses session cookies over the CDP
// WebSocket, then GETs the captured export URL. Missing attach, missing URL,
// or login-page HTML must fail before any parse. No vision. No invented miles.
func (a *ChromeSessionAdapter) Fetch(ctx context.Context, kind ReportKind) ([]byte, string, error) {
	if err := a.probeCDP(ctx); err != nil {
		return nil, "", err
	}
	u, name, err := exportURL(kind, a.DetailsURL, a.MaintURL, a.FleetURL)
	if err != nil {
		return nil, "", err
	}
	cookies, err := a.sessionCookies(ctx, u)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	applyCDPCookies(req, cookies)
	res, err := a.httpClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode >= 400 {
		return nil, "", fmt.Errorf("eFleets %s: HTTP %s", kind, res.Status)
	}
	if looksLikeLoginHTML(b) {
		return nil, "", fmt.Errorf("eFleets %s returned the login page after CDP cookie reuse; Chrome tab is not a logged-in session (no password typing, no HTTP login fallback)", kind)
	}
	return b, name, nil
}

// probeCDP hits /json/version so we know remote debugging is up without
// driving the page or typing into the login form.
func (a *ChromeSessionAdapter) probeCDP(ctx context.Context) error {
	client := a.httpClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.CDPURL+"/json/version", nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Chrome session missing at EFLEETS_CDP_URL (no password typing; file-drop is --vehicles/--fuel-details): %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("EFLEETS_CDP_URL did not expose /json/version (is Chrome listening with remote debugging?)")
	}
	return nil
}

// sessionCookies pulls cookies from the logged-in tab over the CDP WebSocket.
// It does not navigate, click, or type. Empty cookies are allowed — Fetch
// still GETs and then fails hard if the body is login HTML.
func (a *ChromeSessionAdapter) sessionCookies(ctx context.Context, exportURL string) ([]cdpCookie, error) {
	client := a.httpClient()
	wsURL, useStorage, err := a.debuggerURL(ctx, client, exportURL)
	if err != nil {
		return nil, err
	}
	dial := a.Dial
	if dial == nil {
		dial = DialCDP
	}
	conn, err := dial(ctx, wsURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if useStorage {
		raw, err := conn.Call(ctx, "Storage.getCookies", nil)
		if err != nil {
			return nil, fmt.Errorf("CDP Storage.getCookies failed (no password typing): %w", err)
		}
		return parseCookieResult(raw)
	}

	if _, err := conn.Call(ctx, "Network.enable", nil); err != nil {
		return nil, fmt.Errorf("CDP Network.enable failed (no password typing): %w", err)
	}
	raw, err := conn.Call(ctx, "Network.getCookies", map[string]any{"urls": []string{exportURL}})
	if err != nil {
		return nil, fmt.Errorf("CDP Network.getCookies failed (no password typing): %w", err)
	}
	cookies, err := parseCookieResult(raw)
	if err != nil {
		return nil, err
	}
	if len(cookies) > 0 {
		return cookies, nil
	}
	raw, err = conn.Call(ctx, "Network.getAllCookies", nil)
	if err != nil {
		return nil, fmt.Errorf("CDP Network.getAllCookies failed (no password typing): %w", err)
	}
	return parseCookieResult(raw)
}

// debuggerURL prefers a page-target WebSocket (Network.getCookies). Browser
// Storage.getCookies is the fallback when Chrome has no matching eFleets tab.
func (a *ChromeSessionAdapter) debuggerURL(ctx context.Context, client *http.Client, exportURL string) (wsURL string, useStorage bool, err error) {
	targets, listErr := listCDPTargets(ctx, client, a.CDPURL)
	if listErr == nil {
		if page, pickErr := pickEFleetsPage(targets, exportURL); pickErr == nil {
			return rewriteWSHost(page.WebSocketDebuggerURL, a.CDPURL), false, nil
		} else {
			err = pickErr
		}
	} else {
		err = listErr
	}
	ver, verErr := getCDPVersion(ctx, client, a.CDPURL)
	if verErr == nil && strings.TrimSpace(ver.WebSocketDebuggerURL) != "" {
		return rewriteWSHost(ver.WebSocketDebuggerURL, a.CDPURL), true, nil
	}
	if err != nil {
		return "", false, err
	}
	return "", false, fmt.Errorf("Chrome DevTools WebSocket URL missing at EFLEETS_CDP_URL (no password typing)")
}
