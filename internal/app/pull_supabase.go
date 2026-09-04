package app

import (
	"context"
	"fmt"

	"oilchange/internal/model"
	"oilchange/internal/syncsupabase"
)

// PullStats is printed by oilchange pull-supabase. No DSNs or keys.
type PullStats struct {
	Cars, LastReading, LastOil, Holds, PDIMismatch, Devices int
	DevicesNote                                             string
}

// PullSupabase copies fleet_cars into sqlite. Last Reading is merged only when
// remote is non-null. Never calls WriteLastReading. Never invents miles.
func (a *App) PullSupabase(ctx context.Context) (PullStats, error) {
	var st PullStats
	cfg := syncsupabase.Config{
		URL:         a.Cfg.SupabaseURL,
		ServiceRole: a.Cfg.ServiceRole,
		AnonKey:     a.Cfg.SupabaseAnonKey,
	}
	rows, err := syncsupabase.FetchCars(ctx, cfg)
	if err != nil {
		return st, err
	}
	st.Cars = len(rows)
	for _, row := range rows {
		c := syncsupabase.CarFromRow(row)
		if c.EFleetsID == "" {
			continue
		}
		if err := a.Store.UpsertCar(ctx, c); err != nil {
			return st, fmt.Errorf("upsert car %s: %w", c.EFleetsID, err)
		}
		if err := a.Store.MergeCarRemote(ctx, c); err != nil {
			return st, fmt.Errorf("merge car %s: %w", c.EFleetsID, err)
		}
		if c.LastReadingMiles != nil {
			st.LastReading++
		}
		if c.LastOilMiles != nil {
			st.LastOil++
		}
		if c.HoldReason != nil && *c.HoldReason != "" {
			st.Holds++
		}
		existing, err := a.Store.CarByEFleets(ctx, c.EFleetsID)
		if err == nil && existing != nil && c.PDIID != "" && existing.PDIID != c.PDIID {
			st.PDIMismatch++
		}
	}
	if a.Cfg.ServiceRole == "" {
		st.DevicesNote = "skipped-no-service-role"
		return st, nil
	}
	devs, err := syncsupabase.FetchDevices(ctx, cfg)
	if err != nil {
		return st, err
	}
	for _, d := range devs {
		if d.FactoryID == "" {
			continue
		}
		md := model.OneStepDevice{
			FactoryID:          d.FactoryID,
			DeviceID:           d.DeviceID,
			DisplayName:        d.DisplayName,
			LinkedCarEFleetsID: d.LinkedCarEFleetsID,
			LinkedCarPDIID:     d.LinkedCarPDIID,
			Dead:               d.Dead,
			Active:             d.Active,
		}
		if !md.Dead {
			md.Active = true
		}
		if err := a.Store.UpsertDevice(ctx, md); err != nil {
			return st, fmt.Errorf("upsert device %s: %w", d.FactoryID, err)
		}
		st.Devices++
	}
	return st, nil
}
