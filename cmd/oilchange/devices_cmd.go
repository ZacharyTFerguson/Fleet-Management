package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"oilchange/internal/app"
	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

func cmdDevices(ctx context.Context, cfg config.Config, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: oilchange devices sync|list|csv|vin …")
		return model.ExitError
	}
	switch args[0] {
	case "sync":
		return cmdDevicesSync(ctx, cfg, args[1:])
	case "list":
		return cmdDevicesList(ctx, cfg, args[1:])
	case "csv":
		return cmdDevicesCSV(ctx, cfg, args[1:])
	case "vin":
		return cmdDevicesVIN(ctx, cfg, args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: oilchange devices sync|list|csv|vin …")
		return model.ExitError
	}
}

func cmdDevicesSync(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("devices sync", flag.ContinueOnError)
	mapPath := fs.String("map", "", "factory_id,device_id,efleets_id[,display_name,dead] CSV")
	infoPath := fs.String("information", "", "saved Device Information JSON (no live /device GET)")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	if strings.TrimSpace(*infoPath) != "" {
		if strings.TrimSpace(*mapPath) != "" {
			if _, err := a.SyncDevices(ctx, *mapPath, nil); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return model.ExitError
			}
		}
		res, err := a.ApplyDeviceInformation(ctx, *infoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "devices sync --information: %v\n", err)
			return model.ExitError
		}
		fmt.Fprint(os.Stdout, res.Format())
		for _, l := range res.Links {
			fmt.Printf("LINK factory_id=%s device_id=%s vin=%s car=%s\n", l.FactoryID, l.DeviceID, l.VIN, l.EFleetsID)
		}
		return model.ExitOK
	}
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

func cmdDevicesVIN(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("devices vin", flag.ContinueOnError)
	factoryID := fs.String("factory-id", "", "limit to one factory_id (default: every unpaired box)")
	pace := fs.Duration("pace", 35*time.Second, "min interval between per-device VIN GETs (Retry-After wins)")
	from := fs.String("from", "", "saved Device Information JSON (no live /device GET). Empty --from uses data/runtime/device-information.json")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	fromSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "from" {
			fromSet = true
		}
	})
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	if fromSet {
		res, err := a.ApplyDeviceInformation(ctx, *from)
		if err != nil {
			fmt.Fprintf(os.Stderr, "devices vin --from: %v\n", err)
			return model.ExitError
		}
		fmt.Fprint(os.Stdout, res.Format())
		for _, l := range res.Links {
			fmt.Printf("LINK factory_id=%s device_id=%s vin=%s car=%s\n", l.FactoryID, l.DeviceID, l.VIN, l.EFleetsID)
		}
		return model.ExitOK
	}
	if cfg.OneStepToken != "" {
		c := onestep.NewClient(cfg.OneStepBase, cfg.OneStepToken)
		c.PrivateKeyPEM = cfg.OneStepPrivateKey
		a.OneStep = c
	}
	opt := app.PairVINOpts{AskEmpty: true, Pace: *pace}
	if strings.TrimSpace(*factoryID) != "" {
		opt.FactoryIDs = []string{*factoryID}
	}
	res, err := a.PairDevicesByVIN(ctx, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "devices vin: %v\n", err)
		return model.ExitError
	}
	fmt.Fprint(os.Stdout, res.Format())
	for _, l := range res.Links {
		fmt.Printf("LINK factory_id=%s device_id=%s vin=%s car=%s\n", l.FactoryID, l.DeviceID, l.VIN, l.EFleetsID)
	}
	return model.ExitOK
}
