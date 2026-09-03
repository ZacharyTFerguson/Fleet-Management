package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

func cmdDevices(ctx context.Context, cfg config.Config, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: oilchange devices sync|list …")
		return model.ExitError
	}
	switch args[0] {
	case "sync":
		return cmdDevicesSync(ctx, cfg, args[1:])
	case "list":
		return cmdDevicesList(ctx, cfg, args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: oilchange devices sync|list …")
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
	_ = args
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
		active := "active"
		if d.Dead || !d.Active {
			active = "retired"
		}
		fmt.Printf("factory_id=%s device_id=%s efleets_id=%s status=%s display_name=%q\n",
			d.FactoryID, d.DeviceID, link, active, d.DisplayName)
	}
	return model.ExitOK
}
