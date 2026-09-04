package main

import (
	"flag"
	"fmt"
	"os"

	"oilchange/internal/desk"
	"oilchange/internal/model"
)

// cmdServe hosts the static Oil Desk UI + /api/cars. No Node/npm required.
func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:4739", "listen address")
	webDir := fs.String("web-dir", "", "optional on-disk static export (default: embedded web/out)")
	mirror := fs.String("mirror", defaultMirrorPath(), "cars.json mirror for /api/cars")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	opts := desk.Options{
		Addr:       *addr,
		WebDir:     *webDir,
		MirrorPath: *mirror,
		CardsPath:  defaultCardsPath(),
	}
	if err := desk.ListenAndServe(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	return model.ExitOK
}
