package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"oilchange/internal/config"
	"oilchange/internal/model"
)

// cmdBoxScore prints gas-card vs maintenance+drive-stop. Does not write Last Reading.
func cmdBoxScore(ctx context.Context, cfg config.Config, args []string) int {
	fs := flag.NewFlagSet("boxscore", flag.ContinueOnError)
	rebuild := fs.Bool("rebuild", false, "recompute from stored drive-stop windows (never invents miles)")
	id := fs.String("efleets-id", "", "limit printed rows to one car")
	if err := fs.Parse(args); err != nil {
		return model.ExitError
	}
	a, done, err := openApp(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	defer done()
	out, err := a.ListBoxScore(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return model.ExitError
	}
	if *rebuild {
		out, err = a.RebuildBoxScore(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return model.ExitError
		}
	}
	fmt.Printf("boxscore trusted=%d suspect=%d hold=%d overage=%d shortage=%d trend_up=%d trend_down=%d trend_flat=%d\n",
		out.TrustedN, out.SuspectN, out.HoldN, out.SumOverage, out.SumShortage, out.TrendUp, out.TrendDown, out.TrendFlat)
	want := *id
	for _, v := range out.Vehicles {
		if want != "" && v.EFleetsID != want {
			continue
		}
		for _, r := range v.Rows {
			exp := "-"
			if r.Expected != nil {
				exp = fmt.Sprintf("%d", *r.Expected)
			}
			abs := "-"
			if r.AbsDiff != nil {
				abs = fmt.Sprintf("%d", *r.AbsDiff)
			}
			diff := "-"
			if d := r.SignedDifference(); d != nil {
				diff = fmt.Sprintf("%d", *d)
			}
			miles := "-"
			if r.MilesSince != nil {
				miles = fmt.Sprintf("%.1f", *r.MilesSince)
			}
			fmt.Printf("%s %s gas_card_tx=%d exp=%s diff=%s over=%d short=%d abs=%s miles=%s trend=%s status=%s %s\n",
				r.EFleetsID, r.PunchAt.UTC().Format("2006-01-02T15:04:05Z"), r.Recorded, exp, diff,
				r.Overage, r.Shortage, abs, miles, r.Trend, r.Status, r.HoldDetail)
		}
	}
	return model.ExitOK
}
