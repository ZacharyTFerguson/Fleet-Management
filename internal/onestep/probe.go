package onestep

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ProbeResult is one drive-stop GET. It never writes Last Reading and never
// uses OneStep odometer fields (sumDriveStop ignores those).
type ProbeResult struct {
	DeviceID string
	From     time.Time
	To       time.Time
	Miles    float64
	AuthMode string
	Elapsed  time.Duration
}

// ProbeDriveStop is a live check: JWT/query as configured, all three params
// required. Empty query is rejected here so we do not hit the misleading 403.
func (c *Client) ProbeDriveStop(ctx context.Context, deviceID string, from, to time.Time) (ProbeResult, error) {
	out := ProbeResult{
		DeviceID: strings.TrimSpace(deviceID),
		From:     from.UTC(),
		To:       to.UTC(),
		AuthMode: "none",
	}
	if c != nil {
		out.AuthMode = c.AuthMode()
	}
	if out.DeviceID == "" {
		return out, fmt.Errorf("probe-onestep: device_id required")
	}
	if out.From.IsZero() || out.To.IsZero() {
		return out, fmt.Errorf("probe-onestep: dt_tracker_from and dt_tracker_to required")
	}
	if !out.To.After(out.From) {
		return out, fmt.Errorf("probe-onestep: dt_tracker_to must be after dt_tracker_from")
	}
	start := time.Now()
	n, err := c.fetchDriveStopWindow(ctx, out.DeviceID, out.From, out.To)
	out.Elapsed = time.Since(start)
	if err != nil {
		return out, err
	}
	out.Miles = n
	return out, nil
}
