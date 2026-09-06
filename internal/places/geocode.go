package places

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GeocodeResult is a third-party check. It does not invent OneStep miles.
type GeocodeResult struct {
	Provider string  `json:"provider"`
	Query    string  `json:"query"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Label    string  `json:"label,omitempty"`
	MapURL   string  `json:"map_url"`
}

// Geocode looks up an address. Default is OSM Nominatim (no key).
// Mapbox / Google run only when a key is supplied from the server vault — never from the client.
func Geocode(ctx context.Context, provider, key, address string) (GeocodeResult, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return GeocodeResult{}, fmt.Errorf("address required")
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" || p == "nominatim" {
		return geocodeNominatim(ctx, address)
	}
	if key == "" {
		return GeocodeResult{}, fmt.Errorf("%s needs geocoder_api_key on the Secrets page", p)
	}
	switch p {
	case "mapbox":
		return geocodeMapbox(ctx, key, address)
	case "google":
		return geocodeGoogle(ctx, key, address)
	default:
		return GeocodeResult{}, fmt.Errorf("unknown geocoder %q (nominatim|mapbox|google)", p)
	}
}

func OSMEmbed(lat, lng float64) string {
	d := 0.008
	return fmt.Sprintf(
		"https://www.openstreetmap.org/export/embed.html?bbox=%f,%f,%f,%f&layer=mapnik&marker=%f,%f",
		lng-d, lat-d, lng+d, lat+d, lat, lng,
	)
}

func geocodeNominatim(ctx context.Context, address string) (GeocodeResult, error) {
	u := "https://nominatim.openstreetmap.org/search?format=jsonv2&limit=1&q=" + url.QueryEscape(address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return GeocodeResult{}, err
	}
	req.Header.Set("User-Agent", "oilchange-fleet-desk/1.0 (gas-station review; no secrets)")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return GeocodeResult{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return GeocodeResult{}, err
	}
	if res.StatusCode >= 300 {
		return GeocodeResult{}, fmt.Errorf("nominatim HTTP %d", res.StatusCode)
	}
	var rows []struct {
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return GeocodeResult{}, err
	}
	if len(rows) == 0 {
		return GeocodeResult{}, fmt.Errorf("nominatim: no result")
	}
	var lat, lng float64
	if _, err := fmt.Sscanf(rows[0].Lat, "%f", &lat); err != nil {
		return GeocodeResult{}, err
	}
	if _, err := fmt.Sscanf(rows[0].Lon, "%f", &lng); err != nil {
		return GeocodeResult{}, err
	}
	return GeocodeResult{
		Provider: "nominatim",
		Query:    address,
		Lat:      lat,
		Lng:      lng,
		Label:    rows[0].DisplayName,
		MapURL:   OSMEmbed(lat, lng),
	}, nil
}

func geocodeMapbox(ctx context.Context, key, address string) (GeocodeResult, error) {
	u := "https://api.mapbox.com/geocoding/v5/mapbox.places/" + url.PathEscape(address) + ".json?limit=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return GeocodeResult{}, err
	}
	q := req.URL.Query()
	q.Set("access_token", key)
	req.URL.RawQuery = q.Encode()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return GeocodeResult{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return GeocodeResult{}, err
	}
	if res.StatusCode >= 300 {
		return GeocodeResult{}, fmt.Errorf("mapbox HTTP %d", res.StatusCode)
	}
	var wrap struct {
		Features []struct {
			PlaceName string     `json:"place_name"`
			Center    []float64  `json:"center"`
		} `json:"features"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return GeocodeResult{}, err
	}
	if len(wrap.Features) == 0 || len(wrap.Features[0].Center) < 2 {
		return GeocodeResult{}, fmt.Errorf("mapbox: no result")
	}
	lng, lat := wrap.Features[0].Center[0], wrap.Features[0].Center[1]
	return GeocodeResult{Provider: "mapbox", Query: address, Lat: lat, Lng: lng, Label: wrap.Features[0].PlaceName, MapURL: OSMEmbed(lat, lng)}, nil
}

func geocodeGoogle(ctx context.Context, key, address string) (GeocodeResult, error) {
	u := "https://maps.googleapis.com/maps/api/geocode/json?address=" + url.QueryEscape(address)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return GeocodeResult{}, err
	}
	q := req.URL.Query()
	q.Set("key", key)
	req.URL.RawQuery = q.Encode()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return GeocodeResult{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return GeocodeResult{}, err
	}
	if res.StatusCode >= 300 {
		return GeocodeResult{}, fmt.Errorf("google HTTP %d", res.StatusCode)
	}
	var wrap struct {
		Results []struct {
			Formatted string `json:"formatted_address"`
			Geometry  struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return GeocodeResult{}, err
	}
	if wrap.Status != "OK" || len(wrap.Results) == 0 {
		return GeocodeResult{}, fmt.Errorf("google: %s", wrap.Status)
	}
	lat := wrap.Results[0].Geometry.Location.Lat
	lng := wrap.Results[0].Geometry.Location.Lng
	return GeocodeResult{Provider: "google", Query: address, Lat: lat, Lng: lng, Label: wrap.Results[0].Formatted, MapURL: OSMEmbed(lat, lng)}, nil
}

func init() {
	http.DefaultClient.Timeout = 20 * time.Second
}
