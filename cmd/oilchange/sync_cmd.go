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

// cmdSyncSupabase pushes local SQLite → fleet-oil Supabase (or mock mirror).
// --interval keeps a durable ticker for "throughout the day" refresh.
func cmdSyncSupabase(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	interval := fs.Duration("interval", 0, "repeat every duration (e.g. 5m); 0 = once")
	mirror := fs.String("mirror", defaultMirrorPath(), "local JSON mirror for the web UI (no secrets)")
	requireNeon := fs.Bool("require-neon", false, "fail if Neon backup is unset or fails (default: best-effort)")
	noRemote := fs.Bool("no-remote", false, "refresh the JSON mirror only; do not upsert fleet_cars")
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
		if _, err := a.SyncSupabase(ctx, *mirror, *noRemote); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return model.ExitError
		}
		if cfg.DatabaseURL == "" {
			if *requireNeon {
				fmt.Fprintln(os.Stderr, "neon backup: set DATABASE_URL in oilchange.env to Neon unpooled")
				return model.ExitError
			}
			return model.ExitOK
		}
		counts, err := a.BackupNeon(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "neon backup: %v\n", err)
			if *requireNeon {
				return model.ExitError
			}
			fmt.Fprintln(os.Stderr, "neon backup skipped; sqlite + supabase/mirror still ok")
			return model.ExitOK
		}
		fmt.Println(counts.LogLine())
		return model.ExitOK
	}
	if code := runOnce(); code != model.ExitOK || *interval <= 0 {
		return code
	}
	t := time.NewTicker(*interval)
	defer t.Stop()
	fmt.Fprintf(os.Stderr, "sync ticker every %s (ctrl-c to stop)\n", interval.String())
	for {
		select {
		case <-ctx.Done():
			return model.ExitOK
		case <-t.C:
			if code := runOnce(); code != model.ExitOK {
				fmt.Fprintln(os.Stderr, "sync tick failed; will retry next interval")
			}
		}
	}
}

func defaultMirrorPath() string {
	if v := os.Getenv("FLEET_MIRROR_PATH"); v != "" {
		return v
	}
	return filepath.Join("web", "data", "cars.json")
}

func defaultCardsPath() string {
	if v := os.Getenv("FLEET_CARDS_PATH"); v != "" {
		return v
	}
	return filepath.Join(filepath.Dir(defaultMirrorPath()), "cards.json")
}
