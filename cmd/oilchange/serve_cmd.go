package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"oilchange/internal/app"
	"oilchange/internal/config"
	"oilchange/internal/desk"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
	"oilchange/internal/places"
)

// cmdServe hosts the static Oil Desk UI + /api/cars. No Node/npm required.
func cmdServe(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:4739", "listen address")
	webDir := fs.String("web-dir", "", "optional on-disk static export (default: embedded web/out)")
	mirror := fs.String("mirror", defaultMirrorPath(), "cars.json mirror for /api/cars")
	infoPath := fs.String("device-information", app.DefaultDeviceInformationPath(), "saved Device Information JSON for the desk apply button (no live /device GET)")
	appWin := fs.Bool("app", false, "open a Chrome/Edge window without browser chrome (oilchange desk)")
	start := fs.String("start", "/history/", "path opened by --app / desk")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	opts := desk.Options{
		Addr:                  *addr,
		WebDir:                *webDir,
		MirrorPath:            *mirror,
		CardsPath:             defaultCardsPath(),
		DeviceInformationPath: *infoPath,
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "oilchange serve: VIN-from-file API off (%v)\n", err)
	} else {
		defer done()
		opts.History = &desk.HistoryAPI{
			Board:    a.HistoryBoard,
			Assign:   a.AssignFill,
			Events:   a.Store.ListAssignmentEvents,
			Evidence: a.FillEvidence,
			Box:      a.BoxEvidence,
			Probe:    a.ProbeOneBox,
		}
		auth := "none"
		if cfg.OneStepToken != "" {
			c := onestepAuthMode(cfg)
			auth = c
		}
		placeCat := loadPlacesForServe(a)
		opts.Records = &desk.RecordsAPI{
			Fuel:        func(ctx context.Context) ([]model.CardTx, error) { return a.Store.ListCardTxs(ctx, "") },
			Maintenance: a.Store.ListAllShopROs,
			OilChanges:  a.Store.ListOilChanges,
			Devices:     a.Store.ListDevices,
			Miles:       a.Store.ListAllMilesSince,
			FuelSource:  getenvDefault("OILCHANGE_FUEL_SOURCE", "sqlite card_transactions (eFleets DETAILS file-drop)"),
			MaintSource: getenvDefault("OILCHANGE_MAINT_SOURCE", "sqlite shop_ros (file-drop Maintenance Detail)"),
			OneStepAuth: auth,
			Places:      placeCat,
		}
		opts.ApplyDeviceInformation = func(ctx context.Context) (desk.VINFromFileResult, error) {
			res, err := a.ApplyDeviceInformation(ctx, *infoPath)
			out := desk.VINFromFileResult{
				Path:               res.Path,
				Exists:             true,
				Parsed:             res.Parsed,
				Upserted:           res.Upserted,
				Asked:              res.Asked,
				Linked:             res.Linked,
				Already:            res.Already,
				NoVIN:              res.NoVIN,
				NoRoster:           res.NoRoster,
				SkippedExistingMap: res.SkippedSteal,
			}
			for _, l := range res.Links {
				out.Links = append(out.Links, desk.VINFromFileLink{
					FactoryID: l.FactoryID,
					DeviceID:  l.DeviceID,
					VIN:       l.VIN,
					EFleetsID: l.EFleetsID,
				})
			}
			return out, err
		}
	}
	if *appWin {
		go func() {
			path := *start
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			url := "http://" + opts.Addr + path
			if err := desk.WaitHTTP(url, 8*time.Second); err != nil {
				fmt.Fprintln(os.Stderr, "oilchange desk:", err)
				return
			}
			if err := desk.OpenAppWindow(url); err != nil {
				fmt.Fprintln(os.Stderr, "oilchange desk:", err)
				fmt.Fprintln(os.Stderr, "open that URL in Chrome/Edge, or on a phone: Share → Add to Home Screen")
			}
		}()
	}
	if err := desk.ListenAndServe(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}

func loadPlacesForServe(a *app.App) *places.Catalog {
	if a == nil || a.Store == nil {
		return places.NewCatalog(nil)
	}
	ctx := context.Background()
	got, err := places.Ensure(ctx, a.Store, strings.TrimSpace(os.Getenv("OILCHANGE_PLACES_JSON")), a.Cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "oilchange serve: places catalog: %v\n", err)
		if cached, loadErr := places.Load(ctx, a.Store); loadErr == nil {
			return cached.Catalog
		}
		return places.NewCatalog(nil)
	}
	fmt.Fprintf(os.Stderr, "oilchange serve: Canon places catalog %d rows (%s)\n", got.Count, got.Source)
	return got.Catalog
}

func getenvDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func onestepAuthMode(cfg config.Config) string {
	c := onestep.NewClient(cfg.OneStepBase, cfg.OneStepToken)
	c.PrivateKeyPEM = cfg.OneStepPrivateKey
	return c.AuthMode()
}
