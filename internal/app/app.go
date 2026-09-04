package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/config"
	"oilchange/internal/enterprise"
	"oilchange/internal/export"
	"oilchange/internal/model"
	"oilchange/internal/oil"
	"oilchange/internal/onestep"
	"oilchange/internal/store"
)

// App wires CLI commands. Last Reading still happens only in internal/oil.
type App struct {
	Cfg         config.Config
	Store       *store.Store
	CardsMirror string          // web/data/cards.json; empty skips the Cards desk write
	OneStep     *onestep.Client // optional; GPS stop times for card matching
}

// OpenStore opens sqlite or postgres from env.
func OpenStore(cfg config.Config) (*store.Store, error) {
	driver, dsn, err := cfg.DSN()
	if err != nil {
		return nil, err
	}
	return store.Open(driver, dsn)
}

// SyncEnterprise loads vehicles, fills, shop ROs. Oil/lube ROs seed last oil. Does not compute Last Reading.
func (a *App) SyncEnterprise(ctx context.Context, vehicles, fuel, shop, mileage string) error {
	ad, err := a.adapter(vehicles, fuel, shop, mileage)
	if err != nil {
		return err
	}
	if vehicles != "" || a.hasFile(ad, enterprise.ReportVehicles) {
		b, _, err := ad.Fetch(ctx, enterprise.ReportVehicles)
		if err != nil {
			return err
		}
		cars, err := enterprise.ParseVehicles(bytes.NewReader(b))
		if err != nil {
			return err
		}
		for _, c := range cars {
			if err := a.Store.UpsertCar(ctx, c); err != nil {
				return err
			}
		}
	}
	if fuel != "" || a.hasFile(ad, enterprise.ReportFuelDetails) {
		b, _, err := ad.Fetch(ctx, enterprise.ReportFuelDetails)
		if err != nil {
			return err
		}
		fills, stations, cardRows, err := enterprise.ParseFills(bytes.NewReader(b))
		if err != nil {
			return err
		}
		roster, err := a.Store.ListCars(ctx)
		if err != nil {
			return err
		}
		known := make(map[string]bool, len(roster))
		for _, c := range roster {
			known[c.EFleetsID] = true
		}
		skipped := 0
		for _, f := range fills {
			if !known[f.EFleetsID] {
				skipped++
				continue
			}
			if err := a.Store.UpsertFill(ctx, f); err != nil {
				return err
			}
			if tx, ok := cards.TxFromFill(f); ok {
				if err := a.Store.UpsertCardTx(ctx, tx); err != nil {
					return err
				}
			}
		}
		if skipped > 0 {
			fmt.Fprintf(os.Stderr, "DETAILS skipped %d punches for vehicles not on the roster\n", skipped)
		}
		for _, g := range stations {
			if err := a.Store.UpsertStation(ctx, g); err != nil {
				return err
			}
		}
		for _, c := range cardRows {
			if c.LinkedCarEFleetsID != nil && !known[*c.LinkedCarEFleetsID] {
				c.LinkedCarEFleetsID = nil
			}
			if err := a.Store.UpsertCard(ctx, c); err != nil {
				return err
			}
		}
	}
	if shop != "" || a.hasFile(ad, enterprise.ReportShopRO) {
		b, _, err := ad.Fetch(ctx, enterprise.ReportShopRO)
		if err != nil {
			return err
		}
		ros, locs, oils, err := enterprise.ParseShopROs(bytes.NewReader(b))
		if err != nil {
			return err
		}
		for _, r := range ros {
			if err := a.Store.UpsertShopRO(ctx, r); err != nil {
				return err
			}
		}
		for _, l := range locs {
			if err := a.Store.UpsertMaintLoc(ctx, l); err != nil {
				return err
			}
		}
		for _, o := range oils {
			ok, err := a.Store.HasOilChange(ctx, o.EFleetsID, o.Miles, o.Date)
			if err != nil {
				return err
			}
			if ok {
				continue
			}
			if err := a.Store.InsertOilChange(ctx, o); err != nil {
				return err
			}
		}
	}
	if mileage != "" {
		b, _, err := ad.Fetch(ctx, enterprise.ReportMileage)
		if err != nil {
			return err
		}
		if _, err := enterprise.ParseMileage(bytes.NewReader(b)); err != nil {
			return err
		}
	}
	return nil
}

// hasFile is true for a live adapter that can actually fetch that report, so file-drop empty flags do not skip live.
func (a *App) hasFile(ad enterprise.Adapter, kind enterprise.ReportKind) bool {
	_, ok := ad.(enterprise.FileAdapter)
	if ok {
		return false
	}
	_, _, err := ad.Fetch(context.Background(), kind)
	return err == nil
}

// adapter prefers explicit files (tests) over live login so CI never needs eFleets secrets.
func (a *App) adapter(vehicles, fuel, shop, mileage string) (enterprise.Adapter, error) {
	if vehicles != "" || fuel != "" || shop != "" {
		return enterprise.FileAdapter{Vehicles: vehicles, Fuel: fuel, ShopRO: shop, Mileage: mileage}, nil
	}
	if a.Cfg.EFleetsUser == "" {
		return nil, fmt.Errorf("pass --vehicles/--fuel-details or set EFLEETS_USERNAME")
	}
	h, err := enterprise.NewHTTPAdapter(a.Cfg.EFleetsBase, a.Cfg.EFleetsUser, a.Cfg.EFleetsPass, a.Cfg.EFleetsCust)
	if err != nil {
		return nil, err
	}
	h.DetailsURL = a.Cfg.EFleetsDetails
	h.MaintURL = a.Cfg.EFleetsMaint
	h.FleetURL = a.Cfg.EFleetsFleet
	return h, nil
}

// SyncDevices upserts the durable OneStep device registry from a map CSV and/or API.
// Join key is factory_id only; display_name is never used to pair. Does not fetch drive-stop miles.
func (a *App) SyncDevices(ctx context.Context, mapPath string, client *onestep.Client) (int, error) {
	factoryToCar := map[string]string{}
	var mapped []model.OneStepDevice
	if mapPath != "" {
		var err error
		mapped, err = onestep.LoadMapCSV(mapPath)
		if err != nil {
			return 0, err
		}
		for _, d := range mapped {
			if d.LinkedCarEFleetsID != nil && *d.LinkedCarEFleetsID != "" {
				factoryToCar[d.FactoryID] = *d.LinkedCarEFleetsID
			}
		}
	}

	var devices []model.OneStepDevice
	if client != nil && a.Cfg.OneStepToken != "" {
		apiDevs, err := client.ListDevices(ctx)
		if err != nil {
			return 0, err
		}
		for _, d := range apiDevs {
			devices = append(devices, onestep.LinkByFactoryID(d, factoryToCar))
		}
		// Keep map rows whose factory_id is absent from the live inventory (retired / offline boxes).
		seen := make(map[string]bool, len(devices))
		for _, d := range devices {
			seen[d.FactoryID] = true
		}
		for _, d := range mapped {
			if !seen[d.FactoryID] {
				devices = append(devices, d)
			}
		}
	} else {
		devices = mapped
	}

	for _, d := range devices {
		if err := a.Store.UpsertDevice(ctx, d); err != nil {
			return 0, err
		}
	}
	return len(devices), nil
}

// SyncOneStep loads the device registry then drive-stop miles-since after each car's trusted second.
func (a *App) SyncOneStep(ctx context.Context, mapPath string, client *onestep.Client) error {
	if _, err := a.SyncDevices(ctx, mapPath, client); err != nil {
		return err
	}
	if client == nil {
		return nil
	}
	var fetchErrors []error
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return err
	}
	for _, c := range cars {
		devs, err := a.Store.ListDevicesForCar(ctx, c.EFleetsID)
		if err != nil {
			return err
		}
		fills, err := a.Store.ListFills(ctx, c.EFleetsID)
		if err != nil {
			return err
		}
		ros, err := a.Store.ListShopROs(ctx, c.EFleetsID)
		if err != nil {
			return err
		}
		out := oil.EvaluateHolds(oil.ComputeIn{Nickname: c.Nickname, Fills: fills, ShopROs: ros, Devices: devs})
		if out.FillTime.IsZero() {
			continue
		}
		for _, d := range devs {
			if d.Dead || !d.Active {
				continue
			}
			n, err := client.DriveStopMilesFor(ctx, d, out.FillTime)
			if err != nil {
				fetchErrors = append(fetchErrors, fmt.Errorf("%s factory_id %s: %w", c.EFleetsID, d.FactoryID, err))
				fmt.Fprintf(os.Stderr, "drive-stop %s factory_id %s: %v\n", c.EFleetsID, d.FactoryID, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "drive-stop %s factory_id %s miles=%.2f\n", c.EFleetsID, d.FactoryID, n)
			if err := a.Store.SaveMilesSince(ctx, model.DriveStopMiles{FactoryID: d.FactoryID, Since: out.FillTime, Miles: n}); err != nil {
				return err
			}
		}
	}
	return errors.Join(fetchErrors...)
}

// Compute runs Last Reading + HOLD for every car. Returns exit 2 if any open HOLD remains.
func (a *App) Compute(ctx context.Context, overrideLower bool) (int, error) {
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return model.ExitError, err
	}
	open := 0
	for _, c := range cars {
		fills, err := a.Store.ListFills(ctx, c.EFleetsID)
		if err != nil {
			return model.ExitError, err
		}
		ros, err := a.Store.ListShopROs(ctx, c.EFleetsID)
		if err != nil {
			return model.ExitError, err
		}
		devs, err := a.Store.ListDevicesForCar(ctx, c.EFleetsID)
		if err != nil {
			return model.ExitError, err
		}
		var ids []string
		for _, d := range devs {
			ids = append(ids, d.FactoryID)
		}
		miles, err := a.Store.ListMilesSince(ctx, ids)
		if err != nil {
			return model.ExitError, err
		}
		out := oil.EvaluateHolds(oil.ComputeIn{
			Nickname:          c.Nickname,
			Fills:             fills,
			ShopROs:           ros,
			Devices:           devs,
			MilesSince:        miles,
			StoredLastReading: c.LastReadingMiles,
			OverrideLower:     overrideLower,
		})
		if out.SkipWrite {
			reason := model.HoldNoTrustedFill
			detail := ""
			if len(out.Holds) > 0 {
				reason = out.Holds[0].Code
				detail = out.Holds[0].Detail
			}
			if err := a.Store.SetHold(ctx, c.EFleetsID, reason, detail); err != nil {
				return model.ExitError, err
			}
			open++
			fmt.Fprintf(os.Stderr, "HOLD %s %s %s\n", c.EFleetsID, reason, detail)
			continue
		}
		if err := a.Store.WriteLastReading(ctx, c.EFleetsID, out.Reading, out.FillTime, out.Source); err != nil {
			return model.ExitError, err
		}
	}
	if open > 0 {
		return model.ExitHolds, nil
	}
	return model.ExitOK, nil
}

// OilDone records an operator oil change without touching Last Reading.
func (a *App) OilDone(ctx context.Context, efleetsID string, miles int, day time.Time, loc string) error {
	return a.Store.InsertOilChange(ctx, model.OilChange{
		EFleetsID: efleetsID,
		Miles:     miles,
		Date:      day,
		Location:  loc,
		Source:    "oil-done",
	})
}

// Report writes the lean CSV. due-within filters in-app using due_at = last_oil + interval; that number is not a column.
func (a *App) Report(ctx context.Context, interval, dueWithin int, outPath string) error {
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return err
	}
	w := os.Stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return export.WriteCSV(w, cars, dueWithin, interval)
}

// Holds prints open HOLDs. Stale last_reading is not shown as current odo.
func (a *App) Holds(ctx context.Context) error {
	hs, err := a.Store.OpenHolds(ctx)
	if err != nil {
		return err
	}
	for _, h := range hs {
		fmt.Printf("HOLD %s %s %s\n", h.EFleetsID, h.Reason, h.Detail)
	}
	return nil
}

func (a *App) CardsRebuild(ctx context.Context, fuelPath string) (int, error) {
	if fuelPath != "" {
		if err := a.SyncEnterprise(ctx, "", fuelPath, "", ""); err != nil {
			return 0, err
		}
	}
	txs, err := a.Store.ListCardTxs(ctx, "")
	if err != nil {
		return 0, err
	}
	gps, err := a.matchCardsAtGPSStops(ctx, txs)
	if err != nil {
		return 0, err
	}
	scored := cards.ApplyCalls(txs, gps.Calls)
	ps := cards.ScorePairings(scored, time.Now().UTC())
	if err := a.Store.ReplacePairings(ctx, ps); err != nil {
		return 0, err
	}
	if err := a.writeCardsSnapshot(ctx, scored, ps, gps); err != nil {
		return 0, err
	}
	splits := 0
	seen := map[string]struct{}{}
	for _, e := range gps.Eras {
		if e.Split {
			if _, ok := seen[e.CardID]; !ok {
				seen[e.CardID] = struct{}{}
				splits++
			}
		}
	}
	best := 0
	hits := 0
	for _, g := range gps.Matches {
		hits += g.EvidenceN
		if g.Best {
			best++
		}
	}
	fmt.Fprintf(os.Stderr, "gps-first matches=%d best=%d eras=%d split_cards=%d calls=%d geocoded_stations=%d\n",
		hits, best, len(gps.Eras), splits, len(gps.Calls), len(gps.Stations))
	return len(txs), nil
}

func gpsStopsCachePath() string {
	return filepath.Join("data", "runtime", "gps-stops.json")
}

func (a *App) matchCardsAtGPSStops(ctx context.Context, txs []model.CardTx) (cards.GPSFirstResult, error) {
	empty := cards.GPSFirstResult{}
	if a == nil || a.Store == nil || len(txs) == 0 {
		return empty, nil
	}
	fleet, err := a.Store.ListCars(ctx)
	if err != nil {
		return empty, err
	}
	cache := gpsStopsCachePath()
	if a.OneStep == nil {
		visits, err := cards.LoadStopVisits(cache)
		if err != nil {
			return empty, nil
		}
		fmt.Fprintf(os.Stderr, "gps-stops cache %d visits\n", len(visits))
		return cards.MatchGPSFirst(visits, txs, fleet, cards.DefaultStopSlack), nil
	}
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
	if from.IsZero() {
		return empty, nil
	}
	devs, err := a.Store.ListDevices(ctx)
	if err != nil {
		return empty, err
	}
	var visits []model.StopVisit
	n := 0
	for _, d := range devs {
		if d.Dead || !d.Active || d.LinkedCarEFleetsID == nil || *d.LinkedCarEFleetsID == "" {
			continue
		}
		n++
		v, err := a.OneStep.DriveStopVisitsFor(ctx, d, from, to)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gps-stops %s factory_id %s: %v\n", *d.LinkedCarEFleetsID, d.FactoryID, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "gps-stops %s factory_id %s stops=%d\n", *d.LinkedCarEFleetsID, d.FactoryID, len(v))
		visits = append(visits, v...)
	}
	if n == 0 {
		return empty, nil
	}
	if err := cards.SaveStopVisits(cache, visits); err != nil {
		fmt.Fprintf(os.Stderr, "gps-stops cache write: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "gps-stops cache wrote %d visits\n", len(visits))
	}
	return cards.MatchGPSFirst(visits, txs, fleet, cards.DefaultStopSlack), nil
}

func (a *App) writeCardsSnapshot(ctx context.Context, txs []model.CardTx, ps []model.CardPairing, gps cards.GPSFirstResult) error {
	if a == nil || a.CardsMirror == "" {
		return nil
	}
	nicks := map[string]string{}
	if a.Store != nil {
		cars, err := a.Store.ListCars(ctx)
		if err != nil {
			return err
		}
		for _, c := range cars {
			nicks[c.EFleetsID] = c.Nickname
		}
	}
	snap := cards.BuildSnapshotFull(txs, ps, gps.Matches, gps.Eras, gps.Calls, nicks, time.Now().UTC())
	snap.GeocodedStations = gps.Stations
	return cards.WriteSnapshot(a.CardsMirror, snap)
}

func (a *App) gpsFirstFromCache(ctx context.Context) (cards.GPSFirstResult, []model.CardTx, error) {
	empty := cards.GPSFirstResult{}
	txs, err := a.Store.ListCardTxs(ctx, "")
	if err != nil {
		return empty, nil, err
	}
	res, err := a.matchCardsAtGPSStops(ctx, txs)
	if err != nil {
		return empty, nil, err
	}
	return res, txs, nil
}

func (a *App) CardsSplit(ctx context.Context, cardID string) error {
	res, _, err := a.gpsFirstFromCache(ctx)
	if err != nil {
		return err
	}
	cardID = strings.TrimSpace(cardID)
	n := 0
	for _, e := range res.Eras {
		if cardID != "" && e.CardID != cardID {
			continue
		}
		n++
		name := firstNonEmpty(e.Nickname, e.EFleetsID)
		flag := "HOME"
		if e.Split {
			flag = "SPLIT"
		}
		fmt.Printf("%s card=%s car=%s name=%s from=%s to=%s n=%d stations=%s\n",
			flag, e.CardID, e.EFleetsID, name,
			e.From.UTC().Format(time.RFC3339), e.To.UTC().Format(time.RFC3339),
			e.EvidenceN, strings.Join(e.Stations, ","))
	}
	if n == 0 {
		if cardID == "" {
			fmt.Println("no GPS card eras (need gps-stops cache or live OneStep)")
		} else {
			fmt.Printf("no GPS eras for card %s\n", cardID)
		}
	}
	return nil
}

func (a *App) CardsCall(ctx context.Context, cardID string, disagreeOnly bool) error {
	res, _, err := a.gpsFirstFromCache(ctx)
	if err != nil {
		return err
	}
	cardID = strings.TrimSpace(cardID)
	n := 0
	for _, c := range res.Calls {
		if cardID != "" && c.CardID != cardID {
			continue
		}
		if disagreeOnly && (c.EnterpriseCar == "" || c.EnterpriseCar == c.CalledCar) {
			continue
		}
		n++
		fmt.Printf("CALL card=%s at=%s station=%s enterprise=%s called=%s (%s) why=%s\n",
			c.CardID, c.At.UTC().Format(time.RFC3339), c.Station, c.EnterpriseCar,
			c.CalledName, c.CalledCar, c.Why)
	}
	if n == 0 {
		fmt.Println("no GPS-named swipes (try cards rebuild, then cards call)")
	}
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func (a *App) CardsSuspects(ctx context.Context) error {
	txs, err := a.Store.ListCardTxs(ctx, "")
	if err != nil {
		return err
	}
	ps, err := a.Store.ListPairings(ctx, "")
	if err != nil {
		return err
	}
	if len(ps) == 0 {
		ps = cards.ScorePairings(txs, time.Now().UTC())
	}
	ss := cards.FindSuspects(txs, ps)
	if len(ss) == 0 {
		fmt.Println("no suspect cards")
		return nil
	}
	for _, s := range ss {
		fmt.Printf("SUSPECT card=%s enterprise_car=%s best_car=%s latest=%s station=%s evidence_best=%d evidence_latest=%d\n  %s\n",
			s.CardID, s.EnterpriseCar, s.BestCar, s.LatestAt.UTC().Format(time.RFC3339), s.LatestStation, s.EvidenceBest, s.EvidenceLatest, s.Reason)
	}
	return nil
}

func (a *App) CardsTrace(ctx context.Context, cardID string, windowDays int) error {
	txs, err := a.Store.ListCardTxs(ctx, "")
	if err != nil {
		return err
	}
	hits := cards.TraceStationDays(txs, cardID, windowDays)
	if len(hits) == 0 {
		fmt.Printf("TRACE card=%s no station-day neighbors in ±%d days\n", cardID, windowDays)
		return nil
	}
	for _, h := range hits {
		fmt.Printf("TRACE card=%s station=%s day=%s other_car=%s other_card=%s other_at=%s days_apart=%d\n",
			h.CardID, h.Station, h.Day.Format("2006-01-02"), h.OtherEFleetsID, h.OtherCardID,
			h.OtherAt.UTC().Format(time.RFC3339), h.DaysApart)
	}
	return nil
}

func (a *App) CardsPairings(ctx context.Context, cardID string) error {
	ps, err := a.Store.ListPairings(ctx, cardID)
	if err != nil {
		return err
	}
	if len(ps) == 0 {
		txs, err := a.Store.ListCardTxs(ctx, cardID)
		if err != nil {
			return err
		}
		ps = cards.ScorePairings(txs, time.Now().UTC())
	}
	if len(ps) == 0 {
		fmt.Println("no pairings (run cards rebuild first)")
		return nil
	}
	for _, p := range ps {
		mark := ""
		if p.Best {
			mark = " BEST"
		}
		fmt.Printf("PAIR card=%s type=%s key=%s n=%d score=%.2f%s\n", p.CardID, p.EntityType, p.EntityKey, p.EvidenceN, p.Score, mark)
	}
	return nil
}
