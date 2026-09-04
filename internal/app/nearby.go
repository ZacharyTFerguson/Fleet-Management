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
	savedOS := a.OneStep
	if !opt.LiveStops {
		a.OneStep = nil
	}
	gps, err := a.matchCardsAtGPSStops(ctx, txs)
	a.OneStep = savedOS
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
	liveOK := true
	if opt.LiveStops && a.OneStep != nil {
		if live, lerr := a.OneStep.ListDevices(ctx); lerr != nil {
			fmt.Fprintf(os.Stderr, "nearby live devices: %v\n", lerr)
			liveOK = false
		} else {
			devs = mergeNearbyDevices(devs, live)
		}
	}
	visits := loadNearbyVisits(a)
	complete := liveOK && nearbyCoverageComplete(devs, visits, txs)
	if opt.LiveStops && a.OneStep != nil {
		var failed int
		visits, failed = a.fillMissingNearbyStops(ctx, visits, txs, devs)
		complete = liveOK && failed == 0 && nearbyCoverageComplete(devs, visits, txs)
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
		// A short stop inside a multi-day window is not a fetch of that window.
		if !start.After(from) && !end.Before(to) {
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
	// drive-stop is one device_id per GET (no proven multi-device batch param).
	// Cap at ~1 req/s so a fleet pull stays under the 5,000/hour support ceiling.
	fmt.Fprintf(os.Stderr, "nearby live-stops fetching %d boxes not covered in fill-day window (1 device_id/req, min_interval=%s, progress every %d)\n",
		len(missing), nearbyDriveStopMinInterval, nearbyLiveStopProgressEvery)
	extra, failed := a.pullDriveStopVisitsCounted(ctx, missing, from, to)
	visits = append(visits, extra...)
	if err := cards.SaveStopVisits(a.gpsStopsCacheFile(), visits); err != nil {
		fmt.Fprintf(os.Stderr, "gps-stops cache write: %v\n", err)
	}
	return visits, failed
}

// nearbyDriveStopMinInterval keeps serialized multi-box drive-stop under ~1 req/s.
// Support (track.onestepgps.com/support.php) allows 5,000/hour and recommends
// batching when possible; this route only accepts a single device_id, so pace
// instead of inventing multi-id query params. 15–30s would be too slow for ~260 boxes.
var nearbyDriveStopMinInterval = time.Second

const nearbyLiveStopProgressEvery = 25

func (a *App) pullDriveStopVisitsCounted(ctx context.Context, devs []model.OneStepDevice, from, to time.Time) ([]model.StopVisit, int) {
	if a == nil || a.OneStep == nil {
		return nil, len(devs)
	}
	var visits []model.StopVisit
	failed := 0
	var lastCall time.Time
	total := len(devs)
	for i, d := range devs {
		if err := nearbyPaceDriveStop(ctx, lastCall); err != nil {
			return visits, failed + (total - i)
		}
		started := time.Now()
		v, err := a.OneStep.DriveStopVisitsFor(ctx, d, from, to)
		lastCall = started
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "nearby gps-stops factory_id %s: %v\n", d.FactoryID, err)
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
		n := i + 1
		if n == 1 || n == total || n%nearbyLiveStopProgressEvery == 0 {
			fmt.Fprintf(os.Stderr, "nearby live-stops progress %d/%d failed=%d\n", n, total, failed)
		}
	}
	return visits, failed
}

func nearbyPaceDriveStop(ctx context.Context, lastCall time.Time) error {
	if lastCall.IsZero() || nearbyDriveStopMinInterval <= 0 {
		return nil
	}
	wait := nearbyDriveStopMinInterval - time.Since(lastCall)
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
		if n >= capN {
			break
		}
		k := key{addr, from.Format(time.RFC3339), to.Format(time.RFC3339)}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		n++
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
		if len(c.Certain) != 1 {
			continue
		}
		d := c.Certain[0]
		if strings.TrimSpace(d.LinkedCar) == "" {
			continue
		}
		if oil.HasLogisticsPersonnel(d.LinkedCar) {
			continue
		}
		from, to := exclusiveDaySpan(txs, c.CardID, d.ExclusiveDays)
		if from.IsZero() || to.IsZero() {
			continue
		}
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

func exclusiveDaySpan(txs []model.CardTx, cardID string, days []string) (time.Time, time.Time) {
	want := map[string]struct{}{}
	for _, d := range days {
		d = strings.TrimSpace(d)
		if d != "" {
			want[d] = struct{}{}
		}
	}
	if len(want) == 0 {
		return time.Time{}, time.Time{}
	}
	var from, to time.Time
	for _, t := range txs {
		if t.CardID != cardID || t.At.IsZero() {
			continue
		}
		if _, ok := want[cards.EasternDay(t.At)]; !ok {
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

func mergeNearbyDevices(stored, live []model.OneStepDevice) []model.OneStepDevice {
	by := map[string]model.OneStepDevice{}
	for _, d := range stored {
		id := strings.TrimSpace(d.FactoryID)
		if id == "" {
			continue
		}
		by[id] = d
	}
	for _, d := range live {
		id := strings.TrimSpace(d.FactoryID)
		if id == "" {
			continue
		}
		if old, ok := by[id]; ok {
			if strings.TrimSpace(d.DeviceID) != "" {
				old.DeviceID = d.DeviceID
			}
			old.Active = d.Active
			old.Dead = d.Dead
			if strings.TrimSpace(d.DisplayName) != "" {
				old.DisplayName = d.DisplayName
			}
			if strings.TrimSpace(d.VIN) != "" {
				old.VIN = d.VIN
			}
			by[id] = old
			continue
		}
		by[id] = d
	}
	out := make([]model.OneStepDevice, 0, len(by))
	for _, d := range by {
		out = append(out, d)
	}
	return out
}
