package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/store"
	"oilchange/internal/syncsupabase"
)

// heldCarStore is a roster car with a trusted fill and no GPS box: NO_DEVICE on
// every compute tick, which is the state of 55 live cars.
func heldCarStore(t *testing.T, name string) *store.Store {
	t.Helper()
	st, err := store.Open("sqlite", filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "CAR1", Nickname: "VA1"}); err != nil {
		t.Fatal(err)
	}
	odo := 100000
	if err := st.UpsertFill(ctx, model.Fill{
		EFleetsID:                    "CAR1",
		ProviderCompanyVehicleNumber: "VA1",
		Odometer:                     &odo,
		ProviderTransactionTime:      time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		Source:                       model.SourceFuelDetails,
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

// Daily compute ticks on a car that stays NO_DEVICE must not grow `oilchange holds`
// by one line per tick; the HOLD count operators see must equal cars on HOLD.
func TestRepeatedComputeOnHeldCarKeepsOneOpenHold(t *testing.T) {
	st := heldCarStore(t, "ticks.sqlite")
	a := &App{Store: st}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		code, err := a.Compute(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if code != model.ExitHolds {
			t.Fatalf("tick %d exit %d want %d", i, code, model.ExitHolds)
		}
	}
	car, err := st.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if car.HoldReason == nil || *car.HoldReason != model.HoldNoDevice {
		t.Fatalf("hold_reason %+v", car.HoldReason)
	}
	if car.LastReadingMiles != nil {
		t.Fatalf("HOLD wrote a reading %d", *car.LastReadingMiles)
	}
	holds, err := st.OpenHolds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(holds) != 1 {
		t.Fatalf("one car on HOLD after 3 ticks, open hold_events=%d", len(holds))
	}
}

// `oilchange sync --interval` snapshots cars and open holds in two separate
// reads. A compute tick committing between them (cross-process on the same
// sqlite) yields a mirror whose cars[].hold_reason and holds[] disagree about the
// same car. Each snapshot must be internally consistent.
func TestSyncMirrorHoldsAgreeWithCarsUnderConcurrentCompute(t *testing.T) {
	st := heldCarStore(t, "sync-race.sqlite")
	a := &App{Cfg: config.Config{}, Store: st}
	mirror := filepath.Join(t.TempDir(), "cars.json")
	ctx := context.Background()

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = devnull
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = devnull.Close()
	})

	stop := make(chan struct{})
	var flappers sync.WaitGroup
	var flapErr atomic.Value
	for f := 0; f < 3; f++ {
		flappers.Add(1)
		go func(f int) {
			defer flappers.Done()
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				var err error
				if (i+f)%2 == 0 {
					err = st.SetHold(ctx, "CAR1", model.HoldNoDevice, "tick")
				} else {
					err = st.WriteLastReading(ctx, "CAR1", 100000+i, time.Now().UTC(), model.SourceFuelDetails)
				}
				if err != nil {
					flapErr.CompareAndSwap(nil, err.Error())
					return
				}
				i++
			}
		}(f)
	}

	const rounds = 200
	inconsistent := 0
	var sample string
	for i := 0; i < rounds; i++ {
		snap, err := a.SyncSupabase(ctx, mirror, true)
		if err != nil {
			t.Fatal(err)
		}
		if msg := disagreement(snap); msg != "" {
			inconsistent++
			if sample == "" {
				sample = msg
			}
		}
	}
	close(stop)
	flappers.Wait()
	if v := flapErr.Load(); v != nil {
		t.Fatal(v)
	}
	if inconsistent > 0 {
		t.Fatalf("%d/%d mirrors disagree about HOLD (first: %s)", inconsistent, rounds, sample)
	}
}

// disagreement reports a car whose hold_reason and open holds[] rows tell different stories.
func disagreement(snap *syncsupabase.Snapshot) string {
	open := map[string]int{}
	for _, h := range snap.Holds {
		open[h.EFleetsID]++
	}
	for _, c := range snap.Cars {
		onHold := c.HoldReason != nil && *c.HoldReason != ""
		if onHold && open[c.EFleetsID] == 0 {
			return c.EFleetsID + " hold_reason=" + *c.HoldReason + " but no open hold event"
		}
		if !onHold && open[c.EFleetsID] > 0 {
			return c.EFleetsID + " has no hold_reason but open hold events"
		}
	}
	return ""
}
