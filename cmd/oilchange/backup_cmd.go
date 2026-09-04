package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"oilchange/internal/config"
	"oilchange/internal/model"
)

// cmdBackupNeon copies sqlite → Neon. It does not compute Last Reading.
func cmdBackupNeon(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("backup-neon", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	counts, err := a.BackupNeon(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	fmt.Println(counts.LogLine())
	return model.ExitOK
}
