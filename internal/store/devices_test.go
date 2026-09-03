package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"oilchange/internal/model"
)

func deviceFor(t *testing.T, s *Store, efleetsID, factoryID string) *model.OneStepDevice {
	t.Helper()
	devs, err := s.ListDevicesForCar(context.Background(), efleetsID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range devs {
		if devs[i].FactoryID == factoryID {
			return &devs[i]
		}
	}
	return nil
}

// TestUpsertDeviceCoalesceKeepLink pins the link rule: nil (API inventory) keeps
// the existing pairing, non-nil (device map) relinks, and nothing unlinks.
func TestUpsertDeviceCoalesceKeepLink(t *testing.T) {
	p := filepath.Join(t.TempDir(), "coalesce.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, id := range []string{"CARA", "CARB"} {
		if err := s.UpsertCar(ctx, model.Car{EFleetsID: id}); err != nil {
			t.Fatal(err)
		}
	}
	linkA, linkB, empty := "CARA", "CARB", ""

	// Inventory row first: no link known yet.
	if err := s.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F1", DeviceID: "D1"}); err != nil {
		t.Fatal(err)
	}
	if deviceFor(t, s, "CARA", "F1") != nil {
		t.Fatal("no map row yet, F1 must not belong to CARA")
	}
	// Map row links.
	if err := s.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F1", DeviceID: "D1", LinkedCarEFleetsID: &linkA}); err != nil {
		t.Fatal(err)
	}
	// Inventory refresh (nil link) keeps the pairing but refreshes device_id.
	if err := s.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F1", DeviceID: "D1-v2"}); err != nil {
		t.Fatal(err)
	}
	got := deviceFor(t, s, "CARA", "F1")
	if got == nil || got.DeviceID != "D1-v2" {
		t.Fatalf("nil link must keep CARA and still refresh device_id: %+v", got)
	}
	// Empty-string link is "unknown", not "unlink".
	if err := s.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F1", DeviceID: "D1-v2", LinkedCarEFleetsID: &empty}); err != nil {
		t.Fatal(err)
	}
	if deviceFor(t, s, "CARA", "F1") == nil {
		t.Fatal("empty link must not unlink")
	}
	// A map row for a different car relinks (box moved to another car).
	if err := s.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F1", DeviceID: "D1-v2", LinkedCarEFleetsID: &linkB}); err != nil {
		t.Fatal(err)
	}
	if deviceFor(t, s, "CARA", "F1") != nil || deviceFor(t, s, "CARB", "F1") == nil {
		t.Fatal("non-nil link must relink to CARB")
	}
	// Marking dead keeps the link (history) but the box stops counting.
	if err := s.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F1", DeviceID: "D1-v2", Dead: true}); err != nil {
		t.Fatal(err)
	}
	got = deviceFor(t, s, "CARB", "F1")
	if got == nil || !got.Dead {
		t.Fatalf("dead box keeps its link for history: %+v", got)
	}
}

func TestUpsertDevicesIsAllOrNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "batch.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CARA"}); err != nil {
		t.Fatal(err)
	}
	linkA, ghost := "CARA", "NOSUCHCAR"
	if err := s.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "F1", DeviceID: "OLD", LinkedCarEFleetsID: &linkA}); err != nil {
		t.Fatal(err)
	}
	err = s.UpsertDevices(ctx, []model.OneStepDevice{
		{FactoryID: "F1", DeviceID: "NEW"},
		{FactoryID: "F2", DeviceID: "D2", LinkedCarEFleetsID: &linkA},
		{FactoryID: "F3", DeviceID: "D3", LinkedCarEFleetsID: &ghost}, // FK violation
	})
	if err == nil {
		t.Fatal("map row linking a car that does not exist must fail the import")
	}
	if !strings.Contains(err.Error(), "F3") {
		t.Fatalf("error should name the offending factory_id: %v", err)
	}
	devs, err := s.ListDevicesForCar(ctx, "CARA")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].FactoryID != "F1" || devs[0].DeviceID != "OLD" {
		t.Fatalf("partial import applied: %+v", devs)
	}
	// The same batch without the bad row applies as a unit.
	if err := s.UpsertDevices(ctx, []model.OneStepDevice{
		{FactoryID: "F1", DeviceID: "NEW"},
		{FactoryID: "F2", DeviceID: "D2", LinkedCarEFleetsID: &linkA},
	}); err != nil {
		t.Fatal(err)
	}
	devs, _ = s.ListDevicesForCar(ctx, "CARA")
	if len(devs) != 2 {
		t.Fatalf("batch upsert: %+v", devs)
	}
	for _, d := range devs {
		if d.FactoryID == "F1" && d.DeviceID != "NEW" {
			t.Fatalf("F1 not refreshed: %+v", d)
		}
	}
	if err := s.UpsertDevices(ctx, nil); err != nil {
		t.Fatalf("empty batch is a no-op: %v", err)
	}
}
