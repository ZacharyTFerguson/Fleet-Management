package cards

import (
	"math"
	"strings"
	"time"

	"oilchange/internal/model"
)

// FuelJumpWindow is how far before/after the fill second we sample the gauge.
const FuelJumpWindow = time.Hour

// FuelSampleSlack is how close a reading must be to fill±1h. A now-snapshot
// from GET /device?latest_point=true is not in this window for a past fill.
const FuelSampleSlack = DefaultStopSlack

// FuelPoint is a gauge reading keyed by factory_id. Not Last Reading. Not odo.
type FuelPoint struct {
	FactoryID string
	At        time.Time
	Level     float64
}

// CollisionFuel is one linked box’s fill±1h gauge plus drive-stop miles in
// that same window (distance.value, never odometer_from / odometer_to).
type CollisionFuel struct {
	FactoryID string
	Before    float64
	After     float64
	HasBefore bool
	HasAfter  bool
	Miles     float64
	HasMiles  bool
}

// FuelJumpLook is an injected series. Production GPS-first passes nil — we do
// not invent fuel from device-information JSON or gps-stops.
type FuelJumpLook struct {
	LevelAt func(factoryID string, at time.Time) (level float64, ok bool)
	Miles   func(factoryID string, from, to time.Time) (miles float64, ok bool)
}

// PickFuelJumpCollision names the factory_id whose fuel rose uniquely after
// controlling for miles. Exactly two linked boxes. Fail closed when fuel or
// miles are missing, both rose, or both fell. Does not invent MPG or tank size:
// a drop is a drive-down (not a fill); a unique rise is the fill jump.
func PickFuelJumpCollision(a, b CollisionFuel) (factoryID string, ok bool) {
	if !collisionFuelReady(a) || !collisionFuelReady(b) {
		return "", false
	}
	if strings.TrimSpace(a.FactoryID) == strings.TrimSpace(b.FactoryID) {
		return "", false
	}
	ra, rb := fuelRose(a), fuelRose(b)
	if ra && !rb {
		return strings.TrimSpace(a.FactoryID), true
	}
	if rb && !ra {
		return strings.TrimSpace(b.FactoryID), true
	}
	return "", false
}

func collisionFuelReady(c CollisionFuel) bool {
	fid := strings.TrimSpace(c.FactoryID)
	if fid == "" || !c.HasBefore || !c.HasAfter || !c.HasMiles {
		return false
	}
	return finiteNonNeg(c.Before) && finiteNonNeg(c.After) && finiteNonNeg(c.Miles)
}

func fuelRose(c CollisionFuel) bool {
	return c.After > c.Before
}

func finiteNonNeg(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0
}

// NearestFuelLevel returns the sample for factory_id closest to at, within slack.
func NearestFuelLevel(series []FuelPoint, factoryID string, at time.Time, slack time.Duration) (float64, bool) {
	fid := strings.TrimSpace(factoryID)
	if fid == "" || slack < 0 {
		return 0, false
	}
	at = at.UTC()
	bestOK := false
	var best FuelPoint
	var bestDist time.Duration
	for _, p := range series {
		if strings.TrimSpace(p.FactoryID) != fid || p.At.IsZero() || !finiteNonNeg(p.Level) {
			continue
		}
		d := p.At.UTC().Sub(at)
		if d < 0 {
			d = -d
		}
		if d > slack {
			continue
		}
		if !bestOK || d < bestDist || (d == bestDist && p.At.UTC().Before(best.At.UTC())) {
			bestOK = true
			best = p
			bestDist = d
		}
	}
	return best.Level, bestOK
}

// SeriesFuelLook builds a look from synthetic (or saved) points. Miles must be
// drive-stop distance for that factory_id in the fill±1h window; missing keys
// fail closed. display_name is never a key.
func SeriesFuelLook(series []FuelPoint, miles map[string]float64) *FuelJumpLook {
	return &FuelJumpLook{
		LevelAt: func(factoryID string, at time.Time) (float64, bool) {
			return NearestFuelLevel(series, factoryID, at, FuelSampleSlack)
		},
		Miles: func(factoryID string, from, to time.Time) (float64, bool) {
			if miles == nil {
				return 0, false
			}
			n, ok := miles[strings.TrimSpace(factoryID)]
			if !ok || !finiteNonNeg(n) {
				return 0, false
			}
			return n, true
		},
	}
}

func collisionFuelOf(v model.StopVisit, fill time.Time, fuel *FuelJumpLook) CollisionFuel {
	cf := CollisionFuel{FactoryID: strings.TrimSpace(v.FactoryID)}
	if fuel == nil || cf.FactoryID == "" || fill.IsZero() {
		return cf
	}
	beforeAt := fill.UTC().Add(-FuelJumpWindow)
	afterAt := fill.UTC().Add(FuelJumpWindow)
	if fuel.LevelAt != nil {
		if n, ok := fuel.LevelAt(cf.FactoryID, beforeAt); ok {
			cf.Before, cf.HasBefore = n, true
		}
		if n, ok := fuel.LevelAt(cf.FactoryID, afterAt); ok {
			cf.After, cf.HasAfter = n, true
		}
	}
	if fuel.Miles != nil {
		if n, ok := fuel.Miles(cf.FactoryID, beforeAt, afterAt); ok {
			cf.Miles, cf.HasMiles = n, true
		}
	}
	return cf
}

// pumpCollisionPair is two linked boxes both sitting inside the exclusive-sit
// radius of each other. Fuel-gauge is not a substitute for that 350 m rule.
func pumpCollisionPair(a, b model.StopVisit) bool {
	if !a.HasPos || !b.HasPos {
		return false
	}
	if strings.TrimSpace(a.FactoryID) == "" || strings.TrimSpace(b.FactoryID) == "" {
		return false
	}
	if strings.TrimSpace(a.FactoryID) == strings.TrimSpace(b.FactoryID) {
		return false
	}
	return metersBetween(a.Lat, a.Lng, b.Lat, b.Lng) <= StationRadiusMeters
}

func disambiguatePumpCollision(cands []model.StopVisit, fill time.Time, fuel *FuelJumpLook) []model.StopVisit {
	if fuel == nil || len(cands) != 2 {
		return cands
	}
	if !pumpCollisionPair(cands[0], cands[1]) {
		return cands
	}
	fid, ok := PickFuelJumpCollision(
		collisionFuelOf(cands[0], fill, fuel),
		collisionFuelOf(cands[1], fill, fuel),
	)
	if !ok {
		return cands
	}
	for _, v := range cands {
		if strings.TrimSpace(v.FactoryID) == fid {
			return []model.StopVisit{v}
		}
	}
	return cands
}
