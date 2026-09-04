package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

// cmdProbeOneStep GETs drive-stop with all three query params. It does not
// compute or write Last Reading.
func cmdProbeOneStep(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("probe-onestep", flag.ContinueOnError)
	ids := fs.String("device-id", "", "comma-separated OneStep device_id values (not factory_id, not display_name)")
	hours := fs.String("hours", "48", "comma-separated lookback windows ending now, e.g. 6,24,48")
	fromFlag := fs.String("from", "", "RFC3339 dt_tracker_from (optional; overrides --hours)")
	toFlag := fs.String("to", "", "RFC3339 dt_tracker_to (optional; default now)")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	deviceIDs := splitCSV(*ids)
	if len(deviceIDs) == 0 {
		fmt.Fprintln(os.Stderr, "probe-onestep: --device-id required")
		return model.ExitError
	}
	if cfg.OneStepToken == "" {
		fmt.Fprintln(os.Stderr, "probe-onestep: ONESTEP_API_KEY missing")
		return model.ExitError
	}
	client := onestep.NewClient(cfg.OneStepBase, cfg.OneStepToken)
	client.PrivateKeyPEM = cfg.OneStepPrivateKey

	windows, err := probeWindows(*hours, *fromFlag, *toFlag, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}

	fails := 0
	for _, id := range deviceIDs {
		for _, w := range windows {
			res, err := client.ProbeDriveStop(ctx, id, w.from, w.to)
			if err != nil {
				fails++
				fmt.Printf("device_id=%s from=%s to=%s auth=%s elapsed=%s error=%v\n",
					res.DeviceID, res.From.Format(time.RFC3339), res.To.Format(time.RFC3339),
					res.AuthMode, res.Elapsed.Round(time.Millisecond), err)
				continue
			}
			fmt.Printf("device_id=%s from=%s to=%s miles=%.4f auth=%s elapsed=%s\n",
				res.DeviceID, res.From.Format(time.RFC3339), res.To.Format(time.RFC3339),
				res.Miles, res.AuthMode, res.Elapsed.Round(time.Millisecond))
		}
	}
	if fails > 0 {
		return model.ExitError
	}
	return model.ExitOK
}

type probeWindow struct {
	from, to time.Time
}

func probeWindows(hoursCSV, fromStr, toStr string, now time.Time) ([]probeWindow, error) {
	now = now.UTC()
	if strings.TrimSpace(fromStr) != "" {
		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return nil, fmt.Errorf("probe-onestep: --from RFC3339: %w", err)
		}
		to := now
		if strings.TrimSpace(toStr) != "" {
			to, err = time.Parse(time.RFC3339, toStr)
			if err != nil {
				return nil, fmt.Errorf("probe-onestep: --to RFC3339: %w", err)
			}
		}
		return []probeWindow{{from: from.UTC(), to: to.UTC()}}, nil
	}
	var out []probeWindow
	for _, part := range splitCSV(hoursCSV) {
		h, err := strconv.Atoi(part)
		if err != nil || h <= 0 {
			return nil, fmt.Errorf("probe-onestep: --hours must be positive integers")
		}
		out = append(out, probeWindow{from: now.Add(-time.Duration(h) * time.Hour), to: now})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("probe-onestep: no windows")
	}
	return out, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
