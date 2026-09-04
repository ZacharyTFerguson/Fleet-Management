package app

import (
	"context"
	"fmt"
	"time"

	"oilchange/internal/syncsupabase"
)

// SyncSupabase pushes cars/holds from local SQLite to the shared fleet Supabase
// project (fleet_cars) when SUPABASE_URL + (SUPABASE_SERVICE_ROLE or
// SUPABASE_SYNC_SECRET) are set, and always refreshes the local JSON mirror.
// Does not compute Last Reading. Never targets XRAY.
func (a *App) SyncSupabase(ctx context.Context, mirrorPath string, noRemote bool) (*syncsupabase.Snapshot, error) {
	cars, holds, err := a.Store.ListCarsAndOpenHolds(ctx)
	if err != nil {
		return nil, err
	}
	cfg := syncsupabase.Config{
		URL:         a.Cfg.SupabaseURL,
		ServiceRole: a.Cfg.ServiceRole,
		SyncSecret:  a.Cfg.SyncSecret,
		MirrorPath:  mirrorPath,
		NoRemote:    noRemote,
	}
	snap, err := syncsupabase.Run(ctx, cfg, syncsupabase.FromCars(cars), syncsupabase.FromHolds(holds), nil)
	if err != nil {
		return nil, err
	}
	fmt.Printf("sync %s cars=%d holds=%d at %s\n",
		snap.Source, len(snap.Cars), len(snap.Holds), snap.SyncedAt.Format(time.RFC3339))
	return snap, nil
}
