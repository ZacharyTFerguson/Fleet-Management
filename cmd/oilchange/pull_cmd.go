package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"oilchange/internal/config"
	"oilchange/internal/model"
)

// cmdPullSupabase copies fleet_cars into sqlite. It does not compute Last Reading.
func cmdPullSupabase(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("pull-supabase", flag.ContinueOnError)
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
	st, err := a.PullSupabase(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	fmt.Printf("pull-supabase cars=%d last_reading=%d last_oil=%d holds=%d pdi_mismatch=%d devices=%d %s\n",
		st.Cars, st.LastReading, st.LastOil, st.Holds, st.PDIMismatch, st.Devices, st.DevicesNote)
	return model.ExitOK
}
