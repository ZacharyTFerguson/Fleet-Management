package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

func cmdDevices(ctx context.Context, cfg config.Config, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: oilchange devices sync|list|csv …")
		return model.ExitError
	}
	switch args[0] {
	case "sync":
		return cmdDevicesSync(ctx, cfg, args[1:])
	case "list":
		return cmdDevicesList(ctx, cfg, args[1:])
	case "csv":
		return cmdDevicesCSV(ctx, cfg, args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: oilchange devices sync|list|csv …")
		return model.ExitError
	}
}

func cmdDevicesSync(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("devices sync", flag.ContinueOnError)
	mapPath := fs.String("map", "", "factory_id,device_id,efleets_id[,display_name,dead] CSV")
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
	n, err := a.SyncDevices(ctx, *mapPath, client)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	fmt.Printf("devices sync: upserted %d by factory_id\n", n)
	return model.ExitOK
}

func cmdDevicesList(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("devices list", flag.ContinueOnError)
	asCSV := fs.Bool("csv", false, "write factory_id CSV (display_name is a label only)")
	out := fs.String("out", "", "CSV path; stdout if empty")
	live := fs.Bool("live", false, "refresh from OneStep if token present (no miles)")
	mapPath := fs.String("map", "", "optional factory_id,device_id,efleets_id CSV used with --live")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	if *asCSV {
		return writeDevicesCSV(ctx, cfg, *out, *live, *mapPath)
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	devs, err := a.Store.ListDevices(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	for _, d := range devs {
		link := ""
		if d.LinkedCarEFleetsID != nil {
			link = *d.LinkedCarEFleetsID
		}
		fmt.Printf("factory_id=%s device_id=%s efleets_id=%s status=%s display_name=%q\n",
			d.FactoryID, d.DeviceID, link, onestep.DeviceStatus(d), d.DisplayName)
	}
	return model.ExitOK
}

func cmdDevicesCSV(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("devices csv", flag.ContinueOnError)
	out := fs.String("out", "", "CSV path; stdout if empty (data/runtime is gitignored)")
	live := fs.Bool("live", false, "refresh from OneStep if token present (no miles)")
	mapPath := fs.String("map", "", "optional factory_id,device_id,efleets_id CSV used with --live")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	return writeDevicesCSV(ctx, cfg, *out, *live, *mapPath)
}

func writeDevicesCSV(ctx context.Context, cfg config.Config, outPath string, live bool, mapPath string) int {
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	var client *onestep.Client
	if live && cfg.OneStepToken != "" {
		client = onestep.NewClient(cfg.OneStepBase, cfg.OneStepToken)
		client.PrivateKeyPEM = cfg.OneStepPrivateKey
	}
	var w io.Writer = os.Stdout
	if outPath != "" {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return model.ExitError
		}
		f, err := os.Create(outPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return model.ExitError
		}
		defer f.Close()
		w = f
	}
	n, err := a.DevicesCSV(ctx, w, live, mapPath, client)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	if outPath != "" {
		fmt.Fprintf(os.Stderr, "devices csv: %d rows -> %s\n", n, outPath)
	}
	return model.ExitOK
}
