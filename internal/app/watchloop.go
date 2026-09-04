package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/model"
	"oilchange/internal/oil"
	"oilchange/internal/onestep"
)

// CardsWatchOpts is the targeted unknown-card loop. Never Last Reading.
type CardsWatchOpts struct {
	CardID    string
	LiveStops bool
	Persist   bool
	Fills     int           // newest punches per card to fetch (default 10)
	Pace      time.Duration // min interval between drive-stop GETs (default 35s)
}

// watchDriveStopMinInterval is OneStep support's 15–30s routine cadence plus a
// little slack. The API's Retry-After header wins when it asks for longer.
var watchDriveStopMinInterval = 35 * time.Second

// CardsWatch loops unknown cards: newest 10 fills, watched boxes only.
// Virginia recorded vehicles seed which factory_id to ask about; GPS exclusive
// days still have to land before persist. Fleet-wide nearby --live is not used.
func (a *App) CardsWatch(ctx context.Context, opt CardsWatchOpts) (cards.NearbyResult, error) {
	empty := cards.NearbyResult{}
	maxFills := opt.Fills
	if maxFills <= 0 {
		maxFills = cards.DefaultWatchFills
	}
	pace := opt.Pace
	if pace <= 0 {
		pace = watchDriveStopMinInterval
	}

	txs, err := a.Store.ListCardTxs(ctx, strings.TrimSpace(opt.CardID))
	if err != nil {
		return empty, err
	}
	if len(txs) == 0 {
		return empty, nil
	}
	devs, err := a.Store.ListDevices(ctx)
	if err != nil {
		return empty, err
	}
	eras, err := a.Store.ListEras(ctx)
	if err != nil {
		return empty, err
	}
	fleet, err := a.Store.ListCars(ctx)
	if err != nil {
		return empty, err
	}

	savedOS := a.OneStep
	a.OneStep = nil
	gps, err := a.matchCardsAtGPSStops(ctx, txs)
	a.OneStep = savedOS
	if err != nil {
		return empty, err
	}
	txs = cards.ApplyCalls(txs, gps.Calls)
	txs = cards.EligibleUnknownFills(txs, eras)
	if opt.CardID != "" {
		txs = cards.FillsForCard(txs, opt.CardID)
	}
	if len(txs) == 0 {
		return empty, nil
	}

	liveOK := true
	var live []model.OneStepDevice
	if opt.LiveStops && a.OneStep != nil {
		var lerr error
		live, lerr = a.OneStep.ListDevices(ctx)
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "watch live devices: %v\n", lerr)
			liveOK = false
		} else {
			devs = mergeNearbyDevices(devs, live)
			pr, perr := a.PairDevicesByVIN(ctx, PairVINOpts{Live: live, AskEmpty: false})
			if perr != nil {
				fmt.Fprintf(os.Stderr, "watch VIN-pair from /device: %v\n", perr)
			} else if pr.Linked > 0 {
				fmt.Fprintf(os.Stderr, "watch %s", pr.Format())
				if stored, serr := a.Store.ListDevices(ctx); serr == nil {
					devs = overlayDeviceLinks(devs, stored)
				}
			}
		}
	}

	visits := loadNearbyVisits(a)
	if len(visits) > 0 {
		geo := cards.MatchGPSFirst(visits, txs, fleet, cards.DefaultStopSlack)
		if len(geo.Stations) > 0 {
			gps.Stations = geo.Stations
		}
	}
	prior := cards.HuntNearbyFull(visits, txs, gps.Stations, devs, cards.DefaultStopSlack, false)
	order := cards.WatchCardOrder(txs, prior, fleet)
	fmt.Fprintf(os.Stderr, "watch cards=%d fills_cap=%d live=%v persist=%v pace=%s (watched boxes only; not a fleet pull)\n",
		len(order), maxFills, opt.LiveStops && a.OneStep != nil, opt.Persist, pace)

	var merged cards.NearbyResult
	allComplete := true
	var lastCall time.Time
	for _, cardID := range order {
		allFills := cards.FillsForCard(txs, cardID)
		if len(allFills) == 0 {
			continue
		}
		batch := cards.WatchFillBatch(allFills, maxFills)
		watched := cards.SeedWatchedFactoryIDs(allFills, prior, devs, fleet)
		need := watchDevicesByFactory(devs, watched)
		needIDs := factoryIDsOf(need)
		rankIDs := cards.SeedPriorityFactoryIDs(allFills, devs, fleet)
		if len(rankIDs) == 0 {
			rankIDs = needIDs
		}
		from, to := cards.UnionFillDayWindow(batch)
		cardComplete := liveOK && cards.WatchedCoverageComplete(visits, rankIDs, from, to)
		if opt.LiveStops && a.OneStep != nil && len(need) > 0 && !from.IsZero() {
			var failed int
			before := len(visits)
			visits, lastCall, failed = a.fillWatchedStops(ctx, visits, needIDs, devs, from, to, pace, lastCall, cardID)
			cardComplete = liveOK && failed == 0 && cards.WatchedCoverageComplete(visits, rankIDs, from, to)
			if len(visits) > before {
				g2 := cards.MatchGPSFirst(visits, txs, fleet, cards.DefaultStopSlack)
				if len(g2.Stations) > 0 {
					gps.Stations = g2.Stations
				}
				prior = cards.HuntNearbyFull(visits, txs, gps.Stations, devs, cards.DefaultStopSlack, false)
			}
		}
		if !cardComplete {
			allComplete = false
		}
		one := cards.HuntNearbyFull(visits, allFills, gps.Stations, devs, cards.DefaultStopSlack, cardComplete)
		one = cards.RewriteIncompleteWatchWhy(one, cardComplete)
		if opt.Persist {
			if !cardComplete {
				fmt.Fprintf(os.Stderr, "watch persist skip card=%s: watched-box coverage incomplete\n", cardID)
			} else if err := a.persistCertainNearby(ctx, one, allFills, eras); err != nil {
				return mergeWatchResults(merged, one, allComplete), err
			} else {
				eras, err = a.Store.ListEras(ctx)
				if err != nil {
					return mergeWatchResults(merged, one, allComplete), err
				}
			}
		}
		merged = mergeWatchResults(merged, one, allComplete)
	}
	merged.CoverageComplete = allComplete
	if opt.LiveStops && a.OneStep != nil {
		ids := unpairedFactoryIDs(devs, nearbyHuntFactoryIDs(merged))
		if len(ids) > 0 {
			pr, perr := a.PairDevicesByVIN(ctx, PairVINOpts{
				Live: live, FactoryIDs: ids, Pace: pace, AskEmpty: true,
			})
			if perr != nil {
				fmt.Fprintf(os.Stderr, "watch VIN-ask: %v\n", perr)
			} else {
				fmt.Fprintf(os.Stderr, "watch %s", pr.Format())
			}
			if pr.Linked > 0 {
				if stored, serr := a.Store.ListDevices(ctx); serr == nil {
					devs = overlayDeviceLinks(devs, stored)
				}
				rehunt := cards.HuntNearbyFull(visits, txs, gps.Stations, devs, cards.DefaultStopSlack, allComplete)
				rehunt = cards.RewriteIncompleteWatchWhy(rehunt, allComplete)
				if opt.Persist && allComplete {
					if err := a.persistCertainNearby(ctx, rehunt, txs, eras); err != nil {
						return rehunt, err
					}
				}
				merged = rehunt
				merged.CoverageComplete = allComplete
			}
		}
	}
	return merged, nil
}

func mergeWatchResults(acc, one cards.NearbyResult, complete bool) cards.NearbyResult {
	acc.Cards = append(acc.Cards, one.Cards...)
	acc.Certain += one.Certain
	acc.Likely += one.Likely
	acc.Watch += one.Watch
	acc.CoverageComplete = complete
	if acc.Cards == nil {
		acc.Cards = []cards.NearbyCard{}
	}
	return acc
}

func (a *App) fillWatchedStops(ctx context.Context, visits []model.StopVisit, factoryIDs []string, devs []model.OneStepDevice, from, to time.Time, pace time.Duration, lastCall time.Time, cardID string) ([]model.StopVisit, time.Time, int) {
	need := watchDevicesByFactory(devs, factoryIDs)
	var missing []model.OneStepDevice
	for _, d := range need {
		if cards.DeviceCoveredInWindow(visits, d.FactoryID, from, to) {
			continue
		}
		missing = append(missing, d)
	}
	if len(missing) == 0 {
		return visits, lastCall, 0
	}
	fmt.Fprintf(os.Stderr, "watch live-stops card=%s fetching %d boxes window=%s..%s (1 device_id/req, min_interval=%s)\n",
		cardID, len(missing), from.Format(time.RFC3339), to.Format(time.RFC3339), pace)
	extra, lastCall, failed := a.pullWatchedDriveStops(ctx, missing, from, to, pace, lastCall)
	visits = append(visits, extra...)
	if err := cards.SaveStopVisits(a.gpsStopsCacheFile(), visits); err != nil {
		fmt.Fprintf(os.Stderr, "gps-stops cache write: %v\n", err)
	}
	return visits, lastCall, failed
}

func watchDevicesByFactory(devs []model.OneStepDevice, factoryIDs []string) []model.OneStepDevice {
	want := map[string]struct{}{}
	for _, id := range factoryIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	var out []model.OneStepDevice
	seen := map[string]struct{}{}
	for _, d := range devs {
		id := strings.TrimSpace(d.FactoryID)
		if _, ok := want[id]; !ok {
			continue
		}
		if d.Dead {
			continue
		}
		if oil.HasLogisticsPersonnel(d.DisplayName) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, d)
	}
	return out
}

func (a *App) pullWatchedDriveStops(ctx context.Context, devs []model.OneStepDevice, from, to time.Time, pace time.Duration, lastCall time.Time) ([]model.StopVisit, time.Time, int) {
	if a == nil || a.OneStep == nil {
		return nil, lastCall, len(devs)
	}
	var visits []model.StopVisit
	failed := 0
	total := len(devs)
	for i, d := range devs {
		if err := watchPaceDriveStop(ctx, lastCall, pace, 0); err != nil {
			return visits, lastCall, failed + (total - i)
		}
		started := time.Now()
		v, err := a.OneStep.DriveStopVisitsFor(ctx, d, from, to)
		lastCall = started
		if err != nil && isWatchRetryable(err) {
			wait := onestep.RetryAfterOf(err)
			if wait <= 0 {
				wait = pace
			}
			fmt.Fprintf(os.Stderr, "watch Retry-After %s factory_id=%s: %v\n", wait, d.FactoryID, err)
			if werr := watchPaceDriveStop(ctx, lastCall, 0, wait); werr != nil {
				return visits, lastCall, failed + (total - i)
			}
			started = time.Now()
			v, err = a.OneStep.DriveStopVisitsFor(ctx, d, from, to)
			lastCall = started
		}
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "watch gps-stops factory_id %s: %v\n", d.FactoryID, err)
		} else {
			sentinel := model.StopVisit{
				FactoryID: d.FactoryID,
				DeviceID:  d.DeviceID,
				From:      from,
				To:        to,
			}
			if len(v) == 0 {
				v = []model.StopVisit{sentinel}
			} else {
				v = append(v, sentinel)
			}
			visits = append(visits, v...)
		}
		fmt.Fprintf(os.Stderr, "watch live-stops progress %d/%d failed=%d factory_id=%s\n", i+1, total, failed, d.FactoryID)
	}
	return visits, lastCall, failed
}

func factoryIDsOf(devs []model.OneStepDevice) []string {
	var out []string
	for _, d := range devs {
		if id := strings.TrimSpace(d.FactoryID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func isWatchRetryable(err error) bool {
	if err == nil {
		return false
	}
	var se *onestep.StatusError
	if errors.As(err, &se) && se != nil {
		return se.StatusCode == http.StatusTooManyRequests || se.StatusCode == http.StatusServiceUnavailable
	}
	return strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), "HTTP 503")
}

func watchPaceDriveStop(ctx context.Context, lastCall time.Time, minInterval, extra time.Duration) error {
	wait := extra
	if !lastCall.IsZero() && minInterval > 0 {
		gap := minInterval - time.Since(lastCall)
		if gap > wait {
			wait = gap
		}
	}
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
