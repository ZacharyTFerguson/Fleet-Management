package onestep

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// NearAddressReportType is Generate Reports → Nearby Address.
const NearAddressReportType = "near_address"

// NearAddressRadiusMiles is the hunt radius the operator asked for.
const NearAddressRadiusMiles = 1.0

const (
	reportGeneratePath  = "/v3/api/public/report/generate"
	reportGeneratedPath = "/v3/api/public/report-generated"
)

// NearAddressQuery is one Nearby Address generate-reports job.
// Address is the station street string. From/To are the fill-day ±1 UTC bounds.
type NearAddressQuery struct {
	Address string
	From    time.Time
	To      time.Time
	Name    string
}

// NearAddressJob is the queued/done generate-reports row. Not Last Reading.
type NearAddressJob struct {
	ID             string
	Status         string
	Error          string
	OutputFilePath string
	Raw            []byte
}

// NearAddressHit is one device that stopped near the station.
// FactoryID is the join identity. Entity/display_name is a label only.
type NearAddressHit struct {
	FactoryID string
	DeviceID  string
	VIN       string
	Entity    string
	Address   string
	From      time.Time
	To        time.Time
	Miles     float64
	HasMiles  bool
	Lat, Lng  float64
	HasPos    bool
}

// GenerateNearAddress queues POST /v3/api/public/report/generate for near_address.
// HTTP is mutex-serialized. oil.LastReading never calls this.
func (c *Client) GenerateNearAddress(ctx context.Context, q NearAddressQuery) (NearAddressJob, error) {
	body, err := marshalNearAddressSpec(q)
	if err != nil {
		return NearAddressJob{}, err
	}
	b, err := c.lockedPost(ctx, reportGeneratePath, body)
	if err != nil {
		return NearAddressJob{}, err
	}
	job, err := parseReportJob(b)
	if err != nil {
		return NearAddressJob{}, err
	}
	if job.ID == "" {
		return job, fmt.Errorf("onestep %s: missing report_generated_id", reportGeneratePath)
	}
	return job, nil
}

// PollReportGenerated GETs one generate-reports job. Mutex-serialized per call.
func (c *Client) PollReportGenerated(ctx context.Context, id string) (NearAddressJob, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return NearAddressJob{}, fmt.Errorf("onestep %s: empty report_generated_id", reportGeneratedPath)
	}
	b, err := c.lockedGet(ctx, reportGeneratedPath+"/"+id, nil)
	if err != nil {
		return NearAddressJob{}, err
	}
	return parseReportJob(b)
}

// DownloadReportJSON tries public file routes for a finished job. Many accounts
// 404 these; the hunt then uses drive-stop rows with the same 1-mile window.
func (c *Client) DownloadReportJSON(ctx context.Context, job NearAddressJob) ([]byte, error) {
	id := strings.TrimSpace(job.ID)
	if id == "" {
		return nil, fmt.Errorf("onestep report download: empty report_generated_id")
	}
	type try struct {
		path string
		q    url.Values
	}
	var tries []try
	tries = append(tries, try{path: reportGeneratedPath + "/" + id + "/file", q: url.Values{"file_type": {"json"}}})
	tries = append(tries, try{path: reportGeneratedPath + "/" + id + "/file", q: url.Values{"report_file_type": {"json"}}})
	if p := strings.TrimSpace(job.OutputFilePath); p != "" {
		tries = append(tries, try{path: "/v3/api/public/file", q: url.Values{"path": {p}}})
		tries = append(tries, try{path: "/v3/api/public/webfile", q: url.Values{"path": {p}}})
		tries = append(tries, try{path: "/v3/api/public/files/download", q: url.Values{"path": {p}}})
	}
	var last error
	for _, t := range tries {
		b, err := c.lockedGet(ctx, t.path, t.q)
		if err != nil {
			last = err
			continue
		}
		if looksLikeReportJob(b) && !looksLikeNearAddressRows(b) {
			last = fmt.Errorf("onestep %s: job metadata, not row JSON", t.path)
			continue
		}
		return b, nil
	}
	if last == nil {
		last = fmt.Errorf("onestep report download: no file route")
	}
	return nil, last
}

var nearAddressPollEvery = 2 * time.Second
var nearAddressPollMax = 5 * time.Minute

// NearAddress generate + poll + parse. Download is best-effort; a missing public
// file route is returned as an error so callers can fall back to drive-stop.
func (c *Client) NearAddress(ctx context.Context, q NearAddressQuery) ([]NearAddressHit, NearAddressJob, error) {
	job, err := c.GenerateNearAddress(ctx, q)
	if err != nil {
		return nil, job, err
	}
	deadline := time.Now().Add(nearAddressPollMax)
	if c != nil && c.now != nil {
		deadline = c.now().Add(nearAddressPollMax)
	}
	for {
		if reportJobDone(job.Status) {
			break
		}
		if reportJobFailed(job.Status) {
			return nil, job, fmt.Errorf("onestep near_address: job %s %s %s", job.ID, job.Status, strings.TrimSpace(job.Error))
		}
		now := time.Now()
		if c != nil && c.now != nil {
			now = c.now()
		}
		if !now.Before(deadline) {
			return nil, job, fmt.Errorf("onestep near_address: job %s still %s after poll timeout", job.ID, job.Status)
		}
		if err := sleepCtx(ctx, nearAddressPollEvery); err != nil {
			return nil, job, err
		}
		job, err = c.PollReportGenerated(ctx, job.ID)
		if err != nil {
			return nil, job, err
		}
	}
	if job.Error != "" {
		return nil, job, fmt.Errorf("onestep near_address: job %s %s", job.ID, job.Error)
	}
	raw, err := c.DownloadReportJSON(ctx, job)
	if err != nil {
		return nil, job, err
	}
	hits, err := ParseNearAddressJSON(raw)
	return hits, job, err
}

func marshalNearAddressSpec(q NearAddressQuery) ([]byte, error) {
	addr := strings.TrimSpace(q.Address)
	if addr == "" {
		return nil, fmt.Errorf("near_address: station address required")
	}
	from := q.From.UTC()
	to := q.To.UTC()
	if from.IsZero() || to.IsZero() || !to.After(from) {
		return nil, fmt.Errorf("near_address: datetime_from/to required")
	}
	name := strings.TrimSpace(q.Name)
	if name == "" {
		name = "oilchange near_address"
	}
	spec := map[string]any{
		"user_report_name":         name,
		"report_type":              NearAddressReportType,
		"all_user_devices":         true,
		"exclude_inactive_devices": true,
		"datetime_from":            from.Format(time.RFC3339),
		"datetime_to":              to.Format("2006-01-02T15:04:05.000Z"),
		"time_zone":                "America/New_York",
		"report_file_type_list":    []string{"json"},
		"report_output_field_list": []string{
			"near_address_entity",
			"near_address_factory_id",
			"near_address_device_id",
			"near_address_vin",
			"stop_start_time",
			"stop_end_time",
			"stop_duration",
			"distance_from_location",
			"position",
			"address",
		},
		"report_options_near_address": map[string]any{
			"search_address_string": addr,
			"range": map[string]any{
				"value":   NearAddressRadiusMiles,
				"unit":    "mi",
				"display": "1 mi",
			},
		},
	}
	return json.Marshal(spec)
}

func parseReportJob(b []byte) (NearAddressJob, error) {
	var wrap struct {
		ID             string `json:"report_generated_id"`
		Status         string `json:"status"`
		Error          string `json:"error"`
		OutputFilePath string `json:"OutputFilePath"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return NearAddressJob{}, fmt.Errorf("onestep report-generated: %w", err)
	}
	return NearAddressJob{
		ID:             strings.TrimSpace(wrap.ID),
		Status:         strings.ToLower(strings.TrimSpace(wrap.Status)),
		Error:          strings.TrimSpace(wrap.Error),
		OutputFilePath: strings.TrimSpace(wrap.OutputFilePath),
		Raw:            append([]byte(nil), b...),
	}, nil
}

func reportJobDone(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "complete", "completed", "success":
		return true
	}
	return false
}

func reportJobFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "fail", "error", "cancelled", "canceled":
		return true
	}
	return false
}

func looksLikeReportJob(b []byte) bool {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	_, ok := m["report_generated_id"]
	return ok
}

func looksLikeNearAddressRows(b []byte) bool {
	hits, err := ParseNearAddressJSON(b)
	return err == nil && len(hits) > 0
}

// ParseNearAddressJSON reads generate-reports JSON rows. display_name/entity is not a join key.
func ParseNearAddressJSON(b []byte) ([]NearAddressHit, error) {
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("near_address JSON: %w", err)
	}
	var hits []NearAddressHit
	walkNearAddress(root, &hits)
	if hits == nil {
		hits = []NearAddressHit{}
	}
	return hits, nil
}

func walkNearAddress(v any, hits *[]NearAddressHit) {
	switch t := v.(type) {
	case map[string]any:
		if h, ok := hitFromMap(t); ok {
			*hits = append(*hits, h)
			return
		}
		for _, key := range []string{"result_list", "data", "rows", "records", "items", "report_data", "tables"} {
			if inner, ok := t[key]; ok {
				walkNearAddress(inner, hits)
			}
		}
	case []any:
		for _, x := range t {
			walkNearAddress(x, hits)
		}
	}
}

func hitFromMap(m map[string]any) (NearAddressHit, bool) {
	fid := strings.TrimSpace(firstMapString(m, "near_address_factory_id", "factory_id"))
	did := strings.TrimSpace(firstMapString(m, "near_address_device_id", "device_id"))
	if fid == "" && did == "" {
		return NearAddressHit{}, false
	}
	h := NearAddressHit{
		FactoryID: fid,
		DeviceID:  did,
		VIN:       normalizeVIN(firstMapString(m, "near_address_vin", "vin")),
		Entity:    firstMapString(m, "near_address_entity", "entity"),
		Address:   firstMapString(m, "address"),
	}
	if t, err := parseVisitTime(firstMapAny(m, "stop_start_time", "time_from")); err == nil {
		h.From = t
	}
	if t, err := parseVisitTime(firstMapAny(m, "stop_end_time", "time_to")); err == nil {
		h.To = t
	}
	if n, ok := measurementMiles(m["distance_from_location"]); ok {
		h.Miles, h.HasMiles = n, true
	} else if n, ok := asFloat(m["distance_from_location"]); ok {
		h.Miles, h.HasMiles = n, true
	}
	if lat, lng, ok := latLngOf(m["position"]); ok {
		h.Lat, h.Lng, h.HasPos = lat, lng, true
	} else if lat, lng, ok := latLngOf(m); ok {
		h.Lat, h.Lng, h.HasPos = lat, lng, true
	}
	return h, true
}

func firstMapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstMapAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func (c *Client) lockedPost(ctx context.Context, path string, body []byte) ([]byte, error) {
	if c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
	}
	b, err := c.post(ctx, path, body)
	if err != nil && c != nil && c.PrivateKeyPEM != "" && driveStopAuthFallback(err) {
		plain := &Client{Base: c.Base, Token: c.Token, HTTP: c.HTTP, now: c.now}
		return plain.post(ctx, path, body)
	}
	return b, err
}

func (c *Client) lockedGet(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
	}
	b, err := c.get(ctx, path, q)
	if err != nil && c != nil && c.PrivateKeyPEM != "" && driveStopAuthFallback(err) {
		plain := &Client{Base: c.Base, Token: c.Token, HTTP: c.HTTP, now: c.now}
		return plain.get(ctx, path, q)
	}
	return b, err
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
