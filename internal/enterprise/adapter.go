package enterprise

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReportKind is which eFleets grid to pull. File drop and live download share this so parsers stay header-based.
type ReportKind string

const (
	ReportVehicles    ReportKind = "vehicles"
	ReportFuelDetails ReportKind = "fuel_details"
	ReportShopRO      ReportKind = "shop_ro"
	ReportMileage     ReportKind = "mileage_history"
)

// Adapter returns CSV bytes for a report. Tests use files; live uses a session against login.efleets.com.
type Adapter interface {
	Fetch(ctx context.Context, kind ReportKind) ([]byte, string, error)
}

// FileAdapter is the test path and the operator fallback when CSVs are already in Downloads.
type FileAdapter struct {
	Vehicles  string
	Fuel      string
	ShopRO    string
	Mileage   string
}

// Fetch opens a local export. Live Last Reading still goes through the same parsers.
func (a FileAdapter) Fetch(_ context.Context, kind ReportKind) ([]byte, string, error) {
	var path string
	switch kind {
	case ReportVehicles:
		path = a.Vehicles
	case ReportFuelDetails:
		path = a.Fuel
	case ReportShopRO:
		path = a.ShopRO
	case ReportMileage:
		path = a.Mileage
	default:
		return nil, "", fmt.Errorf("unknown report %s", kind)
	}
	if path == "" {
		return nil, "", fmt.Errorf("no file for %s", kind)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return b, filepath.Base(path), nil
}

// HTTPAdapter is leftover password-form login. Tonight's lock is: reuse an
// already-open Chrome eFleets session (no password typing); file drop if that
// session is missing. Do not treat this adapter as always-on HTTP login.
type HTTPAdapter struct {
	BaseURL    string
	Username   string
	Password   string
	CustNum    string
	Client     *http.Client
	DetailsURL string
	MaintURL   string
	FleetURL   string
}

// NewHTTPAdapter builds a session client. Missing username/password/cust must fail before any parse so we never invent files.
func NewHTTPAdapter(base, user, pass, cust string) (*HTTPAdapter, error) {
	if user == "" || pass == "" {
		return nil, fmt.Errorf("EFLEETS_USERNAME and EFLEETS_PASSWORD are required for live download")
	}
	if strings.TrimSpace(cust) == "" {
		return nil, fmt.Errorf("EFLEETS_CUST_NUM is required for live download (no hardcoded default)")
	}
	if base == "" {
		base = "https://login.efleets.com"
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &HTTPAdapter{
		BaseURL:  strings.TrimRight(base, "/"),
		Username: user,
		Password: pass,
		CustNum:  cust,
		Client: &http.Client{
			Jar:     jar,
			Timeout: 120 * time.Second,
		},
	}, nil
}

// Fetch logs in if needed and GETs the export. Unknown portal paths return a clear error instead of a guessed CSV.
func (a *HTTPAdapter) Fetch(ctx context.Context, kind ReportKind) ([]byte, string, error) {
	if err := a.login(ctx); err != nil {
		return nil, "", err
	}
	u, name, err := a.urlFor(kind)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	res, err := a.Client.Do(req)
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
		return nil, "", fmt.Errorf("eFleets %s returned the login page; session did not stick", kind)
	}
	return b, name, nil
}

// urlFor refuses to guess portal export paths; wrong CSV would silently feed fake miles.
func (a *HTTPAdapter) urlFor(kind ReportKind) (string, string, error) {
	switch kind {
	case ReportFuelDetails:
		if a.DetailsURL != "" {
			return a.DetailsURL, "DETAILS.csv", nil
		}
		return "", "", fmt.Errorf("set EFLEETS_DETAILS_URL to the Fuel DETAILS export (portal path is not public)")
	case ReportShopRO:
		if a.MaintURL != "" {
			return a.MaintURL, "Maintenance.csv", nil
		}
		return "", "", fmt.Errorf("set EFLEETS_MAINT_URL to the Maintenance Detail export (see fleetweb/maintenanceSummary?maintenanceTab=detail)")
	case ReportVehicles:
		if a.FleetURL != "" {
			return a.FleetURL, "FleetSummary.csv", nil
		}
		return "", "", fmt.Errorf("set EFLEETS_FLEETSUMMARY_URL to the Fleet Summary CSV export")
	case ReportMileage:
		return "", "", fmt.Errorf("mileage history is optional; pass --mileage-history")
	default:
		return "", "", fmt.Errorf("unknown report %s", kind)
	}
}

// login POSTs the client login form. If the portal adds MFA or a JS wall, we stop rather than launch a browser.
func (a *HTTPAdapter) login(ctx context.Context) error {
	loginURL := a.BaseURL + "/fleetweb"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return err
	}
	res, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	action := formAction(string(body), res.Request.URL)
	vals := url.Values{}
	vals.Set("username", a.Username)
	vals.Set("password", a.Password)
	vals.Set("j_username", a.Username)
	vals.Set("j_password", a.Password)
	post, err := http.NewRequestWithContext(ctx, http.MethodPost, action, strings.NewReader(vals.Encode()))
	if err != nil {
		return err
	}
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res2, err := a.Client.Do(post)
	if err != nil {
		return err
	}
	b2, _ := io.ReadAll(res2.Body)
	res2.Body.Close()
	if looksLikeLoginHTML(b2) && res2.StatusCode < 400 {
		return fmt.Errorf("eFleets login did not leave the Client Login page (MFA/JS would require a browser, which is forbidden)")
	}
	if res2.StatusCode >= 400 {
		return fmt.Errorf("eFleets login HTTP %s", res2.Status)
	}
	return nil
}

// looksLikeLoginHTML catches a bounced session so we do not parse the login page as DETAILS.
func looksLikeLoginHTML(b []byte) bool {
	s := strings.ToLower(string(b[:min(len(b), 4000)]))
	return strings.Contains(s, "client login") && strings.Contains(s, "password")
}

// formAction follows the portal's own login POST so we do not hardcode a path that will rot.
func formAction(html string, page *url.URL) string {
	low := strings.ToLower(html)
	i := strings.Index(low, "<form")
	if i < 0 {
		return page.String()
	}
	chunk := html[i:]
	if j := strings.Index(strings.ToLower(chunk), ">"); j >= 0 {
		tag := chunk[:j]
		if k := strings.Index(strings.ToLower(tag), "action="); k >= 0 {
			rest := tag[k+7:]
			rest = strings.TrimLeft(rest, " \t")
			if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
				q := rest[0]
				rest = rest[1:]
				if e := strings.IndexByte(rest, q); e >= 0 {
					raw := rest[:e]
					u, err := page.Parse(raw)
					if err == nil {
						return u.String()
					}
					return raw
				}
			}
		}
	}
	return page.String()
}

// min caps HTML sniffing so a 2MB DETAILS file is not scanned for "password".
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DecodeCSV turns fetched bytes into an io.Reader for the header parsers.
func DecodeCSV(b []byte) io.Reader {
	return bytes.NewReader(b)
}
