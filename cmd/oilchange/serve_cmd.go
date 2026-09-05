package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"oilchange/internal/app"
	"oilchange/internal/config"
	"oilchange/internal/desk"
	"oilchange/internal/model"
)

// cmdServe hosts the static Oil Desk UI + /api/cars. No Node/npm required.
func cmdServe(cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:4739", "listen address")
	webDir := fs.String("web-dir", "", "optional on-disk static export (default: embedded web/out)")
	mirror := fs.String("mirror", defaultMirrorPath(), "cars.json mirror for /api/cars")
	infoPath := fs.String("device-information", app.DefaultDeviceInformationPath(), "saved Device Information JSON for the desk apply button (no live /device GET)")
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
	if err := desk.ListenAndServe(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}
