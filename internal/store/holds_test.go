package store

import (
	"context"
	"path/filepath"
	"testing"

	"oilchange/internal/model"
)

// openHoldsFor is the operator view (`oilchange holds`) narrowed to one car.
func openHoldsFor(t *testing.T, s *Store, efleetsID string) []model.HoldEvent {
	t.Helper()
	all, err := s.OpenHolds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []model.HoldEvent
	for _, h := range all {
		if h.EFleetsID == efleetsID {
			out = append(out, h)
		}
	}
	return out
}

// A car that stays on HOLD across compute ticks is one HOLD, not one per tick.
// STATUS.md: 236 open hold_events for 59 cars. `oilchange holds` must agree with cars.hold_reason.
func TestRepeatedSetHoldDoesNotStackOpenEvents(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "stack.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1", Nickname: "VA1"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.SetHold(ctx, "CAR1", model.HoldNoDevice, "no live factory_id linked; dead boxes are not summed"); err != nil {
			t.Fatal(err)
		}
	}
	car, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if car.HoldReason == nil || *car.HoldReason != model.HoldNoDevice {
		t.Fatalf("hold_reason %+v", car.HoldReason)
	}
	if car.LastReadingMiles != nil {
		t.Fatalf("HOLD must not seed a reading, got %d", *car.LastReadingMiles)
	}
	open := openHoldsFor(t, s, "CAR1")
	if len(open) != 1 {
		t.Fatalf("one car on HOLD must be one open hold_event, got %d", len(open))
	}
}

// When the reason changes (NO_DRIVESTOP resolved, NO_DEVICE remains) the old open
// event must close, otherwise `oilchange holds` prints a HOLD cars.hold_reason no longer has.
func TestSetHoldNewReasonSupersedesOpenEvent(t *testing.T) {
	s, err := Open("sqlite", filepath.Join(t.TempDir(), "supersede.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.UpsertCar(ctx, model.Car{EFleetsID: "CAR1", Nickname: "VA1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHold(ctx, "CAR1", model.HoldNoDriveStop, "no drive-stop miles after the trusted second"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHold(ctx, "CAR1", model.HoldNoDevice, "no live factory_id linked"); err != nil {
		t.Fatal(err)
	}
	car, err := s.CarByEFleets(ctx, "CAR1")
	if err != nil {
		t.Fatal(err)
	}
	if car.HoldReason == nil || *car.HoldReason != model.HoldNoDevice {
		t.Fatalf("hold_reason %+v", car.HoldReason)
	}
	open := openHoldsFor(t, s, "CAR1")
	if len(open) != 1 {
		t.Fatalf("want exactly the current HOLD open, got %+v", open)
	}
	if open[0].Reason != model.HoldNoDevice {
		t.Fatalf("open event %s disagrees with cars.hold_reason %s", open[0].Reason, *car.HoldReason)
	}
}
