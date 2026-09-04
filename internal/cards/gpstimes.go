package cards

import (
	"time"

	"oilchange/internal/model"
)

// DefaultStopSlack is how far a swipe may sit outside a GPS stop window.
// Fuel happens at the pump while the box still reports stopped, or just after.
const DefaultStopSlack = 20 * time.Minute

// MaxPumpStop drops overnight/home sits. A fuel marker is a short stop.
const MaxPumpStop = 2 * time.Hour

func stopCovers(v model.StopVisit, at time.Time, slack time.Duration) bool {
	if v.From.IsZero() || v.To.IsZero() {
		return false
	}
	sit := v.To.Sub(v.From)
	if sit < 0 || sit > MaxPumpStop {
		return false
	}
	at = at.UTC()
	from := v.From.UTC().Add(-slack)
	to := v.To.UTC().Add(slack)
	return !at.Before(from) && !at.After(to)
}
