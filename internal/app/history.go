package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/history"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

// HistoryBoard is the turnstile payload for /api/history.
func (a *App) HistoryBoard(ctx context.Context, region string) (history.Board, error) {
	empty := history.Board{Unassigned: []history.FillBlock{}, Cars: []history.CarColumn{}, Regions: []string{}}
	if a == nil || a.Store == nil {
		return empty, fmt.Errorf("history: no store")
	}
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return empty, err
	}
	txs, err := a.Store.ListCardTxs(ctx, "")
	if err != nil {
		return empty, err
	}
	asg, err := a.Store.ListAssignments(ctx)
	if err != nil {
		return empty, err
	}
	return history.BuildBoard(cars, txs, asg, region, time.Now().UTC()), nil
}

// AssignFill moves one transaction. Empty toEFleets unassigns (undo).
func (a *App) AssignFill(ctx context.Context, txKey, toEFleets, reason, region string) (history.Board, error) {
	empty := history.Board{}
	if a == nil || a.Store == nil {
		return empty, fmt.Errorf("history: no store")
	}
	txKey = strings.TrimSpace(txKey)
	if txKey == "" {
		return empty, fmt.Errorf("history: tx_key required")
	}
	toEFleets = strings.TrimSpace(toEFleets)
	toPDI := ""
	if toEFleets != "" {
		car, err := a.Store.CarByEFleets(ctx, toEFleets)
		if err != nil || car == nil {
			return empty, fmt.Errorf("history: unknown car %s", toEFleets)
		}
		toPDI = car.PDIID
	}
	if reason == "" {
		if toEFleets == "" {
			reason = "undo"
		} else {
			reason = "manual_drag"
		}
	}
	if _, err := a.Store.AssignTx(ctx, txKey, toEFleets, toPDI, "owner", reason); err != nil {
		return empty, err
	}
	return a.HistoryBoard(ctx, region)
}

// FillEvidence is cached GPS for one swipe (Devices page).
func (a *App) FillEvidence(ctx context.Context, txKey string) (history.FillEvidence, error) {
	empty := history.FillEvidence{Nearby: []history.NearbyStop{}}
	tx, err := a.findTx(ctx, txKey)
	if err != nil {
		return empty, err
	}
	asg, err := a.Store.GetAssignment(ctx, tx.Key())
	if err != nil {
		return empty, err
	}
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return empty, err
	}
	visits, _ := cards.LoadStopVisits(a.gpsStopsCacheFile())
	return history.BuildFillEvidence(tx, asg, visits, cars), nil
}

// BoxEvidence is one GPS box from the device registry plus cache stop count.
func (a *App) BoxEvidence(ctx context.Context, factoryID, deviceID string) (history.BoxEvidence, error) {
	empty := history.BoxEvidence{}
	factoryID = strings.TrimSpace(factoryID)
	deviceID = strings.TrimSpace(deviceID)
	devs, err := a.Store.ListDevices(ctx)
	if err != nil {
		return empty, err
	}
	var d model.OneStepDevice
	found := false
	for _, x := range devs {
		if factoryID != "" && x.FactoryID == factoryID {
			d, found = x, true
			break
		}
		if deviceID != "" && x.DeviceID == deviceID {
			d, found = x, true
			break
		}
	}
	if !found {
		return empty, fmt.Errorf("devices: box not found")
	}
	out := history.BoxEvidence{
		FactoryID: d.FactoryID, DeviceID: d.DeviceID, DisplayName: d.DisplayName,
		VIN: d.VIN, Active: d.Active,
	}
	if d.LinkedCarEFleetsID != nil {
		out.LinkedCarEFleetsID = *d.LinkedCarEFleetsID
	}
	if d.LinkedCarPDIID != nil {
		out.LinkedCarPDIID = *d.LinkedCarPDIID
	}
	if visits, err := cards.LoadStopVisits(a.gpsStopsCacheFile()); err == nil {
		for _, v := range visits {
			if v.FactoryID == d.FactoryID {
				out.StopsInCache++
			}
		}
	}
	return out, nil
}

// ProbeOneBox is the v1 live GPS path: exactly one device_id, one window.
// Prefer a saved cache / VIN file when OneStep is cooling down.
func (a *App) ProbeOneBox(ctx context.Context, factoryID, deviceID, txKey string, hours int) (onestep.ProbeResult, error) {
	empty := onestep.ProbeResult{}
	if hours <= 0 {
		hours = 48
	}
	if hours > 48 {
		hours = 48
	}
	id := strings.TrimSpace(deviceID)
	if id == "" {
		resolved, err := a.oneBoxDeviceID(ctx, factoryID, txKey)
		if err != nil {
			return empty, err
		}
		id = resolved
	}
	c := a.oneStepClient()
	if c == nil {
		return empty, fmt.Errorf("devices probe: OneStep token missing — use saved Device Information when cooling down")
	}
	to := time.Now().UTC()
	from := to.Add(-time.Duration(hours) * time.Hour)
	if strings.TrimSpace(txKey) != "" {
		if tx, err := a.findTx(ctx, txKey); err == nil && !tx.At.IsZero() {
			from = tx.At.UTC().Add(-time.Hour)
			to = tx.At.UTC().Add(time.Hour)
		}
	}
	return c.ProbeDriveStop(ctx, id, from, to)
}

func (a *App) findTx(ctx context.Context, txKey string) (model.CardTx, error) {
	txKey = strings.TrimSpace(txKey)
	if txKey == "" {
		return model.CardTx{}, fmt.Errorf("tx_key required")
	}
	txs, err := a.Store.ListCardTxs(ctx, "")
	if err != nil {
		return model.CardTx{}, err
	}
	for _, t := range txs {
		if t.Key() == txKey {
			return t, nil
		}
	}
	return model.CardTx{}, fmt.Errorf("fill %s not found", txKey)
}

func (a *App) oneBoxDeviceID(ctx context.Context, factoryID, txKey string) (string, error) {
	factoryID = strings.TrimSpace(factoryID)
	devs, err := a.Store.ListDevices(ctx)
	if err != nil {
		return "", err
	}
	if factoryID != "" {
		for _, d := range devs {
			if d.FactoryID == factoryID && strings.TrimSpace(d.DeviceID) != "" {
				return d.DeviceID, nil
			}
		}
		return "", fmt.Errorf("devices probe: no device_id for factory_id %s", factoryID)
	}
	if strings.TrimSpace(txKey) == "" {
		return "", fmt.Errorf("devices probe: device_id, factory_id, or tx_key required")
	}
	tx, err := a.findTx(ctx, txKey)
	if err != nil {
		return "", err
	}
	asg, _ := a.Store.GetAssignment(ctx, tx.Key())
	car := firstNonEmpty(asg.AssignedEFleetsID, asg.GPSCalledEFleetsID, tx.CalledEFleetsID, tx.RecordedEFleetsID)
	if car == "" {
		return "", fmt.Errorf("devices probe: fill has no car to pick a box")
	}
	for _, d := range devs {
		if d.LinkedCarEFleetsID != nil && *d.LinkedCarEFleetsID == car && strings.TrimSpace(d.DeviceID) != "" {
			return d.DeviceID, nil
		}
	}
	return "", fmt.Errorf("devices probe: no linked box for car %s", car)
}
