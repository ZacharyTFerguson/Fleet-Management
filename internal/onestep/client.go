package onestep

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// MilesClient is the drive-stop fetch. Device odometer fields on JSON are ignored by design.
type MilesClient interface {
	DriveStopMiles(ctx context.Context, factoryID string, since time.Time) (float64, error)
}

// Client talks to track.onestepgps.com. Pairing is factory_id only; display_name is never a join key.
type Client struct {
	Base          string
	Token         string
	PrivateKeyPEM string
	HTTP          *http.Client
}

// AuthMode is jwt-rs256 when oilchange.env has token + PEM. This account
// does not use ?api-key= or a raw Bearer API key.
func (c *Client) AuthMode() string {
	if c == nil || strings.TrimSpace(c.Token) == "" {
		return "none"
	}
	if strings.TrimSpace(c.PrivateKeyPEM) == "" {
		return "missing-pem"
	}
	if _, err := signAPIKeyJWT(c.PrivateKeyPEM, c.Token, time.Minute); err != nil {
		return "jwt-rs256-invalid-pem"
	}
	return "jwt-rs256"
}

// NewClient builds an API client. Empty token is allowed for tests that inject HTTP.
func NewClient(base, token string) *Client {
	if base == "" {
		base = "https://track.onestepgps.com"
	}
	return &Client{
		Base:  strings.TrimRight(base, "/"),
		Token: token,
		HTTP:  &http.Client{Timeout: 60 * time.Second},
	}
}

type deviceJSON struct {
	FactoryID   string `json:"factory_id"`
	FactoryId   string `json:"factoryId"`
	DeviceID    string `json:"device_id"`
	DeviceId    string `json:"device_id_history"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	Active      *bool  `json:"active"`
	IsActive    *bool  `json:"is_active"`
	Dead        bool   `json:"dead"`
	Odometer    any    `json:"odometer"` // present on some payloads; must not be used as Last Reading
}

// ListDevices GET device-info (OneStep demo) then /device (Cursor guide). Odometer JSON is discarded.
func (c *Client) ListDevices(ctx context.Context) ([]model.OneStepDevice, error) {
	q := url.Values{}
	q.Set("latest_point", "true")
	paths := []string{"/v3/api/public/device-info", "/v3/api/public/device", "/v3/api/public/devices"}
	var last error
	for _, p := range paths {
		b, err := c.get(ctx, p, q)
		if err != nil {
			last = err
			continue
		}
		devs, err := parseDevices(b)
		if err != nil {
			last = err
			continue
		}
		if len(devs) == 0 {
			last = fmt.Errorf("onestep %s: empty device list", p)
			continue
		}
		return devs, nil
	}
	if last == nil {
		last = fmt.Errorf("no device endpoint")
	}
	return nil, last
}

// parseDevices keeps factory_id/device_id/display_name and drops odometer so History UI cannot leak in.
func parseDevices(b []byte) ([]model.OneStepDevice, error) {
	var raw []deviceJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		var wrap struct {
			Devices    []deviceJSON `json:"devices"`
			Data       []deviceJSON `json:"data"`
			ResultList []deviceJSON `json:"result_list"`
			Result     []deviceJSON `json:"result"`
		}
		if err2 := json.Unmarshal(b, &wrap); err2 != nil {
			var one deviceJSON
			if err3 := json.Unmarshal(b, &one); err3 != nil {
				return nil, err
			}
			raw = []deviceJSON{one}
		} else {
			raw = wrap.ResultList
			if len(raw) == 0 {
				raw = wrap.Devices
			}
			if len(raw) == 0 {
				raw = wrap.Data
			}
			if len(raw) == 0 {
				raw = wrap.Result
			}
		}
	}
	var out []model.OneStepDevice
	for _, d := range raw {
		// A generic id is an API/history identity, not the hardware factory_id.
		// Skipping id-only rows lets ListDevices try the actual inventory endpoint.
		fid := first(d.FactoryID, d.FactoryId)
		did := first(d.DeviceID, d.DeviceId, d.ID)
		name := first(d.DisplayName, d.Name)
		if fid == "" {
			continue
		}
		active := true
		if d.Active != nil && !*d.Active {
			active = false
		}
		if d.IsActive != nil && !*d.IsActive {
			active = false
		}
		if d.Dead {
			active = false
		}
		out = append(out, model.OneStepDevice{
			FactoryID:   fid,
			DeviceID:    did,
			DisplayName: name,
			// This branch's model uses Dead as its only non-live state.
			Dead: d.Dead || !active,
		})
	}
	return out, nil
}

// DriveStopMiles GET /v3/api/public/route/drive-stop and sums trip distance. Ignores device odo fields.
func (c *Client) DriveStopMiles(ctx context.Context, factoryID string, since time.Time) (float64, error) {
	return c.driveStopMiles(ctx, factoryID, factoryID, since)
}

// DriveStopMilesFor pairs on factory_id then calls drive-stop with device_id (Cursor guide).
func (c *Client) DriveStopMilesFor(ctx context.Context, d model.OneStepDevice, since time.Time) (float64, error) {
	did := d.DeviceID
	if did == "" {
		did = d.FactoryID
	}
	return c.driveStopMiles(ctx, d.FactoryID, did, since)
}

func (c *Client) driveStopMiles(ctx context.Context, factoryID, deviceID string, since time.Time) (float64, error) {
	q := url.Values{}
	if factoryID != "" {
		q.Set("factory_id", factoryID)
	}
	if deviceID != "" {
		q.Set("device_id", deviceID)
	}
	q.Set("from", since.UTC().Format(time.RFC3339))
	b, err := c.get(ctx, "/v3/api/public/route/drive-stop", q)
	if err != nil {
		return 0, err
	}
	return sumDriveStop(b)
}

// sumDriveStop adds trip miles only. An odometer field on the same JSON is ignored.
func sumDriveStop(b []byte) (float64, error) {
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		var arr []map[string]any
		if err2 := json.Unmarshal(b, &arr); err2 != nil {
			return 0, err
		}
		return sumMaps(arr)
	}
	for _, k := range []string{"stops", "routes", "data", "trips"} {
		if v, ok := obj[k]; ok {
			if sl, ok := v.([]any); ok {
				var maps []map[string]any
				for i, x := range sl {
					m, ok := x.(map[string]any)
					if !ok {
						return 0, fmt.Errorf("drive-stop JSON %s row %d is not an object", k, i)
					}
					maps = append(maps, m)
				}
				sum, err := sumMaps(maps)
				if err != nil {
					return 0, fmt.Errorf("drive-stop JSON %s: %w", k, err)
				}
				return sum, nil
			}
		}
	}
	if v, ok := obj["miles"]; ok {
		n, valid := asFloat(v)
		if !valid {
			return 0, fmt.Errorf("drive-stop JSON root miles is not a finite non-negative number")
		}
		return n, nil
	}
	return 0, fmt.Errorf("drive-stop JSON had no miles")
}

// sumMaps is GPS/trip distance, not a device odometer reading. An empty list
// is a measured zero; non-empty rows without distance are a malformed response.
func sumMaps(maps []map[string]any) (float64, error) {
	var sum float64
	for i, m := range maps {
		found := false
		for _, key := range []string{"miles", "distance", "distance_miles"} {
			v, exists := m[key]
			if !exists {
				continue
			}
			n, ok := asFloat(v)
			if !ok {
				return 0, fmt.Errorf("row %d %s is not a finite non-negative number", i, key)
			}
			sum += n
			if math.IsNaN(sum) || math.IsInf(sum, 0) {
				return 0, fmt.Errorf("row %d makes total miles non-finite", i)
			}
			found = true
			break
		}
		if !found {
			return 0, fmt.Errorf("row %d had no miles", i)
		}
	}
	return sum, nil
}

// asFloat accepts only finite, non-negative JSON number-or-string miles.
func asFloat(v any) (float64, bool) {
	var n float64
	switch t := v.(type) {
	case float64:
		n = t
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		n = f
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		n = f
	case int:
		n = float64(t)
	default:
		return 0, false
	}
	if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// get is the only HTTP in this package; oil.LastReading never calls it.
// Live auth is a per-request RS256 JWS {access_token, exp} as Authorization: Bearer <jwt>.
// Token without PEM, raw Bearer of the API key, and ?api-key= are all refused.
func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(path), nil)
	if err != nil {
		return nil, safeHTTPError(path, "build request", err, c.Token)
	}
	var sentAuth string
	token := strings.TrimSpace(c.Token)
	pem := strings.TrimSpace(c.PrivateKeyPEM)
	switch {
	case token != "" && pem != "":
		tok, err := signAPIKeyJWT(pem, token, time.Minute)
		if err != nil {
			return nil, safeHTTPError(path, "sign authentication", err, c.Token)
		}
		sentAuth = tok
		req.Header.Set("Authorization", "Bearer "+tok)
	case token != "" || pem != "":
		return nil, fmt.Errorf("onestep %s: need token + PEM (RS256 JWS); not raw Bearer, not api-key query", path)
	}
	if len(q) > 0 {
		req.URL.RawQuery = q.Encode()
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	redirectSafeClient := *httpClient
	// Authentication must never be replayed to a Location target. Returning the
	// 3xx response also keeps redirect URLs out of url.Error values.
	redirectSafeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	res, err := redirectSafeClient.Do(req)
	if err != nil {
		return nil, safeHTTPError(path, "request failed", err, c.Token, sentAuth)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, safeHTTPError(path, "read response", err, c.Token, sentAuth)
	}
	if res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		msg = sanitizeAuthError(msg, c.Token, sentAuth)
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		// Never echo query strings or Authorization material.
		if res.StatusCode == http.StatusForbidden && strings.Contains(path, "drive-stop") {
			return nil, fmt.Errorf("onestep %s: HTTP %s: HOLD/History — do not invent miles or use device odo", path, res.Status)
		}
		if msg == "" {
			return nil, fmt.Errorf("onestep %s: HTTP %s", path, res.Status)
		}
		return nil, fmt.Errorf("onestep %s: HTTP %s: %s", path, res.Status, msg)
	}
	return b, nil
}

var (
	errorURLQuery = regexp.MustCompile(`(?i)(https?://[^\s?#"'<>]+)(?:\?|%3f)[^\s#"'<>]*`)
	errorJWT      = regexp.MustCompile(`(?i)\beyJ[A-Za-z0-9_-]*(?:\.|%2e)[A-Za-z0-9_-]+(?:\.|%2e)[A-Za-z0-9_-]+\b`)
	errorBearer   = regexp.MustCompile(`(?i)\bbearer(?:\s+|%20+)[A-Za-z0-9._~+/=%-]+`)
	errorAuthPair = regexp.MustCompile(`(?i)\b(api[-_]?key|access[-_]?token|authorization)(?:\s*[:=]\s*|%3[ad])[^\s"'<>]+`)
)

func safeHTTPError(path, action string, err error, secrets ...string) error {
	msg := "unknown error"
	if err != nil {
		msg = sanitizeAuthError(err.Error(), secrets...)
	}
	return fmt.Errorf("onestep %s: %s: %s", path, action, msg)
}

// sanitizeAuthError strips request query strings and raw, percent-encoded, or
// generic credential echoes from both HTTP bodies and transport/url.Error text.
func sanitizeAuthError(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		msg = encodedSecretPattern(s).ReplaceAllString(msg, "[redacted]")
	}
	msg = errorURLQuery.ReplaceAllString(msg, `$1?[redacted]`)
	msg = errorJWT.ReplaceAllString(msg, "[redacted]")
	msg = errorBearer.ReplaceAllString(msg, "Bearer [redacted]")
	msg = errorAuthPair.ReplaceAllStringFunc(msg, func(pair string) string {
		if i := strings.IndexAny(pair, ":="); i >= 0 {
			return pair[:i+1] + "[redacted]"
		}
		if i := strings.Index(strings.ToLower(pair), "%3"); i >= 0 {
			return pair[:i] + "=[redacted]"
		}
		return "[redacted]"
	})
	return msg
}

// encodedSecretPattern matches a secret even when an echo percent-encodes only
// some bytes or uses '+' for spaces.
func encodedSecretPattern(secret string) *regexp.Regexp {
	parts := make([]string, 0, len(secret))
	for _, b := range []byte(secret) {
		alternatives := []string{regexp.QuoteMeta(string([]byte{b})), fmt.Sprintf("%%%02X", b)}
		if b == ' ' {
			alternatives = append(alternatives, `\+`)
		}
		parts = append(parts, "(?:"+strings.Join(alternatives, "|")+")")
	}
	return regexp.MustCompile("(?i)" + strings.Join(parts, ""))
}

// resolve joins Base with a /v3/api/public path whether Base is the host or already includes that prefix.
func (c *Client) resolve(path string) string {
	base := strings.TrimRight(c.Base, "/")
	const prefix = "/v3/api/public"
	if strings.HasSuffix(base, prefix) {
		path = strings.TrimPrefix(path, prefix)
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}
	return base + path
}

// first prefers factory_id over id so a History device_id cannot be mistaken for the hardware key.
func first(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// LoadMapCSV reads factory_id,device_id,efleets_id. Display_name columns are ignored as join keys.
func LoadMapCSV(path string) ([]model.OneStepDevice, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("empty device map")
	}
	idx := map[string]int{}
	for i, h := range all[0] {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	pick := func(row []string, names ...string) string {
		for _, n := range names {
			i, ok := idx[n]
			if ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}
	var out []model.OneStepDevice
	for _, row := range all[1:] {
		fid := pick(row, "factory_id", "factoryid", "onestep factory id")
		did := pick(row, "device_id", "deviceid", "onestep device id")
		eid := pick(row, "efleets_id", "efleetsid", "vehicle")
		name := pick(row, "display_name", "name")
		if fid == "" {
			continue
		}
		d := model.OneStepDevice{FactoryID: fid, DeviceID: did, DisplayName: name}
		if oil.HasLogisticsPersonnel(name) {
			d.LinkedCarEFleetsID = nil
			continue
		}
		if eid != "" {
			d.LinkedCarEFleetsID = &eid
		}
		out = append(out, d)
	}
	return out, nil
}

// LinkByFactoryID attaches a car using factory_id equality only. Display_name is not consulted.
func LinkByFactoryID(dev model.OneStepDevice, factoryToCar map[string]string) model.OneStepDevice {
	if oil.HasLogisticsPersonnel(dev.DisplayName) {
		dev.LinkedCarEFleetsID = nil
		return dev
	}
	if car, ok := factoryToCar[dev.FactoryID]; ok && car != "" {
		dev.LinkedCarEFleetsID = &car
	}
	return dev
}
