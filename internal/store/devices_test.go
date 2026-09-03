package store

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

func TestUpsertDeviceByFactoryID(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dev.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}
	link := "27TESTA"
	if err := s.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT1", DeviceID: "DEV1", DisplayName: "Label A", LinkedCarEFleetsID: &link, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT1", DeviceID: "DEV1-NEW", DisplayName: "Label B", LinkedCarEFleetsID: &link, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "FACT1", DeviceID: "DEV1-LIVE", DisplayName: "Live API label", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDevice(ctx, "FACT1")
	if err != nil || got == nil {
		t.Fatalf("get: %v %+v", err, got)
	}
	if got.DeviceID != "DEV1-LIVE" {
		t.Fatalf("upsert must key by factory_id, device_id=%s", got.DeviceID)
	}
	if got.DisplayName != "Live API label" {
		t.Fatalf("label update %q", got.DisplayName)
	}
	if got.LinkedCarEFleetsID == nil || *got.LinkedCarEFleetsID != link {
		t.Fatalf("live API refresh erased factory_id pairing: %+v", got)
	}
	if got.LastSyncedAt == nil {
		t.Fatal("last_synced_at required")
	}
	if !got.Active || got.Dead {
		t.Fatalf("active/dead %+v", got)
	}
	all, err := s.ListDevices(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list %d %v", len(all), err)
	}
}

func TestDisplayNameIsNeverTheJoin(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dev.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", Nickname: "VA19"}); err != nil {
		t.Fatal(err)
	}

	_, file, _, _ := runtime.Caller(0)
	mapPath := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "onestep", "map_same_display_name.csv")
	devs, err := onestep.LoadMapCSV(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("want 2 rows with shared display_name, got %d", len(devs))
	}
	for _, d := range devs {
		if err := s.UpsertDevice(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ListDevices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("same display_name must not collapse rows, got %d", len(all))
	}
	byName := map[string]int{}
	for _, d := range all {
		byName[d.DisplayName]++
		if d.FactoryID == "SAME_B" && d.LinkedCarEFleetsID != nil {
			t.Fatalf("SAME_B must not inherit car link from shared display_name: %+v", d)
		}
		if d.FactoryID == "SAME_A" && (d.LinkedCarEFleetsID == nil || *d.LinkedCarEFleetsID != "27TESTA") {
			t.Fatalf("SAME_A links by factory map efleets_id: %+v", d)
		}
	}
	if byName["Shared Label"] != 2 {
		t.Fatalf("expected two Shared Label labels, got %#v", byName)
	}

	linked := onestep.LinkByFactoryID(
		model.OneStepDevice{FactoryID: "NOPE", DisplayName: "27TESTA"},
		map[string]string{"SAME_A": "27TESTA"},
	)
	if linked.LinkedCarEFleetsID != nil {
		t.Fatal("display_name must never be the join key")
	}
}
