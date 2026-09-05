package cards

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestPickFuelJumpCollisionUniqueRise(t *testing.T) {
	a := CollisionFuel{FactoryID: "FACT-A", Before: 20, After: 80, HasBefore: true, HasAfter: true, Miles: 4, HasMiles: true}
	b := CollisionFuel{FactoryID: "FACT-B", Before: 60, After: 45, HasBefore: true, HasAfter: true, Miles: 12, HasMiles: true}
	got, ok := PickFuelJumpCollision(a, b)
	if !ok || got != "FACT-A" {
		t.Fatalf("unique rise must name FACT-A, got %q ok=%v", got, ok)
	}
}

func TestPickFuelJumpCollisionDriveDownIsNotFill(t *testing.T) {
	// Miles in the window explain the drop; do not invent MPG to "add back" a fill.
	drop := CollisionFuel{FactoryID: "FACT-DROP", Before: 70, After: 40, HasBefore: true, HasAfter: true, Miles: 80, HasMiles: true}
	rise := CollisionFuel{FactoryID: "FACT-FILL", Before: 15, After: 90, HasBefore: true, HasAfter: true, Miles: 2, HasMiles: true}
	got, ok := PickFuelJumpCollision(drop, rise)
	if !ok || got != "FACT-FILL" {
		t.Fatalf("drive-down must lose to the unique rise: %q ok=%v", got, ok)
	}
}

func TestPickFuelJumpCollisionFailClosed(t *testing.T) {
	ready := CollisionFuel{FactoryID: "FACT-A", Before: 10, After: 80, HasBefore: true, HasAfter: true, Miles: 1, HasMiles: true}
	cases := []struct {
		name string
		a, b CollisionFuel
	}{
		{"missing fuel", ready, CollisionFuel{FactoryID: "FACT-B", Miles: 1, HasMiles: true}},
		{"missing miles", ready, CollisionFuel{FactoryID: "FACT-B", Before: 10, After: 5, HasBefore: true, HasAfter: true}},
		{"both rose", ready, CollisionFuel{FactoryID: "FACT-B", Before: 12, After: 70, HasBefore: true, HasAfter: true, Miles: 3, HasMiles: true}},
		{"both fell", CollisionFuel{FactoryID: "FACT-A", Before: 80, After: 40, HasBefore: true, HasAfter: true, Miles: 10, HasMiles: true}, CollisionFuel{FactoryID: "FACT-B", Before: 70, After: 50, HasBefore: true, HasAfter: true, Miles: 8, HasMiles: true}},
		{"empty factory_id", CollisionFuel{Before: 10, After: 90, HasBefore: true, HasAfter: true, Miles: 0, HasMiles: true}, CollisionFuel{FactoryID: "FACT-B", Before: 10, After: 5, HasBefore: true, HasAfter: true, Miles: 1, HasMiles: true}},
		{"same factory_id", ready, CollisionFuel{FactoryID: "FACT-A", Before: 90, After: 10, HasBefore: true, HasAfter: true, Miles: 1, HasMiles: true}},
		{"invented negative miles", ready, CollisionFuel{FactoryID: "FACT-B", Before: 90, After: 10, HasBefore: true, HasAfter: true, Miles: -3, HasMiles: true}},
	}
	for _, tc := range cases {
		if got, ok := PickFuelJumpCollision(tc.a, tc.b); ok || got != "" {
			t.Fatalf("%s: want unnamed, got %q ok=%v", tc.name, got, ok)
		}
	}
}

func TestPickFuelJumpCollisionDoesNotInventFuelOrMiles(t *testing.T) {
	zero := CollisionFuel{}
	if fid, ok := PickFuelJumpCollision(zero, zero); ok || fid != "" {
		t.Fatalf("empty series must not invent a winner: %q ok=%v", fid, ok)
	}
}

func TestNearestFuelLevelIgnoresFarReadings(t *testing.T) {
	fill := time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
	series := []FuelPoint{
		{FactoryID: "FACT-A", At: fill.Add(-3 * time.Hour), Level: 99},
		{FactoryID: "FACT-A", At: fill.Add(-time.Hour), Level: 22},
		{FactoryID: "FACT-B", At: fill.Add(-time.Hour), Level: 50},
	}
	n, ok := NearestFuelLevel(series, "FACT-A", fill.Add(-time.Hour), FuelSampleSlack)
	if !ok || n != 22 {
		t.Fatalf("want 22 at fill-1h, got %v ok=%v", n, ok)
	}
	if _, ok := NearestFuelLevel(series, "FACT-A", fill.Add(time.Hour), FuelSampleSlack); ok {
		t.Fatal("must not invent fill+1h from a fill-3h reading")
	}
	if _, ok := NearestFuelLevel(series, "display-name", fill.Add(-time.Hour), FuelSampleSlack); ok {
		t.Fatal("display_name is never a fuel join")
	}
}

func TestSeriesFuelLookSamplesFillPlusMinusHour(t *testing.T) {
	fill := fuelJumpFillAt
	look := SeriesFuelLook([]FuelPoint{
		{FactoryID: "FACT-A", At: fill.Add(-time.Hour).Add(2 * time.Minute), Level: 18},
		{FactoryID: "FACT-A", At: fill.Add(time.Hour).Add(-3 * time.Minute), Level: 85},
	}, map[string]float64{"FACT-A": 6})
	cf := collisionFuelOf(model.StopVisit{FactoryID: "FACT-A"}, fill, look)
	if !cf.HasBefore || !cf.HasAfter || !cf.HasMiles || cf.Before != 18 || cf.After != 85 || cf.Miles != 6 {
		t.Fatalf("synthetic series around fill±1h: %+v", cf)
	}
}

func TestStopVisitAndDeviceHaveNoFuelField(t *testing.T) {
	for _, v := range []any{model.StopVisit{}, model.OneStepDevice{}} {
		rt := reflect.TypeOf(v)
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			blob := strings.ToLower(f.Name + " " + string(f.Tag))
			if strings.Contains(blob, "fuel") {
				t.Fatalf("%s must not carry fuel (do not invent a gauge): %s %s", rt.Name(), f.Name, f.Tag)
			}
		}
	}
}

var fuelJumpFillAt = time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
