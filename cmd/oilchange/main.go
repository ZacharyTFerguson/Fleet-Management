package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"oilchange/internal/app"
	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches subcommands so exit 2 (open HOLDs) is distinct from a crash.
func run(args []string) int {
	if len(args) < 1 {
		usage()
		return model.ExitError
	}
	cfg := config.Load()
	ctx := context.Background()
	switch args[0] {
	case "sync-enterprise":
		return cmdSyncEnterprise(ctx, cfg, args[1:])
	case "sync-onestep":
		return cmdSyncOneStep(ctx, cfg, args[1:])
	case "probe-onestep":
		return cmdProbeOneStep(ctx, cfg, args[1:])
	case "compute":
		return cmdCompute(ctx, cfg, args[1:])
	case "oil-done":
		return cmdOilDone(ctx, cfg, args[1:])
	case "report":
		return cmdReport(ctx, cfg, args[1:])
	case "holds":
		return cmdHolds(ctx, cfg, args[1:])
	case "cards":
		return cmdCards(ctx, cfg, args[1:])
	case "devices":
		return cmdDevices(ctx, cfg, args[1:])
	case "sync", "sync-supabase":
		return cmdSyncSupabase(ctx, cfg, args[1:])
	case "backup-neon", "backup":
		return cmdBackupNeon(ctx, cfg, args[1:])
	case "pull-supabase":
		return cmdPullSupabase(ctx, cfg, args[1:])
	case "serve":
		return cmdServe(args[1:])
	case "env":
		return cmdEnv(cfg)
	default:
		usage()
		return model.ExitError
	}
}

// usage is stderr so a mistaken invocation cannot look like a report CSV.
func usage() {
	fmt.Fprintf(os.Stderr, `oilchange — PDI fleet Last Reading

  oilchange sync-enterprise [--vehicles PATH --fuel-details PATH [--shop-ro PATH] [--mileage-history PATH]]
  oilchange sync-onestep [--map PATH]
  oilchange probe-onestep --device-id ID[,ID…] [--hours 6,24,48 | --from RFC3339 [--to RFC3339]]
  oilchange compute [--override-lower]
  oilchange oil-done --efleets-id ID --miles N --date YYYY-MM-DD [--location NAME]
  oilchange report [--interval 5000] [--due-within N] [--out PATH.csv]
  oilchange holds
  oilchange cards rebuild [--fuel-details PATH] [--no-gps]
  oilchange cards suspect
  oilchange cards trace --card ID [--window-days 2]
  oilchange cards pairings [--card ID]
  oilchange cards split [--card ID]
  oilchange cards call [--card ID] [--all]
  oilchange devices sync [--map PATH]
  oilchange devices list
  oilchange sync [--interval 5m] [--mirror web/data/cars.json] [--require-neon] [--no-remote]
  oilchange pull-supabase
  oilchange backup-neon
  oilchange serve [--addr 127.0.0.1:4739] [--mirror web/data/cars.json] [--web-dir PATH]
  oilchange env

Secrets: paste into oilchange.env (gitignored). See oilchange.env.example.
oilchange env prints which keys loaded; it never prints secret values.
Desktop UI: oilchange serve hosts embedded Oil Desk + /api/cars (no npm).
`)
}

// openApp opens sqlite or postgres from env; callers must Close via the returned func.
func openApp(cfg config.Config) (*app.App, func(), error) {
	st, err := app.OpenStore(cfg)
	if err != nil {
		return nil, nil, err
	}
	return &app.App{Cfg: cfg, Store: st}, func() { _ = st.Close() }, nil
}

// cmdSyncEnterprise loads eFleets files or live download. It must not compute Last Reading.
func cmdSyncEnterprise(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("sync-enterprise", flag.ContinueOnError)
	vehicles := fs.String("vehicles", "", "Fleet Summary CSV/xlsx")
	fuel := fs.String("fuel-details", "", "Fuel & Charging DETAILS CSV/xlsx")
	shop := fs.String("shop-ro", "", "Maintenance Detail CSV/xlsx")
	mileage := fs.String("mileage-history", "", "Mileage History (context only)")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	if err := a.SyncEnterprise(ctx, *vehicles, *fuel, *shop, *mileage); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}

// cmdSyncOneStep stores miles-since, never device odometer.
func cmdSyncOneStep(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("sync-onestep", flag.ContinueOnError)
	mapPath := fs.String("map", "", "optional factory_id,device_id,efleets_id CSV")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	var client *onestep.Client
	if cfg.OneStepToken != "" {
		client = onestep.NewClient(cfg.OneStepBase, cfg.OneStepToken)
		client.PrivateKeyPEM = cfg.OneStepPrivateKey
	}
	if err := a.SyncOneStep(ctx, *mapPath, client); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}

// cmdCompute is the only command that writes last_reading_*; HOLD skips that write.
func cmdCompute(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("compute", flag.ContinueOnError)
	override := fs.Bool("override-lower", false, "allow writing a lower Last Reading")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	code, err := a.Compute(ctx, *override)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return code
}

// cmdOilDone records last oil without touching Last Reading.
func cmdOilDone(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("oil-done", flag.ContinueOnError)
	id := fs.String("efleets-id", "", "eFleets vehicle id")
	miles := fs.Int("miles", 0, "odometer at oil change")
	date := fs.String("date", "", "YYYY-MM-DD")
	loc := fs.String("location", "", "shop name")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	if *id == "" || *miles <= 0 || *date == "" {
		fmt.Fprintln(os.Stderr, "--efleets-id --miles --date required")
		return model.ExitError
	}
	day, err := time.Parse("2006-01-02", *date)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	if err := a.OilDone(ctx, *id, *miles, day, *loc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}

// cmdReport prints last oil + last reading by eFleets ID and never remaining/due columns.
func cmdReport(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	interval := fs.Int("interval", model.DefaultInterval, "default interval miles")
	due := fs.Int("due-within", 0, "only cars whose remaining miles (in-app) are <= N")
	out := fs.String("out", "", "CSV path; stdout if empty")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	if err := a.Report(ctx, *interval, *due, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}

// cmdEnv reports which oilchange.env keys loaded. Values are never printed.
func cmdEnv(cfg config.Config) int {
	fmt.Println("oilchange.env (presence only; values not printed)")
	for _, line := range cfg.EnvReport() {
		fmt.Println(line)
	}
	c := onestep.NewClient(cfg.OneStepBase, cfg.OneStepToken)
	c.PrivateKeyPEM = cfg.OneStepPrivateKey
	fmt.Println("onestep auth:", c.AuthMode())
	return model.ExitOK
}

// cmdHolds lists open HOLDs so operators do not read stale last_reading_* as current odo.
func cmdHolds(ctx context.Context, cfg config.Config, args []string) int {
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	if err := a.Holds(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}

func cmdCards(ctx context.Context, cfg config.Config, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: oilchange cards rebuild|suspect|trace|pairings|split|call")
		return model.ExitError
	}
	fs := flag.NewFlagSet("cards", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	details := fs.String("fuel-details", "", "optional DETAILS CSV to ingest before rebuild")
	cardID := fs.String("card", "", "card id (trace/pairings/split/call)")
	window := fs.Int("window-days", 2, "station co-occurrence window in days (trace)")
	noGPS := fs.Bool("no-gps", false, "skip OneStep stop windows (use gps-stops.json cache)")
	disagree := fs.Bool("disagree", true, "cards call: only swipes where GPS name ≠ Enterprise Vehicle")
	allCalls := fs.Bool("all", false, "cards call: print every GPS-named swipe")
	if err := fs.Parse(args[1:]); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	a.CardsMirror = defaultCardsPath()
	if cfg.OneStepToken != "" && !*noGPS {
		c := onestep.NewClient(cfg.OneStepBase, cfg.OneStepToken)
		c.PrivateKeyPEM = cfg.OneStepPrivateKey
		a.OneStep = c
	}
	switch args[0] {
	case "rebuild":
		n, err := a.CardsRebuild(ctx, *details)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cards rebuild: %v\n", err)
			return model.ExitError
		}
		fmt.Fprintf(os.Stdout, "cards rebuild: %d transactions rescored\n", n)
		if a.CardsMirror != "" {
			fmt.Fprintf(os.Stdout, "cards snapshot %s\n", a.CardsMirror)
		}
		return model.ExitOK
	case "suspect":
		if err := a.CardsSuspects(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "cards suspect: %v\n", err)
			return model.ExitError
		}
		return model.ExitOK
	case "trace":
		if *cardID == "" {
			fmt.Fprintln(os.Stderr, "cards trace: --card is required")
			return model.ExitError
		}
		if err := a.CardsTrace(ctx, *cardID, *window); err != nil {
			fmt.Fprintf(os.Stderr, "cards trace: %v\n", err)
			return model.ExitError
		}
		return model.ExitOK
	case "pairings":
		if err := a.CardsPairings(ctx, *cardID); err != nil {
			fmt.Fprintf(os.Stderr, "cards pairings: %v\n", err)
			return model.ExitError
		}
		return model.ExitOK
	case "split":
		if err := a.CardsSplit(ctx, *cardID); err != nil {
			fmt.Fprintf(os.Stderr, "cards split: %v\n", err)
			return model.ExitError
		}
		return model.ExitOK
	case "call":
		onlyDisagree := *disagree && !*allCalls
		if err := a.CardsCall(ctx, *cardID, onlyDisagree); err != nil {
			fmt.Fprintf(os.Stderr, "cards call: %v\n", err)
			return model.ExitError
		}
		return model.ExitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown cards verb %q\n", args[0])
		return model.ExitError
	}
}
