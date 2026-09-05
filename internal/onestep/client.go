package onestep

import (
	"bytes"
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
	"sync"
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
	now           func() time.Time
	mu            sync.Mutex // serializes drive-stop HTTP (probes and sync)
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

type devicePointVIN struct {
	DeviceState struct {
		VIN string `json:"vin"`
	} `json:"device_state"`
}

type deviceJSON struct {
	FactoryID   string `json:"factory_id"`
	FactoryId   string `json:"factoryId"`
	IMEI        string `json:"imei"` // Device Information report hardware id (= factory_id)
	DeviceID    string `json:"device_id"`
	DeviceId    string `json:"device_id_history"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	DeviceName  string `json:"device_name"` // Device Information report label
	Active      *bool  `json:"active"`
	IsActive    *bool  `json:"is_active"`
	Dead        bool   `json:"dead"`
	Odometer    any    `json:"odometer"` // present on some payloads; must not be used as Last Reading
	VIN         string `json:"vin"`      // Device Information report VIN; never params.vin
	DecodedVIN  struct {
		VehicleVIN string `json:"vehicle_vin"`
	} `json:"decoded_vin"`
	Settings struct {
		VIN string `json:"vin"`
	} `json:"settings"`
	LatestDevicePoint         devicePointVIN `json:"latest_device_point"`
	LatestAccurateDevicePoint devicePointVIN `json:"latest_accurate_device_point"`
}

// ListDevices GET device-info (OneStep demo) then /device (Cursor guide). Odometer JSON is discarded.
func (c *Client) ListDevices(ctx context.Context) ([]model.OneStepDevice, error) {
	q := url.Values{}
	q.Set("latest_point", "true")
	// Live /device defaults to 100 rows; limit=1000 returns the full inventory (264 on 2026-09-04).
	q.Set("limit", "1000")
	paths := []string{"/v3/api/public/device-info", "/v3/api/public/device", "/v3/api/public/devices"}
	var last error
	for _, p := range paths {
		b, err := c.lockedGet(ctx, p, q)
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

// parseDevices keeps factory_id/device_id/display_name/VIN and drops odometer so History UI cannot leak in.
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
		fid := first(d.FactoryID, d.FactoryId, d.IMEI)
		did := first(d.DeviceID, d.DeviceId, d.ID)
		name := first(d.DisplayName, d.Name, d.DeviceName)
		// OBD device_state.vin is the car the box is plugged into; settings.vin is portal Vehicle Info.
		// Device Information reports (generate-reports `data`) use imei + top-level vin / decoded_vin.
		// params.vin is noisier and is not used. display_name / device_name is never an identity.
		vin := first(
			d.LatestDevicePoint.DeviceState.VIN,
			d.LatestAccurateDevicePoint.DeviceState.VIN,
			d.Settings.VIN,
			d.DecodedVIN.VehicleVIN,
			d.VIN,
		)
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
			VIN:         normalizeVIN(vin),
			Active:      active,
			Dead:        d.Dead,
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
	if deviceID == "" {
		deviceID = factoryID
	}
	if deviceID == "" {
		return 0, fmt.Errorf("drive-stop needs a device_id")
	}
	from := since.UTC()
	to := time.Now().UTC()
	if c != nil && c.now != nil {
		to = c.now().UTC()
	}
	if !to.After(from) {
		to = from.Add(time.Second)
	}
	n, err := c.fetchDriveStopWindow(ctx, deviceID, from, to)
	if err != nil && c.PrivateKeyPEM != "" && driveStopAuthFallback(err) {
		// Do not copy c: that would copy mu. Keep the distinct JWT-off client
		// under the parent lock so all drive-stop HTTP remains serialized.
		plain := &Client{Base: c.Base, Token: c.Token, HTTP: c.HTTP, now: c.now}
		c.mu.Lock()
		n, err = plain.fetchDriveStopWindow(ctx, deviceID, from, to)
		c.mu.Unlock()
	}
	if err != nil && driveStopRetryChunked(err) {
		n, err = c.fetchDriveStopChunked(ctx, deviceID, from, to)
	}
	return n, err
}

// Live apidoc names (alexbeattie/OneStepGPS + portal): device_id, dt_tracker_from, dt_tracker_to, stop_duration.
// Do not send factory_id or from — those 500. Do not request return_points (map UI only; it hung fleet sync).
// One device_id per request only — multi-device batch query params are not proven on this route
// (support.php recommends batching when an endpoint allows it; nearby paces serial calls instead).
func (c *Client) fetchDriveStopWindow(ctx context.Context, deviceID string, from, to time.Time) (float64, error) {
	b, err := c.fetchDriveStopBytes(ctx, deviceID, from, to)
	if err != nil {
		return 0, err
	}
	return sumDriveStop(b)
}

func (c *Client) fetchDriveStopBytes(ctx context.Context, deviceID string, from, to time.Time) ([]byte, error) {
	if c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
	}
	q := url.Values{}
	q.Set("device_id", deviceID)
	q.Set("dt_tracker_from", from.Format(time.RFC3339))
	q.Set("dt_tracker_to", to.Format(time.RFC3339))
	q.Set("stop_duration", "5m0s")
	return c.get(ctx, "/v3/api/public/route/drive-stop", q)
}

// DriveStopVisitsFor returns GPS stop windows for card matching. Not miles.
func (c *Client) DriveStopVisitsFor(ctx context.Context, d model.OneStepDevice, from, to time.Time) ([]model.StopVisit, error) {
	did := d.DeviceID
	if did == "" {
		did = d.FactoryID
	}
	if did == "" {
		return nil, fmt.Errorf("drive-stop needs a device_id")
	}
	from = from.UTC()
	to = to.UTC()
	if !to.After(from) {
		to = from.Add(time.Second)
	}
	var out []model.StopVisit
	for start := from; start.Before(to); {
		end := start.Add(driveStopChunk)
		if end.After(to) {
			end = to
		}
		b, err := c.fetchDriveStopBytes(ctx, did, start, end)
		if err != nil && c.PrivateKeyPEM != "" && driveStopAuthFallback(err) {
			// Same JWT-off fallback as driveStopMiles: do not copy mu; hold the parent lock.
			plain := &Client{Base: c.Base, Token: c.Token, HTTP: c.HTTP, now: c.now}
			c.mu.Lock()
			b, err = plain.fetchDriveStopBytes(ctx, did, start, end)
			c.mu.Unlock()
		}
		if err != nil {
			return nil, err
		}
		chunk, err := parseDriveStopVisits(b)
		if err != nil {
			return nil, err
		}
		eid := ""
		if d.LinkedCarEFleetsID != nil {
			eid = *d.LinkedCarEFleetsID
		}
		for _, v := range chunk {
			v.FactoryID = d.FactoryID
			v.DeviceID = did
			v.EFleetsID = eid
			out = append(out, v)
		}
		start = end
	}
	return out, nil
}

const driveStopChunk = 31 * 24 * time.Hour

func (c *Client) fetchDriveStopChunked(ctx context.Context, deviceID string, from, to time.Time) (float64, error) {
	var sum float64
	for start := from; start.Before(to); {
		end := start.Add(driveStopChunk)
		if end.After(to) {
			end = to
		}
		n, err := c.fetchDriveStopWindow(ctx, deviceID, start, end)
		if err != nil {
			return 0, err
		}
		sum += n
		if math.IsNaN(sum) || math.IsInf(sum, 0) {
			return 0, fmt.Errorf("drive-stop chunk sum is not finite")
		}
		start = end
	}
	return sum, nil
}

func driveStopAuthFallback(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 401") || strings.Contains(s, "HTTP 403") || strings.Contains(s, "HTTP 500")
}

func driveStopRetryChunked(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 500") || strings.Contains(s, "timeout") || strings.Contains(s, "deadline")
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
	// Live payload: root distance is {value, unit}. A bare number at root is not used (could be odo-like).
	if n, ok := measurementMiles(obj["distance"]); ok {
		return n, nil
	}
	if v, ok := obj["drive_stop_list"]; ok {
		if sl, ok := v.([]any); ok {
			return sumDriveStopList(sl)
		}
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

func sumDriveStopList(sl []any) (float64, error) {
	if len(sl) == 0 {
		return 0, nil
	}
	var sum float64
	anyDist := false
	for i, x := range sl {
		m, ok := x.(map[string]any)
		if !ok {
			return 0, fmt.Errorf("drive-stop JSON drive_stop_list row %d is not an object", i)
		}
		if n, ok := rowTripMiles(m); ok {
			sum += n
			if math.IsNaN(sum) || math.IsInf(sum, 0) {
				return 0, fmt.Errorf("drive-stop JSON drive_stop_list row %d makes total miles non-finite", i)
			}
			anyDist = true
			continue
		}
		typ, _ := m["type"].(string)
		if strings.EqualFold(strings.TrimSpace(typ), "drive") {
			return 0, fmt.Errorf("drive-stop JSON drive_stop_list row %d had no miles", i)
		}
	}
	if !anyDist {
		return 0, nil
	}
	return sum, nil
}

func parseDriveStopVisits(b []byte) ([]model.StopVisit, error) {
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	raw, ok := obj["drive_stop_list"]
	if !ok || raw == nil {
		return nil, nil
	}
	sl, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	var out []model.StopVisit
	for i, x := range sl {
		m, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("drive-stop JSON drive_stop_list row %d is not an object", i)
		}
		typ, _ := m["type"].(string)
		typ = strings.ToLower(strings.TrimSpace(typ))
		if typ != "" && typ != "stop" {
			continue
		}
		from, err1 := parseVisitTime(m["time_from"])
		to, err2 := parseVisitTime(m["time_to"])
		if err1 != nil || err2 != nil || from.IsZero() || to.IsZero() {
			continue
		}
		if to.Before(from) {
			from, to = to, from
		}
		v := model.StopVisit{From: from, To: to}
		if lat, lng, ok := visitLatLng(m); ok {
			v.Lat, v.Lng, v.HasPos = lat, lng, true
		}
		out = append(out, v)
	}
	return out, nil
}

func parseVisitTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return time.Time{}, fmt.Errorf("empty time")
		}
		for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if parsed, err := time.Parse(layout, s); err == nil {
				return parsed.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("visit time %q", s)
	default:
		return time.Time{}, fmt.Errorf("visit time type %T", v)
	}
}

// Live drive-stop rows use lat_lng_from / lat_lng_best_first (not first_valid_lat_lng).
func visitLatLng(m map[string]any) (lat, lng float64, ok bool) {
	for _, k := range []string{
		"lat_lng_best_first",
		"lat_lng_from",
		"first_valid_lat_lng",
		"lat_lng_best_last",
		"lat_lng_to",
		"last_valid_lat_lng",
	} {
		if lat, lng, ok := latLngOf(m[k]); ok {
			return lat, lng, true
		}
	}
	return latLngOf(m)
}

func latLngOf(v any) (lat, lng float64, ok bool) {
	m, isMap := v.(map[string]any)
	if !isMap {
		return 0, 0, false
	}
	lat, ok1 := asCoord(m["lat"])
	lng, ok2 := asCoord(m["lng"])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, false
	}
	if lat == 0 && lng == 0 {
		return 0, 0, false
	}
	return lat, lng, true
}

func asCoord(v any) (float64, bool) {
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
	default:
		return 0, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

func rowTripMiles(m map[string]any) (float64, bool) {
	if n, ok := measurementMiles(m["distance"]); ok {
		return n, true
	}
	if n, ok := measurementMiles(m["miles"]); ok {
		return n, true
	}
	for _, key := range []string{"miles", "distance", "distance_miles"} {
		v, exists := m[key]
		if !exists {
			continue
		}
		if n, ok := asFloat(v); ok {
			return n, true
		}
	}
	return 0, false
}

func measurementMiles(v any) (float64, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return 0, false
	}
	n, ok := asFloat(m["value"])
	if !ok {
		return 0, false
	}
	unit, _ := m["unit"].(string)
	miles, err := toMiles(n, unit)
	if err != nil {
		return 0, false
	}
	return miles, true
}

func toMiles(n float64, unit string) (float64, error) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "mi", "mile", "miles":
		return n, nil
	case "km", "kilometer", "kilometers":
		return n * 0.621371192237, nil
	case "m", "meter", "meters":
		return n / 1609.344, nil
	case "ft", "foot", "feet":
		return n / 5280, nil
	default:
		return 0, fmt.Errorf("unknown distance unit %q", unit)
	}
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

// GetPublic is the live probe / smoketest GET. oil.LastReading never calls it.
func (c *Client) GetPublic(ctx context.Context, path string, q url.Values) ([]byte, error) {
	return c.lockedGet(ctx, path, q)
}

// PostPublic is the live probe / smoketest POST. oil.LastReading never calls it.
func (c *Client) PostPublic(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.lockedPost(ctx, path, body)
}

func (c *Client) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve(path), rdr)
	if err != nil {
		return nil, safeHTTPError(path, "build request", err, c.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	q := url.Values{}
	var sentAuth string
	if c.PrivateKeyPEM != "" && c.Token != "" {
		tok, err := signAPIKeyJWT(c.PrivateKeyPEM, c.Token, time.Minute)
		if err != nil {
			return nil, safeHTTPError(path, "sign authentication", err, c.Token)
		}
		sentAuth = tok
		req.Header.Set("Authorization", "Bearer "+tok)
	} else if c.Token != "" {
		sentAuth = c.Token
		q.Set("api-key", c.Token)
		req.URL.RawQuery = q.Encode()
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	redirectSafeClient := *httpClient
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
		return nil, httpStatusError(path, res, b, 400, c.Token, sentAuth)
	}
	return b, nil
}

// get is the only HTTP in this package; oil.LastReading never calls it.
// With a PEM, auth is a per-request RS256 JWT (OneStep usage note). Without PEM, api-key query (Cursor guide).
func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(path), nil)
	if err != nil {
		return nil, safeHTTPError(path, "build request", err, c.Token)
	}
	var sentAuth string
	if c.PrivateKeyPEM != "" && c.Token != "" {
		tok, err := signAPIKeyJWT(c.PrivateKeyPEM, c.Token, time.Minute)
		if err != nil {
			return nil, safeHTTPError(path, "sign authentication", err, c.Token)
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
		// Never echo query strings (api-key) or Authorization material.
		return nil, httpStatusError(path, res, b, 240, c.Token, sentAuth)
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
		dead := strings.ToLower(pick(row, "dead", "retired"))
		d.Dead = dead == "1" || dead == "true" || dead == "yes" || dead == "dead"
		d.Active = !d.Dead
		if oil.HasLogisticsPersonnel(name) {
			d.LinkedCarEFleetsID = nil
		} else if eid != "" {
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

const vinLen = 17

func normalizeVIN(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// VINToEFleets maps a 17-char roster VIN onto exactly one efleets_id.
// Duplicate roster VINs are dropped so a collision cannot invent a join.
func VINToEFleets(cars []model.Car) map[string]string {
	count := map[string]int{}
	firstCar := map[string]string{}
	for _, c := range cars {
		v := normalizeVIN(c.VIN)
		if len(v) != vinLen {
			continue
		}
		count[v]++
		if _, ok := firstCar[v]; !ok {
			firstCar[v] = strings.TrimSpace(c.EFleetsID)
		}
	}
	out := map[string]string{}
	for v, n := range count {
		eid := firstCar[v]
		if n == 1 && eid != "" {
			out[v] = eid
		}
	}
	return out
}

// LinkByVIN attaches a car using exact 17-char VIN equality to cars.vin.
// Display_name and plate are not consulted. An existing factory_id link is kept.
func LinkByVIN(dev model.OneStepDevice, vinToCar map[string]string) model.OneStepDevice {
	if oil.HasLogisticsPersonnel(dev.DisplayName) {
		dev.LinkedCarEFleetsID = nil
		return dev
	}
	if dev.LinkedCarEFleetsID != nil && strings.TrimSpace(*dev.LinkedCarEFleetsID) != "" {
		return dev
	}
	v := normalizeVIN(dev.VIN)
	if len(v) != vinLen {
		return dev
	}
	if car, ok := vinToCar[v]; ok && car != "" {
		dev.LinkedCarEFleetsID = &car
	}
	return dev
}
