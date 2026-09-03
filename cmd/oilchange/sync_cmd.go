package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"oilchange/internal/config"
	"oilchange/internal/model"
)

// cmdSyncSupabase pushes local SQLite to fleet Supabase (or the mock mirror).
// --interval keeps a durable ticker for throughout-the-day refreshes.
func cmdSyncSupabase(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	interval := fs.Duration("interval", 0, "repeat every duration (e.g. 5m); 0 = once")
	mirror := fs.String("mirror", defaultMirrorPath(), "local JSON mirror for the web UI (no secrets)")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	runOnce := func() int {
		a, done, err := openApp(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return model.ExitError
		}
		defer done()
		if _, err := a.SyncSupabase(ctx, *mirror); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return model.ExitError
		}
		return model.ExitOK
	}
	if code := runOnce(); code != model.ExitOK || *interval <= 0 {
		return code
	}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	fmt.Fprintf(os.Stderr, "sync ticker every %s (ctrl-c to stop)\n", interval.String())
	for {
		select {
		case <-ctx.Done():
			return model.ExitOK
		case <-ticker.C:
			if code := runOnce(); code != model.ExitOK {
				fmt.Fprintln(os.Stderr, "sync tick failed; will retry next interval")
			}
		}
	}
}

func defaultMirrorPath() string {
	if path := os.Getenv("FLEET_MIRROR_PATH"); path != "" {
		return path
	}
	return filepath.Join("web", "data", "cars.json")
}
