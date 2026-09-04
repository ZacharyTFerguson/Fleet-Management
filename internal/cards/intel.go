// Package cards scores fuel-card pairings from swipe history.
// Last Reading still lives only in internal/oil. This package never writes odo.
package cards

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// DefaultWindowDays is the station-day lookaround for a swapped-card trace.
// Two days covers overnight routes without treating a weekly shop habit as a hit.
const DefaultWindowDays = 2

// RecentBonusDays adds score weight for swipes in this trailing window so a
// recent true pairing can beat a long stale Enterprise assignment.
const RecentBonusDays = 30

// TxFromFill copies one DETAILS punch into the card intelligence store.
// CardID empty means the export had no card number; those punches cannot vote.
func TxFromFill(f model.Fill) (model.CardTx, bool) {
	if strings.TrimSpace(f.CardID) == "" {
		return model.CardTx{}, false
	}
	return model.CardTx{
		CardID:            strings.TrimSpace(f.CardID),
		At:                f.ProviderTransactionTime,
		StationName:       f.MerchantName,
		StationAddress:    f.MerchantAddress,
		Gallons:           f.Gallons,
		Amount:            f.Amount,
		RecordedEFleetsID: f.EFleetsID,
		RecordedCVN:       firstNonEmpty(f.ProviderCompanyVehicleNumber, f.CardCompanyVehicleNumber),
		Plate:             f.Plate,
		DriverFirst:       f.DriverFirst,
		DriverLast:        f.DriverLast,
		SourceRow:         f.EFleetsID + "|" + f.ProviderTransactionTime.UTC().Format(time.RFC3339),
		Odometer:          f.Odometer,
	}, true
}

// ScorePairings builds car and person scores for every card in txs.
//
// Heuristic (car): each swipe for recorded_efleets_id is 1.0 evidence; a swipe
// in the last RecentBonusDays adds +0.25 so a recent majority can override a
// long wrong Enterprise assignment. Best car = highest score.
//
// Heuristic (person): same scoring on "FIRST LAST" when both names are present.
// Logistics-personnel names are still stored on the tx (audit) but pairing
// callers should not treat them as a device join — that rule stays in oil.
func ScorePairings(txs []model.CardTx, now time.Time) []model.CardPairing {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	type key struct{ card, typ, ent string }
	n := map[key]int{}
	score := map[key]float64{}
	for _, t := range txs {
		card := strings.TrimSpace(t.CardID)
		if card == "" {
			continue
		}
		w := 1.0
		if now.Sub(t.At) >= 0 && now.Sub(t.At) <= RecentBonusDays*24*time.Hour {
			w += 0.25
		}
		person := personKey(t.DriverFirst, t.DriverLast)
		if person != "" {
			k := key{card, "person", person}
			n[k]++
			score[k] += w
		}
		// Logistics personnel keep the card; they never vote a device↔car join.
		if oil.HasLogisticsPersonnel(t.DriverFirst, t.DriverLast) {
			continue
		}
		// GPS-called car is the join when present. Enterprise Vehicle is fallback.
		car := strings.TrimSpace(t.CalledEFleetsID)
		if car != "" && !isUnknownCar(car) && !isOfficeLabel(car) {
			w *= 2
		} else {
			car = strings.TrimSpace(t.RecordedEFleetsID)
		}
		if isOfficeLabel(car) || (isUnknownCar(car) && isOfficeLabel(t.RecordedCVN)) {
			off := strings.TrimSpace(car)
			if off == "" || isUnknownCar(off) {
				off = strings.TrimSpace(t.RecordedCVN)
			}
			if off != "" {
				k := key{card, "office", off}
				n[k]++
				score[k] += w
			}
			continue
		}
		if car != "" && !isUnknownCar(car) {
			k := key{card, "car", car}
			n[k]++
			score[k] += w
		}
	}
	bestScore := map[string]float64{}
	bestKey := map[string]key{}
	var out []model.CardPairing
	for k, sc := range score {
		out = append(out, model.CardPairing{
			CardID:     k.card,
			EntityType: k.typ,
			EntityKey:  k.ent,
			EvidenceN:  n[k],
			Score:      sc,
		})
		bk := k.card + "|" + k.typ
		if sc > bestScore[bk] {
			bestScore[bk] = sc
			bestKey[bk] = k
		}
	}
	for i := range out {
		bk := out[i].CardID + "|" + out[i].EntityType
		if best, ok := bestKey[bk]; ok && best.ent == out[i].EntityKey && best.typ == out[i].EntityType {
			out[i].Best = true
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CardID != out[j].CardID {
			return out[i].CardID < out[j].CardID
		}
		if out[i].EntityType != out[j].EntityType {
			return out[i].EntityType < out[j].EntityType
		}
		return out[i].Score > out[j].Score
	})
	return out
}

// Suspect is a card whose latest Enterprise car is not the best-scoring car.
type Suspect struct {
	CardID         string
	EnterpriseCar  string
	BestCar        string
	LatestAt       time.Time
	LatestStation  string
	Reason         string
	EvidenceBest   int
	EvidenceLatest int
}

// FindSuspects flags cards where the swipe majority (or recent majority) points
// at a different car than the latest DETAILS Vehicle column.
func FindSuspects(txs []model.CardTx, pairings []model.CardPairing) []Suspect {
	bestCar := map[string]model.CardPairing{}
	for _, p := range pairings {
		if p.EntityType == "car" && p.Best {
			bestCar[p.CardID] = p
		}
	}
	latest := map[string]model.CardTx{}
	count := map[string]map[string]int{}
	for _, t := range txs {
		if t.CardID == "" || strings.TrimSpace(t.RecordedEFleetsID) == "" {
			continue
		}
		if prev, ok := latest[t.CardID]; !ok || t.At.After(prev.At) {
			latest[t.CardID] = t
		}
		if count[t.CardID] == nil {
			count[t.CardID] = map[string]int{}
		}
		count[t.CardID][t.RecordedEFleetsID]++
	}
	var out []Suspect
	for card, last := range latest {
		b, ok := bestCar[card]
		if !ok {
			continue
		}
		if b.EntityKey == last.RecordedEFleetsID {
			continue
		}
		out = append(out, Suspect{
			CardID:         card,
			EnterpriseCar:  last.RecordedEFleetsID,
			BestCar:        b.EntityKey,
			LatestAt:       last.At,
			LatestStation:  last.StationName,
			Reason:         fmt.Sprintf("latest DETAILS car %s; swipe majority %s (n=%d score=%.2f)", last.RecordedEFleetsID, b.EntityKey, b.EvidenceN, b.Score),
			EvidenceBest:   b.EvidenceN,
			EvidenceLatest: count[card][last.RecordedEFleetsID],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CardID < out[j].CardID })
	return out
}

// TraceHit is another car (or card) at the same station on a nearby day.
type TraceHit struct {
	CardID         string
	Station        string
	Day            time.Time
	OtherEFleetsID string
	OtherCardID    string
	OtherAt        time.Time
	DaysApart      int
}

// TraceStationDays finds cars that fueled at the same station around days
// when cardID was used — especially useful when that swipe was recorded on
// a weird car.
//
// Heuristic: normalize station as lower(name)|lower(address). For each swipe
// of cardID at station S on day D, collect other cars' swipes at S on
// [D-window, D+window]. Those cars are "also at this pump then" — the usual
// tell that the driver of car B used card C while Enterprise wrote car A.
// Vice versa is the same join: given a suspect card↔car pair, the other
// car at S on those days is the candidate true pairing.
func TraceStationDays(txs []model.CardTx, cardID string, windowDays int) []TraceHit {
	if windowDays <= 0 {
		windowDays = DefaultWindowDays
	}
	cardID = strings.TrimSpace(cardID)
	var mine []model.CardTx
	byStation := map[string][]model.CardTx{}
	for _, t := range txs {
		st := stationKey(t.StationName, t.StationAddress)
		if st == "" {
			continue
		}
		byStation[st] = append(byStation[st], t)
		if t.CardID == cardID {
			mine = append(mine, t)
		}
	}
	seen := map[string]struct{}{}
	var out []TraceHit
	for _, m := range mine {
		st := stationKey(m.StationName, m.StationAddress)
		day := truncateDay(m.At)
		for _, o := range byStation[st] {
			if o.CardID == cardID && o.RecordedEFleetsID == m.RecordedEFleetsID && o.At.Equal(m.At) {
				continue
			}
			if o.RecordedEFleetsID == "" {
				continue
			}
			od := truncateDay(o.At)
			apart := daysBetween(day, od)
			if apart > windowDays {
				continue
			}
			k := o.RecordedEFleetsID + "|" + o.CardID + "|" + o.At.UTC().Format(time.RFC3339)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, TraceHit{
				CardID:         cardID,
				Station:        firstNonEmpty(m.StationName, st),
				Day:            day,
				OtherEFleetsID: o.RecordedEFleetsID,
				OtherCardID:    o.CardID,
				OtherAt:        o.At,
				DaysApart:      apart,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DaysApart != out[j].DaysApart {
			return out[i].DaysApart < out[j].DaysApart
		}
		return out[i].OtherAt.Before(out[j].OtherAt)
	})
	return out
}

func personKey(first, last string) string {
	f := strings.ToUpper(strings.TrimSpace(first))
	l := strings.ToUpper(strings.TrimSpace(last))
	if f == "" || l == "" {
		return ""
	}
	// DETAILS placeholder, not a driver who keeps a card.
	if f == "FLEET" && l == "DRIVER" {
		return ""
	}
	return f + " " + l
}

func isUnknownCar(id string) bool {
	s := strings.ToLower(strings.TrimSpace(id))
	return s == "" || s == "unknown" || s == "n/a" || s == "-" || s == "tracker"
}

func skipStationName(name string) bool {
	s := strings.ToLower(strings.TrimSpace(name))
	return s == "" || s == "tracker" || s == "unknown" || s == "n/a"
}

func stationKey(name, addr string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if skipStationName(n) {
		return ""
	}
	city, state := cityState(addr)
	if city == "" && state == "" {
		return n
	}
	return n + "|" + city + "|" + state
}

func cityState(addr string) (string, string) {
	parts := strings.Split(addr, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) >= 2 {
		return out[len(out)-2], out[len(out)-1]
	}
	return "", ""
}

func truncateDay(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func daysBetween(a, b time.Time) int {
	if a.After(b) {
		a, b = b, a
	}
	return int(b.Sub(a).Hours() / 24)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
