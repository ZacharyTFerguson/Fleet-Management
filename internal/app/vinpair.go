package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
	"oilchange/internal/onestep"
)

// PairVINOpts asks OneStep for OBD VIN and joins unpaired boxes to cars.vin.
// Never Last Reading. display_name is never a join key.
type PairVINOpts struct {
	Live       []model.OneStepDevice // already-fetched /device list (optional)
	FactoryIDs []string              // empty = every unpaired box
	Pace       time.Duration         // between per-device VIN GETs (default 35s)
	AskEmpty   bool                  // GET /device?device_id= for boxes still missing VIN
}

// PairVINResult is how many boxes were asked and linked. It never writes Last Reading.
type PairVINResult struct {
	Asked        int
	Linked       int
	Already      int
	NoVIN        int
	NoRoster     int
	SkippedSteal int
	Links        []VINLink
}

// VINLink is one factory_id → Enterprise car via exact 17-char VIN.
type VINLink struct {
	FactoryID string
	DeviceID  string
	VIN       string
	EFleetsID string
}

func (r PairVINResult) Format() string {
	return fmt.Sprintf("vin-pair linked=%d asked=%d already=%d no_vin=%d no_roster=%d skipped_existing_map=%d\n",
		r.Linked, r.Asked, r.Already, r.NoVIN, r.NoRoster, r.SkippedSteal)
}

// PairDevicesByVIN asks OneStep what VIN is plugged into a GPS box, then joins
// that factory_id to the Enterprise car with the same cars.vin. Existing
// factory_id maps are kept. Duplicate roster VINs are not joined.
func (a *App) PairDevicesByVIN(ctx context.Context, opt PairVINOpts) (PairVINResult, error) {
	out := PairVINResult{}
	if a == nil || a.Store == nil {
		return out, fmt.Errorf("vin-pair: no store")
	}
	pace := opt.Pace
	if pace <= 0 {
		pace = watchDriveStopMinInterval
	}
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return out, err
	}
	vinToCar := onestep.VINToEFleets(cars)
	stored, err := a.Store.ListDevices(ctx)
	if err != nil {
		return out, err
	}
	want := map[string]struct{}{}
	for _, id := range opt.FactoryIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	byFactory := map[string]model.OneStepDevice{}
	for _, d := range stored {
		if id := strings.TrimSpace(d.FactoryID); id != "" {
			byFactory[id] = d
		}
	}
	for _, d := range opt.Live {
		id := strings.TrimSpace(d.FactoryID)
		if id == "" {
			continue
		}
		old := byFactory[id]
		if strings.TrimSpace(d.DeviceID) != "" {
			old.DeviceID = d.DeviceID
			old.FactoryID = id
		} else if old.FactoryID == "" {
			old.FactoryID = id
		}
		old.Active = d.Active
		old.Dead = d.Dead
		if strings.TrimSpace(d.DisplayName) != "" {
			old.DisplayName = d.DisplayName
		}
		if v := onestep.ValidVIN(d.VIN); v != "" {
			old.VIN = v
		}
		if old.LinkedCarEFleetsID == nil || strings.TrimSpace(*old.LinkedCarEFleetsID) == "" {
			old.LinkedCarEFleetsID = d.LinkedCarEFleetsID
		}
		byFactory[id] = old
	}

	apply := func(d model.OneStepDevice) (model.OneStepDevice, bool) {
		if oil.HasLogisticsPersonnel(d.DisplayName) {
			return d, false
		}
		if d.LinkedCarEFleetsID != nil && strings.TrimSpace(*d.LinkedCarEFleetsID) != "" {
			out.Already++
			return d, false
		}
		linked := onestep.LinkByVIN(d, vinToCar)
		if linked.LinkedCarEFleetsID == nil || strings.TrimSpace(*linked.LinkedCarEFleetsID) == "" {
			if onestep.ValidVIN(d.VIN) == "" {
				out.NoVIN++
			} else {
				out.NoRoster++
			}
			return d, false
		}
		out.Linked++
		out.Links = append(out.Links, VINLink{
			FactoryID: linked.FactoryID,
			DeviceID:  linked.DeviceID,
			VIN:       onestep.ValidVIN(linked.VIN),
			EFleetsID: strings.TrimSpace(*linked.LinkedCarEFleetsID),
		})
		return linked, true
	}

	var pending []model.OneStepDevice
	for _, d := range byFactory {
		if len(want) > 0 {
			if _, ok := want[d.FactoryID]; !ok {
				continue
			}
		}
		if d.Dead {
			continue
		}
		if d.LinkedCarEFleetsID != nil && strings.TrimSpace(*d.LinkedCarEFleetsID) != "" {
			out.Already++
			continue
		}
		if onestep.ValidVIN(d.VIN) != "" {
			linked, ok := apply(d)
			if ok {
				if err := a.Store.UpsertDevice(ctx, linked); err != nil {
					return out, err
				}
				byFactory[d.FactoryID] = linked
			}
			continue
		}
		pending = append(pending, d)
	}

	if !opt.AskEmpty || a.OneStep == nil || len(pending) == 0 {
		return out, nil
	}

	var lastCall time.Time
	gotFleet := false
	for i, d := range pending {
		if byFactory[d.FactoryID].LinkedCarEFleetsID != nil && strings.TrimSpace(*byFactory[d.FactoryID].LinkedCarEFleetsID) != "" {
			continue
		}
		if gotFleet && onestep.ValidVIN(byFactory[d.FactoryID].VIN) != "" {
			linked, ok := apply(byFactory[d.FactoryID])
			if ok {
				if err := a.Store.UpsertDevice(ctx, linked); err != nil {
					return out, err
				}
			}
			continue
		}
		if err := watchPaceDriveStop(ctx, lastCall, pace, 0); err != nil {
			return out, err
		}
		started := time.Now()
		vin, all, err := a.OneStep.AskDeviceVIN(ctx, d)
		lastCall = started
		out.Asked++
		if err != nil && isWatchRetryable(err) {
			wait := onestep.RetryAfterOf(err)
			if wait <= 0 {
				wait = pace
			}
			fmt.Fprintf(os.Stderr, "vin-pair Retry-After %s factory_id=%s: %v\n", wait, d.FactoryID, err)
			if werr := watchPaceDriveStop(ctx, lastCall, 0, wait); werr != nil {
				return out, werr
			}
			started = time.Now()
			vin, all, err = a.OneStep.AskDeviceVIN(ctx, d)
			lastCall = started
			out.Asked++
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "vin-pair factory_id %s: %v\n", d.FactoryID, err)
			out.NoVIN++
			continue
		}
		if len(all) > 1 {
			gotFleet = true
			for _, live := range all {
				id := strings.TrimSpace(live.FactoryID)
				if id == "" {
					continue
				}
				old := byFactory[id]
				if old.FactoryID == "" {
					old = live
				}
				if v := onestep.ValidVIN(live.VIN); v != "" {
					old.VIN = v
				}
				if strings.TrimSpace(live.DeviceID) != "" {
					old.DeviceID = live.DeviceID
				}
				old.FactoryID = id
				byFactory[id] = old
			}
		}
		if vin != "" {
			d.VIN = vin
		} else if v := onestep.ValidVIN(byFactory[d.FactoryID].VIN); v != "" {
			d.VIN = v
		}
		linked, ok := apply(d)
		if ok {
			if err := a.Store.UpsertDevice(ctx, linked); err != nil {
				return out, err
			}
			byFactory[d.FactoryID] = linked
			fmt.Fprintf(os.Stderr, "vin-pair factory_id=%s vin=%s car=%s\n", linked.FactoryID, onestep.ValidVIN(linked.VIN), *linked.LinkedCarEFleetsID)
		}
		fmt.Fprintf(os.Stderr, "vin-pair progress %d/%d linked=%d factory_id=%s\n", i+1, len(pending), out.Linked, d.FactoryID)
	}
	return out, nil
}

func overlayDeviceLinks(devs, stored []model.OneStepDevice) []model.OneStepDevice {
	link := map[string]*string{}
	vin := map[string]string{}
	for _, d := range stored {
		id := strings.TrimSpace(d.FactoryID)
		if id == "" {
			continue
		}
		if d.LinkedCarEFleetsID != nil && strings.TrimSpace(*d.LinkedCarEFleetsID) != "" {
			v := strings.TrimSpace(*d.LinkedCarEFleetsID)
			link[id] = &v
		}
		if x := onestep.ValidVIN(d.VIN); x != "" {
			vin[id] = x
		}
	}
	out := append([]model.OneStepDevice(nil), devs...)
	for i := range out {
		id := strings.TrimSpace(out[i].FactoryID)
		if v, ok := link[id]; ok {
			out[i].LinkedCarEFleetsID = v
		}
		if x, ok := vin[id]; ok {
			out[i].VIN = x
		}
	}
	return out
}
