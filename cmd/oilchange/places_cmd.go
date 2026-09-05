package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/places"
)

// cmdPlacesCache copies the existing Neon / JSON Canon catalog into sqlite.
// It does not mint general_codes or rewrite OneStep locks.
func cmdPlacesCache(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("places-cache", flag.ContinueOnError)
	jsonPath := fs.String("json", getenvDefault("OILCHANGE_PLACES_JSON", ""), "existing catalog JSON (no new codes)")
	fromNeon := fs.Bool("from-neon", false, "SELECT Fleet_Manage_Oil.places over DATABASE_URL (read-only)")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "oilchange places-cache: %v\n", err)
		return model.ExitError
	}
	defer done()

	neonURL := ""
	if *fromNeon || (*jsonPath == "" && strings.TrimSpace(cfg.DatabaseURL) != "") {
		neonURL = cfg.DatabaseURL
	}
	if strings.TrimSpace(*jsonPath) == "" && neonURL == "" {
		got, err := places.Load(ctx, a.Store)
		if err != nil {
			fmt.Fprintf(os.Stderr, "oilchange places-cache: %v\n", err)
			return model.ExitError
		}
		fmt.Fprintf(os.Stderr, "places cache: %d rows (sqlite). Pass --json or --from-neon to copy the existing catalog.\n", got.Count)
		return model.ExitOK
	}

	got, err := places.Refresh(ctx, a.Store, *jsonPath, neonURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "oilchange places-cache: %v\n", err)
		return model.ExitError
	}
	fmt.Fprintf(os.Stderr, "places cache: copied %d existing Canon rows from %s (no codes minted)\n", got.Count, got.Source)
	return model.ExitOK
}
