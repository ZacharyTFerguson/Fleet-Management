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
// from Network-captured EFLEETS_*_URL only. Cookie reuse over the CDP
// WebSocket is the next increment; this stub only proves attach + GET.
type ChromeSessionAdapter struct {
	CDPURL     string
	DetailsURL string
	MaintURL   string
	FleetURL   string
	// Client is shared by the CDP probe and the export GET so tests stay on httptest.
	Client *http.Client
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

// Fetch attaches to the open Chrome, then GETs the captured export URL.
// Missing attach or missing URL must fail before any parse.
func (a *ChromeSessionAdapter) Fetch(ctx context.Context, kind ReportKind) ([]byte, string, error) {
	if err := a.probeCDP(ctx); err != nil {
		return nil, "", err
	}
	u, name, err := exportURL(kind, a.DetailsURL, a.MaintURL, a.FleetURL)
	if err != nil {
		return nil, "", err
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	res, err := client.Do(req)
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
		return nil, "", fmt.Errorf("eFleets %s returned the login page; Chrome is attached but this stub does not pull cookies yet (no password typing). File-drop or leave the tab logged in after cookie reuse", kind)
	}
	return b, name, nil
}

// probeCDP hits /json/version so we know remote debugging is up without
// driving the page or typing into the login form.
func (a *ChromeSessionAdapter) probeCDP(ctx context.Context) error {
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	u := a.CDPURL + "/json/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
