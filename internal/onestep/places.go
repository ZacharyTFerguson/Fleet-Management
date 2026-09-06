package onestep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// PlaceProbe is one public-API path we tried. Bodies are never logged (may contain keys if echoed).
type PlaceProbe struct {
	Path       string `json:"path"`
	Method     string `json:"method"`
	Status     int    `json:"status,omitempty"`
	Bytes      int    `json:"bytes,omitempty"`
	OK         bool   `json:"ok"`
	Hint       string `json:"hint,omitempty"`
	Error      string `json:"error,omitempty"`
	ItemCount  int    `json:"item_count,omitempty"`
	SampleKeys string `json:"sample_keys,omitempty"`
}

// PlaceItem is a downloaded zone/marker/place. Gas-station pipeline only.
type PlaceItem struct {
	ID      string   `json:"id,omitempty"`
	Name    string   `json:"name,omitempty"`
	Address string   `json:"address,omitempty"`
	Lat     *float64 `json:"lat,omitempty"`
	Lng     *float64 `json:"lng,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Group   string   `json:"group,omitempty"`
	RawKind string   `json:"-"`
}

var placeListPaths = []string{
	"/v3/api/public/zone",
	"/v3/api/public/zones",
	"/v3/api/public/marker",
	"/v3/api/public/markers",
	"/v3/api/public/place",
	"/v3/api/public/places",
	"/v3/api/public/geofence",
	"/v3/api/public/geofences",
	"/v3/api/public/poi",
	"/v3/api/public/user-place",
	"/v3/api/public/user-places",
	"/v3/api/public/important-location",
}

var placeCreatePaths = []string{
	"/v3/api/public/marker",
	"/v3/api/public/markers",
	"/v3/api/public/place",
	"/v3/api/public/places",
}

var zoneCreatePaths = []string{
	"/v3/api/public/zone",
	"/v3/api/public/zones",
	"/v3/api/public/geofence",
	"/v3/api/public/geofences",
}

// DiscoverPlaces GETs candidate list endpoints. Used for docs + dry-run. Does not create.
func (c *Client) DiscoverPlaces(ctx context.Context) []PlaceProbe {
	var out []PlaceProbe
	for _, p := range placeListPaths {
		out = append(out, c.probePlaceGET(ctx, p))
	}
	return out
}

func (c *Client) probePlaceGET(ctx context.Context, path string) PlaceProbe {
	pr := PlaceProbe{Path: path, Method: "GET"}
	q := url.Values{}
	q.Set("limit", "50")
	b, err := c.lockedGet(ctx, path, q)
	if err != nil {
		pr.Error = sanitizeAuthError(err.Error(), c.Token)
		var se *StatusError
		if errors.As(err, &se) && se != nil {
			pr.Status = se.StatusCode
		} else if i := strings.Index(err.Error(), "HTTP "); i >= 0 {
			fmt.Sscanf(err.Error()[i:], "HTTP %d", &pr.Status)
		}
		switch pr.Status {
		case 401, 403:
			pr.Hint = "auth or scope — key may lack places/zones"
		case 404:
			pr.Hint = "path not on this account / public API"
		case 405:
			pr.Hint = "method not allowed — GET list may not exist"
		default:
			if pr.Status == 0 {
				pr.Hint = "transport or parse error (redacted)"
			}
		}
		return pr
	}
	pr.OK = true
	pr.Status = 200
	pr.Bytes = len(b)
	items, keys := parsePlaceList(b)
	pr.ItemCount = len(items)
	pr.SampleKeys = keys
	if pr.ItemCount == 0 {
		pr.Hint = "200 with empty or unrecognized list shape"
	} else {
		pr.Hint = "list parsed"
	}
	return pr
}

func parsePlaceList(b []byte) ([]PlaceItem, string) {
	var wrap map[string]any
	if err := json.Unmarshal(b, &wrap); err != nil {
		var arr []map[string]any
		if err2 := json.Unmarshal(b, &arr); err2 != nil {
			return nil, ""
		}
		return mapsToPlaces(arr), keysOfFirst(arr)
	}
	for _, k := range []string{"result_list", "zones", "markers", "places", "geofences", "data", "result", "items"} {
		if v, ok := wrap[k]; ok {
			if sl, ok := v.([]any); ok {
				var maps []map[string]any
				for _, x := range sl {
					if m, ok := x.(map[string]any); ok {
						maps = append(maps, m)
					}
				}
				return mapsToPlaces(maps), keysOfFirst(maps)
			}
		}
	}
	if looksLikePlace(wrap) {
		return mapsToPlaces([]map[string]any{wrap}), keysOfMap(wrap)
	}
	return nil, keysOfMap(wrap)
}

func looksLikePlace(m map[string]any) bool {
	_, hasName := m["name"]
	_, hasID := m["id"]
	_, hasZone := m["zone_id"]
	return (hasName && hasID) || hasZone
}

func mapsToPlaces(rows []map[string]any) []PlaceItem {
	var out []PlaceItem
	for _, m := range rows {
		it := PlaceItem{
			ID:      strAny(m["id"], m["zone_id"], m["marker_id"], m["place_id"]),
			Name:    strAny(m["name"], m["display_name"], m["label"]),
			Address: strAny(m["address"], m["street_address"]),
			Kind:    strAny(m["type"], m["kind"], m["category"]),
			Group:   strAny(m["group"], m["group_name"], m["folder"]),
		}
		if lat, ok := floatAny(m["lat"], m["latitude"]); ok {
			it.Lat = &lat
		}
		if lng, ok := floatAny(m["lng"], m["lon"], m["longitude"]); ok {
			it.Lng = &lng
		}
		if it.Lat == nil {
			if loc, ok := m["location"].(map[string]any); ok {
				if lat, ok := floatAny(loc["lat"], loc["latitude"]); ok {
					it.Lat = &lat
				}
				if lng, ok := floatAny(loc["lng"], loc["lon"], loc["longitude"]); ok {
					it.Lng = &lng
				}
			}
		}
		out = append(out, it)
	}
	return out
}

func strAny(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func floatAny(vals ...any) (float64, bool) {
	for _, v := range vals {
		switch n := v.(type) {
		case float64:
			return n, true
		case json.Number:
			f, err := n.Float64()
			return f, err == nil
		}
	}
	return 0, false
}

func keysOfFirst(rows []map[string]any) string {
	if len(rows) == 0 {
		return ""
	}
	return keysOfMap(rows[0])
}

func keysOfMap(m map[string]any) string {
	var keys []string
	for k := range m {
		if len(keys) >= 12 {
			break
		}
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}

// ListPlaces returns items from the first list path that parses rows.
func (c *Client) ListPlaces(ctx context.Context) (items []PlaceItem, path string, probes []PlaceProbe, err error) {
	probes = c.DiscoverPlaces(ctx)
	for _, p := range probes {
		if !p.OK {
			continue
		}
		q := url.Values{}
		q.Set("limit", "500")
		b, e := c.lockedGet(ctx, p.Path, q)
		if e != nil {
			continue
		}
		got, _ := parsePlaceList(b)
		if len(got) == 0 {
			continue
		}
		return got, p.Path, probes, nil
	}
	return nil, "", probes, fmt.Errorf("no OneStep public list path returned places/zones/markers")
}

// MarkerPayload is the dry-run / send body. Name must be the Canon Place label.
type MarkerPayload struct {
	Name     string  `json:"name"`
	Address  string  `json:"address,omitempty"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Group    string  `json:"group"`
	Type     string  `json:"type"`
	RadiusM  int     `json:"radius_m,omitempty"`
	Color    string  `json:"color,omitempty"`
}

func (p MarkerPayload) ValidateGas() error {
	if p.Group != "Gas_Stations" {
		return fmt.Errorf("refusing non-Gas_Stations group %q", p.Group)
	}
	if p.Type != "" && p.Type != "gas" && p.Type != "001" && p.Type != "gas_station" {
		return fmt.Errorf("refusing non-gas type %q", p.Type)
	}
	if p.Name == "" || p.Lat == 0 && p.Lng == 0 {
		return fmt.Errorf("name and lat/lng required")
	}
	if !strings.Contains(p.Name, "_001_") {
		return fmt.Errorf("Canon label must include type 001 (gas): %s", p.Name)
	}
	return nil
}

// DryRunMarker validates and reports which create path would be tried. No POST.
func (c *Client) DryRunMarker(ctx context.Context, p MarkerPayload) (map[string]any, error) {
	if err := p.ValidateGas(); err != nil {
		return nil, err
	}
	probes := c.DiscoverPlaces(ctx)
	out := map[string]any{
		"dry_run":     true,
		"would_send":  p,
		"list_probes": probes,
		"create_try":  placeCreatePaths,
		"zone_try":    zoneCreatePaths,
		"note":        "No POST was sent. Confirm SEND_TO_ONESTEP on a reviewed job to create one gas-station marker.",
	}
	return out, nil
}

// CreateMarker POSTs one gas-station marker. Caller must have passed the confirm gate.
func (c *Client) CreateMarker(ctx context.Context, p MarkerPayload) (id string, path string, raw []byte, err error) {
	if err := p.ValidateGas(); err != nil {
		return "", "", nil, err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return "", "", nil, err
	}
	var last error
	for _, path := range placeCreatePaths {
		b, e := c.lockedPost(ctx, path, body)
		if e != nil {
			last = e
			continue
		}
		id := extractID(b)
		return id, path, b, nil
	}
	if last == nil {
		last = fmt.Errorf("no create path accepted a marker POST")
	}
	return "", "", nil, last
}

// CreateZoneNear POSTs a small circle at the marker (canopy/pad). Gas Stations only.
func (c *Client) CreateZoneNear(ctx context.Context, name string, lat, lng float64, radiusM int) (id string, path string, err error) {
	if !strings.Contains(name, "_001_") {
		return "", "", fmt.Errorf("zone name must be a gas Canon label (type 001)")
	}
	if radiusM <= 0 {
		radiusM = 25
	}
	payload := map[string]any{
		"name":    name,
		"group":   "Gas_Stations",
		"type":    "gas",
		"lat":     lat,
		"lng":     lng,
		"radius":  radiusM,
		"shape":   "circle",
		"prefer":  "canopy_pad",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	var last error
	for _, path := range zoneCreatePaths {
		b, e := c.lockedPost(ctx, path, body)
		if e != nil {
			last = e
			continue
		}
		return extractID(b), path, nil
	}
	if last == nil {
		last = fmt.Errorf("no zone create path accepted POST — portal draw may be required")
	}
	return "", "", last
}

func extractID(b []byte) string {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return ""
	}
	return strAny(m["id"], m["zone_id"], m["marker_id"], m["place_id"])
}

// FindByName matches a downloaded place to a Canon label (exact, case-insensitive).
func FindByName(items []PlaceItem, name string) *PlaceItem {
	want := strings.ToLower(strings.TrimSpace(name))
	for i := range items {
		if strings.ToLower(items[i].Name) == want {
			return &items[i]
		}
	}
	return nil
}

// CompactJSON is for draft storage. Secrets must not be in the payload.
func CompactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return string(b)
	}
	return buf.String()
}
