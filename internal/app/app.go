package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

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
	Cfg   config.Config
	Store *store.Store
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
		fills, stations, cards, err := enterprise.ParseFills(bytes.NewReader(b))
		if err != nil {
			return err
		}
		for _, f := range fills {
			if err := a.Store.UpsertFill(ctx, f); err != nil {
				return err
			}
		}
		for _, g := range stations {
			if err := a.Store.UpsertStation(ctx, g); err != nil {
				return err
			}
		}
		for _, c := range cards {
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

	// One transaction: a bad map row aborts the whole import instead of leaving
	// half the registry on the new snapshot and half on the old one.
	if err := a.Store.UpsertDevices(ctx, devices); err != nil {
		return 0, err
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
			if d.Dead {
				continue
			}
			n, err := client.DriveStopMilesFor(ctx, d, out.FillTime)
			if err != nil {
				fetchErrors = append(fetchErrors, fmt.Errorf("%s factory_id %s: %w", c.EFleetsID, d.FactoryID, err))
				continue
			}
			if err := a.Store.SaveMilesSince(ctx, model.DriveStopMiles{FactoryID: d.FactoryID, Since: out.FillTime, Miles: n}); err != nil {
				return err
			}
		}
	}
	return errors.Join(fetchErrors...)
}

// Compute runs Last Reading + HOLD for every car. Returns exit 2 if any open HOLD remains.
//
// Invariant after a Compute that returns nil error: every car has either
// hold_reason set (with exactly one open hold event) or a full
// last_reading_{miles,at,source} triple — never both NULL. Each car is decided
// in its own store transaction, so a failure on one car leaves that car exactly
// as it was and the loop moves on; the joined error and exit 1 tell the
// operator which cars were not decided this run. Context cancellation stops
// the loop immediately (the current car's transaction rolls back as a unit).
func (a *App) Compute(ctx context.Context, overrideLower bool) (int, error) {
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return model.ExitError, err
	}
	open := 0
	var carErrs []error
	for _, c := range cars {
		if err := ctx.Err(); err != nil {
			return model.ExitError, err
		}
		held, err := a.computeCar(ctx, c, overrideLower)
		if err != nil {
			if ctx.Err() != nil {
				return model.ExitError, err
			}
			fmt.Fprintf(os.Stderr, "ERROR %s not decided: %v\n", c.EFleetsID, err)
			carErrs = append(carErrs, fmt.Errorf("%s: %w", c.EFleetsID, err))
			continue
		}
		if held {
			open++
		}
	}
	if len(carErrs) > 0 {
		return model.ExitError, errors.Join(carErrs...)
	}
	if open > 0 {
		return model.ExitHolds, nil
	}
	return model.ExitOK, nil
}

// computeCar decides one car: exactly one of SetHold / WriteLastReading commits.
func (a *App) computeCar(ctx context.Context, c model.Car, overrideLower bool) (held bool, err error) {
	fills, err := a.Store.ListFills(ctx, c.EFleetsID)
	if err != nil {
		return false, err
	}
	ros, err := a.Store.ListShopROs(ctx, c.EFleetsID)
	if err != nil {
		return false, err
	}
	devs, err := a.Store.ListDevicesForCar(ctx, c.EFleetsID)
	if err != nil {
		return false, err
	}
	var ids []string
	for _, d := range devs {
		ids = append(ids, d.FactoryID)
	}
	miles, err := a.Store.ListMilesSince(ctx, ids)
	if err != nil {
		return false, err
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
			return false, err
		}
		fmt.Fprintf(os.Stderr, "HOLD %s %s %s\n", c.EFleetsID, reason, detail)
		return true, nil
	}
	return false, a.Store.WriteLastReading(ctx, c.EFleetsID, out.Reading, out.FillTime, out.Source)
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
