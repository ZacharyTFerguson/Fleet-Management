package syncsupabase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"oilchange/internal/model"
)

// ZacharyProjectHost is the only allowed Supabase host for fleet oil (never XRAY).
const ZacharyProjectHost = "hdtwfdjdvdzdxfdriyzn.supabase.co"

const DevicesTable = "fleet_onestep_devices"

const carsSelect = "pdi_id,efleets_id,nickname,plate,vin,region,last_oil_miles,last_oil_date,last_reading_miles,last_reading_at,last_reading_source,hold_reason,interval_miles"

// DeviceRow is a dumb copy of fleet_onestep_devices (factory_id PK; display_name is label only).
type DeviceRow struct {
	FactoryID          string  `json:"factory_id"`
	DeviceID           string  `json:"device_id"`
	DisplayName        string  `json:"display_name"`
	LinkedCarEFleetsID *string `json:"linked_car_efleets_id"`
	LinkedCarPDIID     *string `json:"linked_car_pdi_id"`
	Dead               bool    `json:"dead"`
	Active             bool    `json:"active"`
}

// checkPullURL refuses XRAY and any supabase.co host that is not Zachary's project.
func checkPullURL(raw string) error {
	if err := refuseXRAY(raw); err != nil {
		return err
	}
	lu := strings.ToLower(raw)
	if strings.Contains(lu, "supabase.") && !strings.Contains(lu, ZacharyProjectHost) {
		return fmt.Errorf("SUPABASE_URL must be ZacharyTFerguson's Project")
	}
	return nil
}

func selectKey(cfg Config) (key string, kind string, err error) {
	if strings.TrimSpace(cfg.ServiceRole) != "" {
		return cfg.ServiceRole, "service-role", nil
	}
	if strings.TrimSpace(cfg.AnonKey) != "" {
		return cfg.AnonKey, "anon", nil
	}
	return "", "", fmt.Errorf("set SUPABASE_GROK_BUILD_KEY (publishable) or SUPABASE_SERVICE_ROLE")
}

// FetchCars GETs fleet_cars. key is anon or service role. Never logs the key.
func FetchCars(ctx context.Context, cfg Config) ([]CarRow, error) {
	if err := checkPullURL(cfg.URL); err != nil {
		return nil, err
	}
	key, _, err := selectKey(cfg)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(cfg.URL, "/")
	q := url.Values{}
	q.Set("select", carsSelect)
	q.Set("order", "efleets_id")
	q.Set("limit", "1000")
	var rows []CarRow
	if err := getJSON(ctx, base+"/rest/v1/"+CarsTable+"?"+q.Encode(), key, &rows); err != nil {
		return nil, fmt.Errorf("%s: %w", CarsTable, err)
	}
	return rows, nil
}

// FetchDevices GETs fleet_onestep_devices. Caller must pass service role; anon is denied in schema.
func FetchDevices(ctx context.Context, cfg Config) ([]DeviceRow, error) {
	if err := checkPullURL(cfg.URL); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ServiceRole) == "" {
		return nil, fmt.Errorf("fleet_onestep_devices requires SUPABASE_SERVICE_ROLE")
	}
	base := strings.TrimRight(cfg.URL, "/")
	q := url.Values{}
	q.Set("select", "factory_id,device_id,display_name,linked_car_efleets_id,linked_car_pdi_id,dead,active")
	q.Set("order", "factory_id")
	q.Set("limit", "1000")
	var rows []DeviceRow
	if err := getJSON(ctx, base+"/rest/v1/"+DevicesTable+"?"+q.Encode(), cfg.ServiceRole, &rows); err != nil {
		return nil, fmt.Errorf("%s: %w", DevicesTable, err)
	}
	return rows, nil
}

func getJSON(ctx context.Context, rawURL, key string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return err
	}
	return nil
}

// CarFromRow maps a fleet_cars JSON row to the sqlite model. Dates stay nil when absent.
func CarFromRow(r CarRow) model.Car {
	c := model.Car{
		PDIID:             r.PDIID,
		EFleetsID:         r.EFleetsID,
		Nickname:          r.Nickname,
		Plate:             r.Plate,
		VIN:               r.VIN,
		Region:            r.Region,
		LastOilMiles:      r.LastOilMiles,
		LastReadingMiles:  r.LastReadingMiles,
		LastReadingSource: r.LastReadingSource,
		HoldReason:        r.HoldReason,
		IntervalMiles:     r.IntervalMiles,
	}
	c.LastOilDate = parseRowTime(r.LastOilDate)
	c.LastReadingAt = parseRowTime(r.LastReadingAt)
	return c
}

func parseRowTime(s *string) *time.Time {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, *s); err == nil {
			return &t
		}
	}
	return nil
}
