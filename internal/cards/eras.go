package cards

import (
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// Backpropagate extends forward GPS exclusive-pump labels backward through earlier
// swipes of the same card until evidence contradicts. Person and office cards
// never receive a car label. Split cards keep separate era windows per car.
func Backpropagate(assigned map[string]gpsHit, txs []model.CardTx, classified map[string]LadderCard, nick map[string]string) ([]RecordCall, []CardEra) {
	holder := cardHolders(txs, classified)
	anchors := anchorsFromAssigned(txs, assigned)
	return backpropCalls(txs, anchors, holder, nick), backpropEras(anchors, txs, assigned, holder, nick)
}

type forwardAnchor struct {
	at      time.Time
	card    string
	car     string
	station string
	rec     string
}

type cardHolder struct {
	bucket    string
	holderKey string
	nickname  string
}

func cardHolders(txs []model.CardTx, classified map[string]LadderCard) map[string]cardHolder {
	out := map[string]cardHolder{}
	for card, lc := range classified {
		if lc.Bucket != "" {
			out[card] = cardHolder{bucket: lc.Bucket, holderKey: lc.HolderKey, nickname: lc.Nickname}
		}
	}
	byCard := map[string][]model.CardTx{}
	for _, t := range txs {
		card := strings.TrimSpace(t.CardID)
		if card == "" {
			continue
		}
		byCard[card] = append(byCard[card], t)
	}
	for card, list := range byCard {
		if _, ok := out[card]; ok {
			continue
		}
		// Logistics and office are explicit; driver-kept person needs ladder classification.
		if h := holderFromTxs(list, len(classified) > 0); h.bucket != "" {
			out[card] = h
		}
	}
	return out
}

func holderFromTxs(list []model.CardTx, allowPerson bool) cardHolder {
	var person, office string
	logistics := false
	for _, t := range list {
		if oil.HasLogisticsPersonnel(t.DriverFirst, t.DriverLast) {
			logistics = true
		}
		if allowPerson {
			if p := personKey(t.DriverFirst, t.DriverLast); p != "" {
				if person == "" {
					person = p
				} else if person != p {
					person = person + "|" + p
				}
			}
		}
		if off := officeHolder(t); off != "" {
			office = off
		}
	}
	if logistics {
		key := "LOGISTICS_PERSONNEL"
		if person != "" && !strings.Contains(person, "|") {
			key = strings.Split(person, "|")[0]
		}
		return cardHolder{bucket: HolderPerson, holderKey: key, nickname: key}
	}
	if allowPerson && person != "" && !strings.Contains(person, "|") {
		return cardHolder{bucket: HolderPerson, holderKey: person, nickname: person}
	}
	if office != "" {
		return cardHolder{bucket: HolderOffice, holderKey: office, nickname: office}
	}
	return cardHolder{}
}

func anchorsFromAssigned(txs []model.CardTx, assigned map[string]gpsHit) []forwardAnchor {
	var out []forwardAnchor
	for _, t := range txs {
		h, ok := assigned[swipeKey(t)]
		if !ok || h.car == "" || isUnknownCar(h.car) {
			continue
		}
		out = append(out, forwardAnchor{
			at: t.At, card: h.card, car: h.car,
			station: h.station, rec: h.rec,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].card != out[j].card {
			return out[i].card < out[j].card
		}
		if out[i].at.Equal(out[j].at) {
			return out[i].car < out[j].car
		}
		return out[i].at.Before(out[j].at)
	})
	return out
}

// eraWindow is one car's backprop segment. [from, to) — to is exclusive.
type eraWindow struct {
	card, car string
	from, to  time.Time
}

func backpropWindows(card string, anchors []forwardAnchor) []eraWindow {
	if len(anchors) == 0 {
		return nil
	}
	type start struct {
		at  time.Time
		car string
	}
	var starts []start
	for _, a := range anchors {
		if len(starts) == 0 || starts[len(starts)-1].car != a.car {
			starts = append(starts, start{a.at, a.car})
		}
	}
	var wins []eraWindow
	for i, s := range starts {
		w := eraWindow{card: card, car: s.car, from: s.at}
		if i+1 < len(starts) {
			w.to = starts[i+1].at
		}
		wins = append(wins, w)
	}
	if len(wins) > 0 {
		wins[0].from = time.Time{}
	}
	return wins
}

func swipeInWindow(at time.Time, w eraWindow) bool {
	if !w.from.IsZero() && at.Before(w.from) {
		return false
	}
	if !w.to.IsZero() && !at.Before(w.to) {
		return false
	}
	return true
}

func stationsForAnchors(anchors []forwardAnchor, car string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, a := range anchors {
		if a.car != car {
			continue
		}
		st := strings.TrimSpace(a.station)
		if st == "" || skipStationName(st) {
			continue
		}
		if _, ok := seen[st]; ok {
			continue
		}
		seen[st] = struct{}{}
		out = append(out, st)
	}
	sort.Strings(out)
	return out
}

func backpropCalls(txs []model.CardTx, anchors []forwardAnchor, holder map[string]cardHolder, nick map[string]string) []RecordCall {
	byCard := map[string][]forwardAnchor{}
	for _, a := range anchors {
		byCard[a.card] = append(byCard[a.card], a)
	}
	direct := map[string]forwardAnchor{}
	for _, a := range anchors {
		sk := a.card + "|" + a.at.UTC().Format(time.RFC3339Nano)
		direct[sk] = a
	}

	var out []RecordCall
	for _, t := range txs {
		card := strings.TrimSpace(t.CardID)
		if card == "" || t.At.IsZero() {
			continue
		}
		if h, ok := holder[card]; ok && (h.bucket == HolderPerson || h.bucket == HolderOffice) {
			continue
		}
		station := firstNonEmpty(t.StationName, stationKey(t.StationName, t.StationAddress))
		if skipStationName(station) {
			station = ""
		}
		rec := strings.TrimSpace(t.RecordedEFleetsID)
		sk := card + "|" + t.At.UTC().Format(time.RFC3339Nano)
		if a, ok := direct[sk]; ok {
			if station == "" {
				station = a.station
			}
			out = append(out, RecordCall{
				CardID: card, At: t.At, Station: station,
				EnterpriseCar: rec, CalledCar: a.car,
				CalledName: firstNonEmpty(nick[a.car], a.car),
				Why:        "gps-stop",
			})
			continue
		}
		wins := backpropWindows(card, byCard[card])
		var cover []eraWindow
		for _, w := range wins {
			if swipeInWindow(t.At, w) {
				cover = append(cover, w)
			}
		}
		if len(cover) != 1 {
			continue
		}
		w := cover[0]
		if station == "" {
			for _, a := range byCard[card] {
				if a.car == w.car && a.station != "" {
					station = a.station
					break
				}
			}
		}
		out = append(out, RecordCall{
			CardID: card, At: t.At, Station: station,
			EnterpriseCar: rec, CalledCar: w.car,
			CalledName: firstNonEmpty(nick[w.car], w.car),
			Why:        "backprop",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CardID != out[j].CardID {
			return out[i].CardID < out[j].CardID
		}
		return out[i].At.Before(out[j].At)
	})
	return out
}

func backpropEras(anchors []forwardAnchor, txs []model.CardTx, assigned map[string]gpsHit, holder map[string]cardHolder, nick map[string]string) []CardEra {
	byCard := map[string][]forwardAnchor{}
	for _, a := range anchors {
		byCard[a.card] = append(byCard[a.card], a)
	}
	calls := backpropCalls(txs, anchors, holder, nick)

	var out []CardEra
	for card, h := range holder {
		if h.bucket != HolderPerson && h.bucket != HolderOffice {
			continue
		}
		var from, to time.Time
		n := 0
		for _, t := range txs {
			if strings.TrimSpace(t.CardID) != card {
				continue
			}
			n++
			if from.IsZero() || t.At.Before(from) {
				from = t.At
			}
			if to.IsZero() || t.At.After(to) {
				to = t.At
			}
		}
		out = append(out, CardEra{
			CardID: card, HolderType: h.bucket, HolderKey: h.holderKey,
			Nickname: h.nickname, From: from, To: to, EvidenceN: n,
		})
	}

	for card, list := range byCard {
		if h, ok := holder[card]; ok && (h.bucket == HolderPerson || h.bucket == HolderOffice) {
			continue
		}
		wins := backpropWindows(card, list)
		cars := map[string]struct{}{}
		for _, w := range wins {
			cars[w.car] = struct{}{}
		}
		for _, w := range wins {
			var from, to time.Time
			evidence := 0
			for _, c := range calls {
				if c.CardID != card || c.CalledCar != w.car {
					continue
				}
				if !swipeInWindow(c.At, w) {
					continue
				}
				evidence++
				if from.IsZero() || c.At.Before(from) {
					from = c.At
				}
				if to.IsZero() || c.At.After(to) {
					to = c.At
				}
			}
			if evidence == 0 {
				for _, t := range txs {
					if strings.TrimSpace(t.CardID) != card {
						continue
					}
					if h, ok := assigned[swipeKey(t)]; ok && h.car == w.car {
						evidence++
						if from.IsZero() || t.At.Before(from) {
							from = t.At
						}
						if to.IsZero() || t.At.After(to) {
							to = t.At
						}
					}
				}
			}
			if evidence == 0 {
				continue
			}
			out = append(out, CardEra{
				CardID: card, EFleetsID: w.car,
				Nickname:   firstNonEmpty(nick[w.car], w.car),
				HolderType: HolderCar, HolderKey: w.car,
				From: from, To: to, EvidenceN: evidence,
				Stations: stationsForAnchors(list, w.car),
				Split:    len(cars) > 1,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CardID != out[j].CardID {
			return out[i].CardID < out[j].CardID
		}
		if out[i].HolderType != out[j].HolderType {
			return out[i].HolderType < out[j].HolderType
		}
		return out[i].From.Before(out[j].From)
	})
	return out
}
