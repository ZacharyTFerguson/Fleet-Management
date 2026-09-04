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
	LiveReport bool // queue OneStep near_address generate-reports (rows still prefer stops)
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
	gps, err := a.matchCardsAtGPSStops(ctx, txs)
	if err != nil {
		return empty, err
	}
	visits := gpsVisitsOrCache(a, gps)
	if opt.LiveStops && a.OneStep != nil {
		visits = a.fillMissingNearbyStops(ctx, visits, txs, devs)
	}
	res := cards.HuntNearby(visits, txs, gps.Stations, devs, cards.DefaultStopSlack)
	if opt.CardID != "" {
		res = filterNearbyCard(res, opt.CardID)
	}
	if opt.LiveReport && a.OneStep != nil {
		a.queueNearAddressReports(ctx, txs, gps.Stations, opt.ReportCap)
	}
	if opt.Persist {
		if err := a.persistCertainNearby(ctx, res, txs); err != nil {
			return res, err
		}
	}
	return res, nil
}

func gpsVisitsOrCache(a *App, gps cards.GPSFirstResult) []model.StopVisit {
	if cached, err := cards.LoadStopVisits(a.gpsStopsCacheFile()); err == nil && len(cached) > 0 {
		return cached
	}
	return nil
}

func (a *App) fillMissingNearbyStops(ctx context.Context, visits []model.StopVisit, txs []model.CardTx, devs []model.OneStepDevice) []model.StopVisit {
	from, to := nearbyUnionWindow(txs)
	if from.IsZero() {
		return visits
	}
	seen := visitFactorySet(visits)
	var missing []model.OneStepDevice
	for _, d := range devs {
		if d.Dead || !d.Active {
			continue
		}
		if strings.TrimSpace(d.FactoryID) == "" {
			continue
		}
		if oil.HasLogisticsPersonnel(d.DisplayName) {
			continue
		}
		if _, ok := seen[d.FactoryID]; ok {
			continue
		}
		missing = append(missing, d)
	}
	if len(missing) == 0 {
		return visits
	}
	fmt.Fprintf(os.Stderr, "nearby live-stops fetching %d boxes not in GPS cache\n", len(missing))
	extra := a.pullDriveStopVisits(ctx, missing, from, to)
	visits = append(visits, extra...)
	if err := cards.SaveStopVisits(a.gpsStopsCacheFile(), visits); err != nil {
		fmt.Fprintf(os.Stderr, "gps-stops cache write: %v\n", err)
	}
	return visits
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

func filterNearbyCard(res cards.NearbyResult, cardID string) cards.NearbyResult {
	cardID = strings.TrimSpace(cardID)
	out := cards.NearbyResult{}
	for _, c := range res.Cards {
		if c.CardID != cardID {
			continue
		}
		out.Cards = append(out.Cards, c)
		out.Certain += len(c.Certain)
		out.Likely += len(c.Likely)
		out.Watch += len(c.Watch)
	}
	if out.Cards == nil {
		out.Cards = []cards.NearbyCard{}
	}
	return out
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

func (a *App) persistCertainNearby(ctx context.Context, res cards.NearbyResult, txs []model.CardTx) error {
	linked := cards.CertainLinkedCars(res)
	if len(linked) == 0 {
		return nil
	}
	existing, err := a.Store.ListEras(ctx)
	if err != nil {
		return err
	}
	hasCar := map[string]struct{}{}
	for _, e := range existing {
		if strings.TrimSpace(e.HolderType) == "" || e.HolderType == cards.HolderCar {
			hasCar[e.CardID] = struct{}{}
		}
	}
	byCard := map[string][]model.CardTx{}
	for _, t := range txs {
		byCard[t.CardID] = append(byCard[t.CardID], t)
	}
	merged := append([]model.CardEra(nil), existing...)
	added := 0
	for _, c := range res.Cards {
		if _, ok := hasCar[c.CardID]; ok {
			continue
		}
		if oil.HasLogisticsPersonnel(c.CardID) {
			continue
		}
		for _, d := range c.Certain {
			if strings.TrimSpace(d.LinkedCar) == "" {
				continue
			}
			from, to := eraSpan(byCard[c.CardID])
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
			break
		}
	}
	if added == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "nearby persist %d certain linked car era(s); unpaired factory_id not joined\n", added)
	return a.Store.ReplaceEras(ctx, merged)
}

func eraSpan(txs []model.CardTx) (time.Time, time.Time) {
	var from, to time.Time
	for _, t := range txs {
		if t.At.IsZero() {
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
