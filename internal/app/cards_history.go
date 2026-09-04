package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"oilchange/internal/cards"
	"oilchange/internal/onestep"
)

// CardsHistoryOpts is the operator one-path: roster → devices → DETAILS → GPS rebuild → ladder eras → coverage.
// Live portals are used only when credentials are already in env — never prompts for a password.
type CardsHistoryOpts struct {
	VehiclesPath    string
	FuelDetailsPath string
	DevicesLive     bool
	DevicesMapPath  string
	DevicesOutPath  string
	NoGPS           bool
}

// CardsHistoryResult is the ladder + coverage after the full history finder run.
type CardsHistoryResult struct {
	DevicesN int
	TxN      int
	Ladder   cards.LadderResult
}

// CardsHistory runs the card/vehicle history finder end to end:
//  1. Fleet Summary roster (file-drop) so factory_id map can FK to cars
//  2. OneStep devices inventory (optional --live / --map) and CSV dump
//  3. Ask OneStep OBD VIN on unpaired boxes and join exact 17-char VIN to cars.vin
//  4. eFleets DETAILS ingest (file-drop or live when EFLEETS_* is in env)
//  5. GPS stop cache + cards rebuild (pairings + card_eras, preserve watch eras)
//  6. Station ladder 3 / 5 / 10 and coverage metrics
//
// Never writes Last Reading.
func (a *App) CardsHistory(ctx context.Context, opt CardsHistoryOpts) (CardsHistoryResult, error) {
	empty := CardsHistoryResult{}
	if a == nil || a.Store == nil {
		return empty, fmt.Errorf("cards history: no store")
	}
	res := CardsHistoryResult{}

	veh := strings.TrimSpace(opt.VehiclesPath)
	fuel := strings.TrimSpace(opt.FuelDetailsPath)
	if veh != "" {
		if err := a.SyncEnterprise(ctx, veh, "", "", ""); err != nil {
			return empty, fmt.Errorf("Fleet Summary ingest: %w", err)
		}
		fmt.Fprintln(os.Stderr, "history: ingested Fleet Summary roster")
	}

	if opt.DevicesLive || strings.TrimSpace(opt.DevicesMapPath) != "" {
		client := a.oneStepClient()
		if opt.DevicesLive && client == nil {
			fmt.Fprintln(os.Stderr, "history: devices --live skipped (no OneStep token)")
		}
		n, err := a.SyncDevices(ctx, opt.DevicesMapPath, client)
		if err != nil {
			return empty, fmt.Errorf("devices sync: %w", err)
		}
		res.DevicesN = n
		fmt.Fprintf(os.Stderr, "history: devices sync upserted %d by factory_id\n", n)
		a.OneStep = client
		if client != nil {
			pr, perr := a.PairDevicesByVIN(ctx, PairVINOpts{AskEmpty: true})
			if perr != nil {
				return empty, fmt.Errorf("devices VIN: %w", perr)
			}
			fmt.Fprintf(os.Stderr, "history: %s", pr.Format())
		}
	}

	// VIN-ask leftover unpaired boxes when a OneStep client is already
	// attached and the operator skipped --devices-live. `cards history
	// --no-gps` rematch-only does not attach a client (no surprise GETs).
	if !opt.DevicesLive && strings.TrimSpace(opt.DevicesMapPath) == "" && a.OneStep != nil {
		pr, perr := a.PairDevicesByVIN(ctx, PairVINOpts{AskEmpty: true})
		if perr != nil {
			return empty, fmt.Errorf("devices VIN: %w", perr)
		}
		fmt.Fprintf(os.Stderr, "history: %s", pr.Format())
	}

	outPath := strings.TrimSpace(opt.DevicesOutPath)
	if outPath == "" {
		outPath = filepath.Join("data", "runtime", "onestep-devices.csv")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return empty, err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return empty, err
	}
	n, err := a.DevicesCSV(ctx, f, false, "", nil)
	_ = f.Close()
	if err != nil {
		return empty, fmt.Errorf("devices csv: %w", err)
	}
	if res.DevicesN == 0 {
		res.DevicesN = n
	}
	fmt.Fprintf(os.Stderr, "history: devices csv %d rows -> %s\n", n, outPath)

	switch {
	case fuel != "":
		if err := a.SyncEnterprise(ctx, "", fuel, "", ""); err != nil {
			return empty, fmt.Errorf("DETAILS ingest: %w", err)
		}
		fmt.Fprintln(os.Stderr, "history: ingested enterprise file-drop")
	case veh == "" && strings.TrimSpace(a.Cfg.EFleetsUser) != "":
		if err := a.SyncEnterprise(ctx, "", "", "", ""); err != nil {
			return empty, fmt.Errorf("live eFleets ingest: %w", err)
		}
		fmt.Fprintln(os.Stderr, "history: live eFleets ingest (EFLEETS_* in env)")
	default:
		if fuel == "" {
			fmt.Fprintln(os.Stderr, "history: no DETAILS path and no EFLEETS_* — using sqlite card_transactions")
		}
	}

	if opt.NoGPS {
		one := a.OneStep
		a.OneStep = nil
		defer func() { a.OneStep = one }()
	} else if a.OneStep == nil {
		a.OneStep = a.oneStepClient()
	}

	txN, err := a.CardsRebuild(ctx, "")
	if err != nil {
		return empty, fmt.Errorf("cards rebuild: %w", err)
	}
	res.TxN = txN

	ladder, err := a.CardsLadder(ctx, false)
	if err != nil {
		return empty, err
	}
	res.Ladder = ladder
	return res, nil
}

func (a *App) oneStepClient() *onestep.Client {
	if a == nil {
		return nil
	}
	if a.OneStep != nil {
		return a.OneStep
	}
	if strings.TrimSpace(a.Cfg.OneStepToken) == "" {
		return nil
	}
	c := onestep.NewClient(a.Cfg.OneStepBase, a.Cfg.OneStepToken)
	c.PrivateKeyPEM = a.Cfg.OneStepPrivateKey
	return c
}
