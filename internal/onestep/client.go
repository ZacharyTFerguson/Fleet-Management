package onestep

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

// AuthMode is jwt-rs256 when oilchange.env has the OneStep usage-note PEM+key, else api-key query (Cursor guide).
func (c *Client) AuthMode() string {
	if c == nil || c.Token == "" {
		return "none"
	}
	if strings.TrimSpace(c.PrivateKeyPEM) == "" {
		return "api-key-query"
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
		fid := first(d.FactoryID, d.FactoryId, d.ID)
		did := first(d.DeviceID, d.DeviceId, d.ID)
		name := first(d.DisplayName, d.Name)
		if fid == "" {
			continue
		}
		out = append(out, model.OneStepDevice{
			FactoryID:   fid,
			DeviceID:    did,
			DisplayName: name,
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
		if sum, ok := sumMaps(arr); ok {
			return sum, nil
		}
		return 0, fmt.Errorf("drive-stop JSON rows had no miles")
	}
	for _, k := range []string{"stops", "routes", "data", "trips"} {
		if v, ok := obj[k]; ok {
			if sl, ok := v.([]any); ok {
				var maps []map[string]any
				for _, x := range sl {
					m, ok := x.(map[string]any)
					if !ok {
						return 0, fmt.Errorf("drive-stop JSON %s contains a non-object row", k)
					}
					maps = append(maps, m)
				}
				if sum, ok := sumMaps(maps); ok {
					return sum, nil
				}
				return 0, fmt.Errorf("drive-stop JSON %s rows had no miles", k)
			}
		}
	}
	if n, ok := asFloat(obj["miles"]); ok {
		return n, nil
	}
	if n, ok := asFloat(obj["distance"]); ok {
		return n, nil
	}
	return 0, fmt.Errorf("drive-stop JSON had no miles")
}

// sumMaps is GPS/trip distance, not a device odometer reading. An empty list
// is a measured zero; non-empty rows without distance are a malformed response.
func sumMaps(maps []map[string]any) (float64, bool) {
	var sum float64
	found := len(maps) == 0
	for _, m := range maps {
		if n, ok := asFloat(m["miles"]); ok {
			sum += n
			found = true
			continue
		}
		if n, ok := asFloat(m["distance"]); ok {
			sum += n
			found = true
			continue
		}
		if n, ok := asFloat(m["distance_miles"]); ok {
			sum += n
			found = true
		}
	}
	return sum, found
}

// asFloat accepts JSON number-or-string miles without defaulting missing to zero (zero would invent a trip).
func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	case int:
		return float64(t), true
	default:
		return 0, false
	}
}

// get is the only HTTP in this package; oil.LastReading never calls it.
// With a PEM, auth is a per-request RS256 JWT (OneStep usage note). Without PEM, api-key query (Cursor guide).
func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(path), nil)
	if err != nil {
		return nil, err
	}
	var sentAuth string
	if c.PrivateKeyPEM != "" && c.Token != "" {
		tok, err := signAPIKeyJWT(c.PrivateKeyPEM, c.Token, time.Minute)
		if err != nil {
			return nil, err
		}
		sentAuth = tok
		req.Header.Set("Authorization", "Bearer "+tok)
	} else if c.Token != "" {
		sentAuth = c.Token
		q.Set("api-key", c.Token)
	}
	if len(q) > 0 {
		req.URL.RawQuery = q.Encode()
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		msg := strings.TrimSpace(string(b))
		msg = redactAuthSecrets(msg, c.Token, sentAuth)
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		// Never echo query strings (api-key) or Authorization material.
		if msg == "" {
			return nil, fmt.Errorf("onestep %s: HTTP %s", path, res.Status)
		}
		return nil, fmt.Errorf("onestep %s: HTTP %s: %s", path, res.Status, msg)
	}
	return b, nil
}

// redactAuthSecrets strips API keys / JWTs that an upstream error body may echo.
func redactAuthSecrets(msg string, secrets ...string) string {
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, s, "[redacted]")
	}
	return msg
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
