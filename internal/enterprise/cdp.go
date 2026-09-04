package enterprise

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// CDPConn is one Chrome DevTools WebSocket. Tests inject a fake; live uses DialCDP.
type CDPConn interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Close() error
}

type cdpTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpVersion struct {
	Browser              string `json:"Browser"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
	SameSite string `json:"sameSite"`
}

type cdpCookieList struct {
	Cookies []cdpCookie `json:"cookies"`
}

type gorillaCDP struct {
	conn *websocket.Conn
	next int64
}

// DialCDP attaches to Chrome's DevTools WebSocket so we can read cookies
// without typing a password or driving the page.
func DialCDP(ctx context.Context, wsURL string) (CDPConn, error) {
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := d.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("CDP WebSocket attach failed (no password typing): %w", err)
	}
	return &gorillaCDP{conn: conn}, nil
}

// Call sends one CDP JSON-RPC method and waits for the matching id.
// Events (method-only frames) are skipped so Network noise cannot steal the reply.
func (c *gorillaCDP) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.next, 1)
	msg := struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{ID: id, Method: method, Params: params}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return nil, err
	}
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	if err := c.conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("CDP %s write: %w", method, err)
	}
	for {
		var raw map[string]json.RawMessage
		if err := c.conn.ReadJSON(&raw); err != nil {
			return nil, fmt.Errorf("CDP %s: %w", method, err)
		}
		if _, isEvent := raw["method"]; isEvent && raw["id"] == nil {
			continue
		}
		var got int64
		if err := json.Unmarshal(raw["id"], &got); err != nil || got != id {
			continue
		}
		if errRaw, ok := raw["error"]; ok && len(errRaw) > 0 && string(errRaw) != "null" {
			return nil, fmt.Errorf("CDP %s: %s", method, errRaw)
		}
		return raw["result"], nil
	}
}

// Close drops the DevTools socket so a later Fetch opens a fresh attach.
func (c *gorillaCDP) Close() error {
	return c.conn.Close()
}

// listCDPTargets reads /json/list (then /json) so we attach to a page, not a worker.
func listCDPTargets(ctx context.Context, client *http.Client, cdp string) ([]cdpTarget, error) {
	var last error
	for _, path := range []string{"/json/list", "/json"} {
		targets, err := getJSON[[]cdpTarget](ctx, client, cdp+path)
		if err != nil {
			last = err
			continue
		}
		if len(targets) > 0 {
			return targets, nil
		}
	}
	if last != nil {
		return nil, fmt.Errorf("EFLEETS_CDP_URL did not expose /json/list: %w", last)
	}
	return nil, fmt.Errorf("EFLEETS_CDP_URL /json/list was empty (open a logged-in eFleets tab)")
}

// getCDPVersion reads /json/version so we can fall back to the browser-level socket.
func getCDPVersion(ctx context.Context, client *http.Client, cdp string) (cdpVersion, error) {
	return getJSON[cdpVersion](ctx, client, cdp+"/json/version")
}

func getJSON[T any](ctx context.Context, client *http.Client, rawURL string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return zero, err
	}
	res, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return zero, err
	}
	if res.StatusCode >= 400 {
		return zero, fmt.Errorf("HTTP %s", res.Status)
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, err
	}
	return out, nil
}

// pickEFleetsPage chooses the logged-in eFleets tab. It will not reuse an
// unrelated tab when several pages are open (no cookie theft from mail/etc.).
func pickEFleetsPage(targets []cdpTarget, exportURL string) (cdpTarget, error) {
	exportHost := hostOf(exportURL)
	var pages []cdpTarget
	for _, t := range targets {
		if !strings.EqualFold(t.Type, "page") || strings.TrimSpace(t.WebSocketDebuggerURL) == "" {
			continue
		}
		pages = append(pages, t)
	}
	if len(pages) == 0 {
		return cdpTarget{}, fmt.Errorf("Chrome has no page target at EFLEETS_CDP_URL (open a logged-in eFleets tab; no password typing)")
	}
	for _, t := range pages {
		if pageLooksLikeEFleets(t, exportHost) {
			return t, nil
		}
	}
	if len(pages) == 1 {
		return pages[0], nil
	}
	return cdpTarget{}, fmt.Errorf("Chrome has no eFleets tab matching the export host (will not reuse an unrelated tab; no password typing)")
}

// pageLooksLikeEFleets is host/title only. No vision and no menu walking.
func pageLooksLikeEFleets(t cdpTarget, exportHost string) bool {
	h := hostOf(t.URL)
	if exportHost != "" && h == exportHost {
		return true
	}
	blob := strings.ToLower(t.URL + " " + t.Title)
	return strings.Contains(blob, "efleets")
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// rewriteWSHost forces the DevTools socket onto the EFLEETS_CDP_URL host.
// Chrome often advertises localhost while the operator set 127.0.0.1.
func rewriteWSHost(wsURL, cdpHTTP string) string {
	wu, err1 := url.Parse(wsURL)
	hu, err2 := url.Parse(cdpHTTP)
	if err1 != nil || err2 != nil || wu.Host == "" || hu.Host == "" {
		return wsURL
	}
	wu.Host = hu.Host
	if wu.Scheme == "http" {
		wu.Scheme = "ws"
	}
	if wu.Scheme == "https" {
		wu.Scheme = "wss"
	}
	return wu.String()
}

// applyCDPCookies copies matching Chrome cookies onto the export GET.
// It never invents a name/value and never adds Authorization.
func applyCDPCookies(req *http.Request, cookies []cdpCookie) {
	var parts []string
	for _, c := range cookies {
		if cookieApplies(req.URL, c) {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	if len(parts) > 0 {
		req.Header.Set("Cookie", strings.Join(parts, "; "))
	}
}

// cookieApplies is domain+path (+ Secure on non-loopback). Loopback http is
// allowed so httptest can prove reuse without a TLS export URL.
func cookieApplies(u *url.URL, c cdpCookie) bool {
	if c.Name == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	domain := strings.ToLower(strings.TrimPrefix(c.Domain, "."))
	if domain != "" && host != domain && !strings.HasSuffix(host, "."+domain) {
		return false
	}
	path := c.Path
	if path == "" {
		path = "/"
	}
	reqPath := u.EscapedPath()
	if reqPath == "" {
		reqPath = "/"
	}
	if path != "/" {
		if !strings.HasPrefix(reqPath, path) {
			return false
		}
		if len(reqPath) > len(path) && !strings.HasSuffix(path, "/") && reqPath[len(path)] != '/' {
			return false
		}
	}
	if c.Secure && u.Scheme != "https" && !isLoopbackHost(host) {
		return false
	}
	return true
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parseCookieResult(raw json.RawMessage) ([]cdpCookie, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list cdpCookieList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list.Cookies, nil
}
