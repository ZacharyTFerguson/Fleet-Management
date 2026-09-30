package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/store"
	"oilchange/internal/syncsupabase"
)

func TestPullSupabaseMergesNonNullAndKeepsLocalReading(t *testing.T) {
	remoteMiles := 100500
	remoteHold := "NO_TRUSTED_FILL"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]syncsupabase.CarRow{
			{PDIID: "PDI-0099", EFleetsID: "KEEP", Nickname: "KeepMe"},
			{PDIID: "PDI-0002", EFleetsID: "SEED", Nickname: "SeedMe", LastReadingMiles: &remoteMiles, HoldReason: &remoteHold},
		})
	}))
	defer srv.Close()

	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := t.Context()
	localMiles := 111111
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "KEEP", Nickname: "KeepMe"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := st.WriteLastReading(ctx, "KEEP", localMiles, at, model.SourceFuelDetails); err != nil {
		t.Fatal(err)
	}
	if err := st.SetHold(ctx, "KEEP", model.HoldNoDevice, "local hold"); err != nil {
		t.Fatal(err)
	}

	a := &App{Cfg: config.Config{SupabaseURL: srv.URL, SupabaseAnonKey: "anon"}, Store: st}
	stats, err := a.PullSupabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Cars != 2 {
		t.Fatalf("cars %d", stats.Cars)
	}

	keep, err := st.CarByEFleets(ctx, "KEEP")
	if err != nil {
		t.Fatal(err)
	}
	if keep.LastReadingMiles == nil || *keep.LastReadingMiles != localMiles {
		t.Fatalf("remote null must not wipe local last reading: %+v", keep.LastReadingMiles)
	}
	if keep.HoldReason == nil || *keep.HoldReason != model.HoldNoDevice {
		t.Fatalf("remote null must not wipe local HOLD: %+v", keep.HoldReason)
	}

	seed, err := st.CarByEFleets(ctx, "SEED")
	if err != nil {
		t.Fatal(err)
	}
	if seed.LastReadingMiles == nil || *seed.LastReadingMiles != remoteMiles {
		t.Fatalf("expected copied last reading %+v", seed.LastReadingMiles)
	}
	if seed.HoldReason == nil || *seed.HoldReason != remoteHold {
		t.Fatalf("expected copied hold %+v", seed.HoldReason)
	}
}

func TestPullSupabaseDoesNotRegressFresherLocalLastReading(t *testing.T) {
	staleMiles := 140000
	staleAt := "2026-08-01T12:00:00Z"
	staleHold := "NO_DRIVESTOP"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]syncsupabase.CarRow{
			{
				PDIID:            "PDI-0001",
				EFleetsID:        "27TESTA",
				Nickname:         "VA19",
				LastReadingMiles: &staleMiles,
				LastReadingAt:    &staleAt,
				HoldReason:       &staleHold,
			},
		})
	}))
	defer srv.Close()

	p := filepath.Join(t.TempDir(), "oil.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := t.Context()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	localAt := time.Date(2026, 9, 10, 14, 0, 0, 0, time.UTC)
	if err := st.WriteLastReading(ctx, "27TESTA", 150000, localAt, model.SourceFuelDetails); err != nil {
		t.Fatal(err)
	}

	a := &App{Cfg: config.Config{SupabaseURL: srv.URL, SupabaseAnonKey: "anon"}, Store: st}
	if _, err := a.PullSupabase(ctx); err != nil {
		t.Fatal(err)
	}

	car, err := st.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if car.LastReadingMiles == nil || *car.LastReadingMiles != 150000 {
		t.Fatalf("pull-supabase must not clobber fresher local last reading with stale remote: %+v", car.LastReadingMiles)
	}
	if car.HoldReason != nil {
		t.Fatalf("pull-supabase must not resurrect stale remote HOLD after local compute: %+v", car.HoldReason)
	}
}
