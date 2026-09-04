package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/model"
	"oilchange/internal/oil"
	"oilchange/internal/onestep"
)

// CardsNearbyOpts controls the fill-day ±1 / 1-mile hunt. Never Last Reading.
type CardsNearbyOpts struct {
	CardID     string
	LiveStops  bool // pull drive-stop for boxes missing from the GPS cache (including unpaired)
	LiveReport bool // queue OneStep near_address generate-reports jobs
	Persist    bool // merge certain linked-car eras; unpaired factory_id is never joined
	ReportCap  int  // max generate-reports jobs (default 3)
}

// CardsNearby hunts unknown cards at geocoded pumps: fill-day ±1, 1 mile.
func (a *App) CardsNearby(ctx context.Context, opt CardsNearbyOpts) (cards.NearbyResult, error) {
	empty := cards.NearbyResult{}
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
	gps, err := a.matchCardsAtGPSStops(ctx, txs)
	if err != nil {
		return empty, err
	}
	txs = cards.ApplyCalls(txs, gps.Calls)
	txs = cards.EligibleUnknownFills(txs, eras)
	if opt.CardID != "" {
		id := strings.TrimSpace(opt.CardID)
		var one []model.CardTx
		for _, t := range txs {
			if t.CardID == id {
				one = append(one, t)
			}
		}
		txs = one
	}
	visits := loadNearbyVisits(a)
	complete := nearbyCoverageComplete(devs, visits, txs)
	if opt.LiveStops && a.OneStep != nil {
		var failed int
		visits, failed = a.fillMissingNearbyStops(ctx, visits, txs, devs)
		complete = failed == 0 && nearbyCoverageComplete(devs, visits, txs)
	}
	res := cards.HuntNearbyFull(visits, txs, gps.Stations, devs, cards.DefaultStopSlack, complete)
	if opt.LiveReport && a.OneStep != nil {
		a.queueNearAddressReports(ctx, txs, gps.Stations, opt.ReportCap)
	}
	if opt.Persist {
		if !complete {
			fmt.Fprintf(os.Stderr, "nearby persist skipped: GPS coverage incomplete (use --live so every active box is fetched)\n")
		} else if err := a.persistCertainNearby(ctx, res, txs, eras); err != nil {
			return res, err
		}
	}
	return res, nil
}

func loadNearbyVisits(a *App) []model.StopVisit {
	if cached, err := cards.LoadStopVisits(a.gpsStopsCacheFile()); err == nil && len(cached) > 0 {
		return cached
	}
	return nil
}

func nearbyCoverageComplete(devs []model.OneStepDevice, visits []model.StopVisit, txs []model.CardTx) bool {
	from, to := nearbyUnionWindow(txs)
	if from.IsZero() {
		return true
	}
	need := 0
	for _, d := range eligibleNearbyDevices(devs) {
		need++
		if !deviceCoveredInWindow(visits, d.FactoryID, from, to) {
			return false
		}
	}
	return need > 0 || len(eligibleNearbyDevices(devs)) == 0
}

func eligibleNearbyDevices(devs []model.OneStepDevice) []model.OneStepDevice {
	var out []model.OneStepDevice
	for _, d := range devs {
		if d.Dead || !d.Active || strings.TrimSpace(d.FactoryID) == "" {
			continue
		}
		if oil.HasLogisticsPersonnel(d.DisplayName) {
			continue
		}
		out = append(out, d)
	}
	return out
}

func deviceCoveredInWindow(visits []model.StopVisit, factoryID string, from, to time.Time) bool {
	factoryID = strings.TrimSpace(factoryID)
	for _, v := range visits {
		if strings.TrimSpace(v.FactoryID) != factoryID {
			continue
		}
		start := v.From
		end := v.To
		if start.IsZero() && end.IsZero() {
			continue
		}
		if end.IsZero() {
			end = start
		}
		if start.IsZero() {
			start = end
		}
		if !end.Before(from) && !start.After(to) {
			return true
		}
	}
	return false
}

func (a *App) fillMissingNearbyStops(ctx context.Context, visits []model.StopVisit, txs []model.CardTx, devs []model.OneStepDevice) ([]model.StopVisit, int) {
	from, to := nearbyUnionWindow(txs)
	if from.IsZero() {
		return visits, 0
	}
	var missing []model.OneStepDevice
	for _, d := range eligibleNearbyDevices(devs) {
		if deviceCoveredInWindow(visits, d.FactoryID, from, to) {
			continue
		}
		missing = append(missing, d)
	}
	if len(missing) == 0 {
		return visits, 0
	}
	fmt.Fprintf(os.Stderr, "nearby live-stops fetching %d boxes not covered in fill-day window\n", len(missing))
	extra, failed := a.pullDriveStopVisitsCounted(ctx, missing, from, to)
	visits = append(visits, extra...)
	if err := cards.SaveStopVisits(a.gpsStopsCacheFile(), visits); err != nil {
		fmt.Fprintf(os.Stderr, "gps-stops cache write: %v\n", err)
	}
	return visits, failed
}

func (a *App) pullDriveStopVisitsCounted(ctx context.Context, devs []model.OneStepDevice, from, to time.Time) ([]model.StopVisit, int) {
	if a == nil || a.OneStep == nil {
		return nil, len(devs)
	}
	var visits []model.StopVisit
	failed := 0
	for _, d := range devs {
		v, err := a.OneStep.DriveStopVisitsFor(ctx, d, from, to)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "nearby gps-stops factory_id %s: %v\n", d.FactoryID, err)
			continue
		}
		if len(v) == 0 {
			v = []model.StopVisit{{
				FactoryID: d.FactoryID,
				DeviceID:  d.DeviceID,
				From:      from,
				To:        to,
			}}
		}
		visits = append(visits, v...)
	}
	return visits, failed
}

func nearbyUnionWindow(txs []model.CardTx) (time.Time, time.Time) {
	var from, to time.Time
	for _, t := range txs {
		if t.At.IsZero() {
			continue
		}
		f, e := cards.FillDayWindow(t.At)
		if from.IsZero() || f.Before(from) {
			from = f
		}
		if to.IsZero() || e.After(to) {
			to = e
		}
	}
	return from, to
}

func (a *App) queueNearAddressReports(ctx context.Context, txs []model.CardTx, stations []cards.GeocodedStation, capN int) {
	if capN <= 0 {
		capN = 3
	}
	geo := map[string]cards.GeocodedStation{}
	for _, s := range stations {
		k := strings.ToLower(strings.TrimSpace(s.Name) + "|" + strings.TrimSpace(s.Address))
		geo[k] = s
	}
	type key struct{ addr, from, to string }
	seen := map[key]struct{}{}
	n := 0
	for _, t := range txs {
		addr := strings.TrimSpace(t.StationAddress)
		if addr == "" {
			k := strings.ToLower(strings.TrimSpace(t.StationName) + "|" + strings.TrimSpace(t.StationAddress))
			if s, ok := geo[k]; ok {
				addr = strings.TrimSpace(s.Address)
			}
		}
		if addr == "" {
			continue
		}
		from, to := cards.FillDayWindow(t.At)
		if from.IsZero() {
			continue
		}
		k := key{addr, from.Format(time.RFC3339), to.Format(time.RFC3339)}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		job, err := a.OneStep.GenerateNearAddress(ctx, onestep.NearAddressQuery{
			Address: addr,
			From:    from,
			To:      to,
			Name:    "oilchange near_address",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "near_address generate %s: %v\n", addr, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "near_address queued id=%s status=%s addr=%s from=%s to=%s\n",
			job.ID, job.Status, addr, from.Format(time.RFC3339), to.Format(time.RFC3339))
		n++
		if n >= capN {
			break
		}
	}
}

func (a *App) persistCertainNearby(ctx context.Context, res cards.NearbyResult, txs []model.CardTx, existing []model.CardEra) error {
	if !res.CoverageComplete {
		return nil
	}
	merged := append([]model.CardEra(nil), existing...)
	added := 0
	for _, c := range res.Cards {
		if cards.CardHasPersonEra(existing, c.CardID) {
			fmt.Fprintf(os.Stderr, "nearby persist skip card=%s: PERSON era\n", c.CardID)
			continue
		}
		var linked []cards.NearbyDevice
		cars := map[string]struct{}{}
		for _, d := range c.Certain {
			if strings.TrimSpace(d.LinkedCar) == "" {
				continue
			}
			if oil.HasLogisticsPersonnel(d.LinkedCar) {
				continue
			}
			linked = append(linked, d)
			cars[d.LinkedCar] = struct{}{}
		}
		if len(cars) != 1 {
			continue
		}
		d := linked[0]
		from, to := exclusiveDaySpan(txs, c.CardID)
		merged = append(merged, model.CardEra{
			CardID:     c.CardID,
			EFleetsID:  d.LinkedCar,
			HolderType: cards.HolderCar,
			HolderKey:  d.LinkedCar,
			From:       from,
			To:         to,
			EvidenceN:  d.ExclusiveFills,
			Stations:   d.Stations,
		})
		added++
	}
	if added == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "nearby persist %d certain linked car era(s); unpaired factory_id not joined\n", added)
	return a.Store.ReplaceEras(ctx, merged)
}

func exclusiveDaySpan(txs []model.CardTx, cardID string) (time.Time, time.Time) {
	var from, to time.Time
	for _, t := range txs {
		if t.CardID != cardID || t.At.IsZero() {
			continue
		}
		if from.IsZero() || t.At.Before(from) {
			from = t.At
		}
		if to.IsZero() || t.At.After(to) {
			to = t.At
		}
	}
	return from, to
}
