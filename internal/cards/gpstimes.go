package cards

import (
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
)

// DefaultStopSlack is how far a swipe may sit outside a GPS stop window.
// Fuel happens at the pump while the box still reports stopped, or just after.
const DefaultStopSlack = 20 * time.Minute

// MaxPumpStop drops overnight/home sits. A fuel marker is a short stop.
const MaxPumpStop = 2 * time.Hour

// MatchByStopTimes assigns a swipe to a car when exactly one GPS-linked car
// was stopped (near a marker) at that swipe time. Two cars stopped → skip.
// Enterprise Vehicle is not the join. Last Reading does not use this.
func MatchByStopTimes(visits []model.StopVisit, txs []model.CardTx, slack time.Duration) []model.GPSCardMatch {
	if slack <= 0 {
		slack = DefaultStopSlack
	}
	type key struct{ card, car string }
	n := map[key]int{}
	stations := map[key]map[string]struct{}{}
	enterprise := map[key]map[string]struct{}{}
	for _, t := range txs {
		card := strings.TrimSpace(t.CardID)
		if card == "" || t.At.IsZero() {
			continue
		}
		var hit []string
		seen := map[string]struct{}{}
		for _, v := range visits {
			car := strings.TrimSpace(v.EFleetsID)
			if car == "" || isUnknownCar(car) {
				continue
			}
			if !stopCovers(v, t.At, slack) {
				continue
			}
			if _, ok := seen[car]; ok {
				continue
			}
			seen[car] = struct{}{}
			hit = append(hit, car)
		}
		if len(hit) != 1 {
			continue
		}
		k := key{card, hit[0]}
		n[k]++
		if stations[k] == nil {
			stations[k] = map[string]struct{}{}
		}
		if name := strings.TrimSpace(t.StationName); name != "" && !skipStationName(name) {
			stations[k][name] = struct{}{}
		}
		if enterprise[k] == nil {
			enterprise[k] = map[string]struct{}{}
		}
		if rec := strings.TrimSpace(t.RecordedEFleetsID); rec != "" && !isUnknownCar(rec) {
			enterprise[k][rec] = struct{}{}
		}
	}
	bestN := map[string]int{}
	bestCard := map[string]string{}
	for k, c := range n {
		if c > bestN[k.car] {
			bestN[k.car] = c
			bestCard[k.car] = k.card
		}
	}
	var out []model.GPSCardMatch
	for k, c := range n {
		m := model.GPSCardMatch{EFleetsID: k.car, CardID: k.card, EvidenceN: c, Best: bestCard[k.car] == k.card}
		for s := range stations[k] {
			m.Stations = append(m.Stations, s)
		}
		sort.Strings(m.Stations)
		for e := range enterprise[k] {
			m.EnterpriseCars = append(m.EnterpriseCars, e)
		}
		sort.Strings(m.EnterpriseCars)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Best != out[j].Best {
			return out[i].Best
		}
		if out[i].EvidenceN != out[j].EvidenceN {
			return out[i].EvidenceN > out[j].EvidenceN
		}
		return out[i].EFleetsID < out[j].EFleetsID
	})
	return out
}

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
