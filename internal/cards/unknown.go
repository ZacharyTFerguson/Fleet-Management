package cards

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
)

const ambiguousRatio = 0.8

// StationSummary is one mapped pump from swipe history. Name+address is the key.
type StationSummary struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Swipes  int    `json:"swipes"`
	Cars    int    `json:"cars"`
	Cards   int    `json:"cards"`
}

// Neighbor is another car/card at the same station within the trace window.
type Neighbor struct {
	EFleetsID string    `json:"efleets_id"`
	CardID    string    `json:"card_id"`
	Station   string    `json:"station"`
	DaysApart int       `json:"days_apart"`
	At        time.Time `json:"at"`
}

// UnknownMatchup is a card (or car) that is not a clean 1:1 pairing yet.
// Kinds: suspect (Enterprise latest ≠ swipe majority), ambiguous (two cars close),
// singleton (one swipe — start here once stations are mapped).
type UnknownMatchup struct {
	Kind          string     `json:"kind"`
	CardID        string     `json:"card_id"`
	EnterpriseCar string     `json:"enterprise_car"`
	BestCar       string     `json:"best_car"`
	BestN         int        `json:"best_n"`
	BestScore     float64    `json:"best_score"`
	RunnerUpCar   string     `json:"runner_up_car,omitempty"`
	RunnerUpN     int        `json:"runner_up_n,omitempty"`
	LatestStation string     `json:"latest_station"`
	LatestAt      time.Time  `json:"latest_at"`
	Neighbors     []Neighbor `json:"neighbors,omitempty"`
	Why           string     `json:"why"`
}

// MapStations rolls swipe history into pumps. Empty name+address is dropped.
func MapStations(txs []model.CardTx) []StationSummary {
	type acc struct {
		name, addr string
		swipes     int
		cars       map[string]struct{}
		cards      map[string]struct{}
	}
	by := map[string]*acc{}
	for _, t := range txs {
		k := stationKey(t.StationName, t.StationAddress)
		if k == "" {
			continue
		}
		a := by[k]
		if a == nil {
			a = &acc{name: strings.TrimSpace(t.StationName), addr: strings.TrimSpace(t.StationAddress), cars: map[string]struct{}{}, cards: map[string]struct{}{}}
			by[k] = a
		}
		a.swipes++
		if car := strings.TrimSpace(t.RecordedEFleetsID); !isUnknownCar(car) {
			a.cars[car] = struct{}{}
		}
		if card := strings.TrimSpace(t.CardID); card != "" {
			a.cards[card] = struct{}{}
		}
	}
	out := make([]StationSummary, 0, len(by))
	for k, a := range by {
		out = append(out, StationSummary{Key: k, Name: a.name, Address: a.addr, Swipes: a.swipes, Cars: len(a.cars), Cards: len(a.cards)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Swipes != out[j].Swipes {
			return out[i].Swipes > out[j].Swipes
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// UnknownMatchups is the dashboard work queue: start with singletons and
// station-neighbor suspects, not with inventing a home car.
func UnknownMatchups(txs []model.CardTx, pairings []model.CardPairing) []UnknownMatchup {
	type carScore struct {
		car   string
		n     int
		score float64
	}
	byCard := map[string][]carScore{}
	for _, p := range pairings {
		if p.EntityType != "car" {
			continue
		}
		byCard[p.CardID] = append(byCard[p.CardID], carScore{p.EntityKey, p.EvidenceN, p.Score})
	}
	for id := range byCard {
		sort.Slice(byCard[id], func(i, j int) bool { return byCard[id][i].score > byCard[id][j].score })
	}
	latest := map[string]model.CardTx{}
	for _, t := range txs {
		if t.CardID == "" {
			continue
		}
		if prev, ok := latest[t.CardID]; !ok || t.At.After(prev.At) {
			latest[t.CardID] = t
		}
	}
	var out []UnknownMatchup
	seen := map[string]struct{}{}
	add := func(u UnknownMatchup) {
		if u.CardID == "" {
			return
		}
		if _, ok := seen[u.CardID]; ok {
			return
		}
		seen[u.CardID] = struct{}{}
		u.Neighbors = stationNeighbors(txs, u.CardID, 6)
		out = append(out, u)
	}

	for _, s := range FindSuspects(txs, pairings) {
		u := UnknownMatchup{
			Kind:          "suspect",
			CardID:        s.CardID,
			EnterpriseCar: s.EnterpriseCar,
			BestCar:       s.BestCar,
			BestN:         s.EvidenceBest,
			LatestStation: s.LatestStation,
			LatestAt:      s.LatestAt,
			Why:           s.Reason,
		}
		if scores := byCard[s.CardID]; len(scores) > 0 {
			u.BestScore = scores[0].score
			if len(scores) > 1 {
				u.RunnerUpCar = scores[1].car
				u.RunnerUpN = scores[1].n
			}
		}
		add(u)
	}

	for card, scores := range byCard {
		if len(scores) == 0 {
			continue
		}
		last := latest[card]
		best := scores[0]
		u := UnknownMatchup{
			Kind:          "singleton",
			CardID:        card,
			EnterpriseCar: last.RecordedEFleetsID,
			BestCar:       best.car,
			BestN:         best.n,
			BestScore:     best.score,
			LatestStation: last.StationName,
			LatestAt:      last.At,
		}
		if len(scores) > 1 {
			u.RunnerUpCar = scores[1].car
			u.RunnerUpN = scores[1].n
		}
		if best.n <= 1 {
			u.Kind = "singleton"
			u.Why = fmt.Sprintf("one swipe on %s at %s — station map is the first clue", firstNonEmpty(best.car, last.RecordedEFleetsID), firstNonEmpty(last.StationName, "unknown station"))
			add(u)
			continue
		}
		if len(scores) > 1 && scores[1].n >= 2 && scores[1].score >= best.score*ambiguousRatio {
			u.Kind = "ambiguous"
			u.Why = fmt.Sprintf("swipe majority split %s n=%d vs %s n=%d", best.car, best.n, scores[1].car, scores[1].n)
			add(u)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		order := map[string]int{"suspect": 0, "ambiguous": 1, "singleton": 2}
		if order[out[i].Kind] != order[out[j].Kind] {
			return order[out[i].Kind] < order[out[j].Kind]
		}
		return out[i].CardID < out[j].CardID
	})
	return out
}

// CarsWithoutBestCard are roster or swipe cars that no card votes BEST.
func CarsWithoutBestCard(txs []model.CardTx, pairings []model.CardPairing, rosters ...map[string]string) []string {
	seen := map[string]struct{}{}
	for _, roster := range rosters {
		for id := range roster {
			if car := strings.TrimSpace(id); !isUnknownCar(car) {
				seen[car] = struct{}{}
			}
		}
	}
	for _, t := range txs {
		if car := strings.TrimSpace(t.RecordedEFleetsID); !isUnknownCar(car) {
			seen[car] = struct{}{}
		}
	}
	best := map[string]struct{}{}
	for _, p := range pairings {
		if p.EntityType == "car" && p.Best {
			best[p.EntityKey] = struct{}{}
		}
	}
	var out []string
	for car := range seen {
		if _, ok := best[car]; !ok {
			out = append(out, car)
		}
	}
	sort.Strings(out)
	return out
}

func stationNeighbors(txs []model.CardTx, cardID string, capN int) []Neighbor {
	hits := TraceStationDays(txs, cardID, DefaultWindowDays)
	out := make([]Neighbor, 0, capN)
	seenCar := map[string]struct{}{}
	for _, h := range hits {
		if h.OtherEFleetsID == "" {
			continue
		}
		if _, ok := seenCar[h.OtherEFleetsID+"|"+h.OtherCardID]; ok {
			continue
		}
		seenCar[h.OtherEFleetsID+"|"+h.OtherCardID] = struct{}{}
		out = append(out, Neighbor{
			EFleetsID: h.OtherEFleetsID,
			CardID:    h.OtherCardID,
			Station:   h.Station,
			DaysApart: h.DaysApart,
			At:        h.OtherAt,
		})
		if len(out) >= capN {
			break
		}
	}
	return out
}
