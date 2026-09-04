package cards

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
)

// StationRadiusMeters is how close a GPS stop must be to a mapped pump.
// A typical lot is well inside this; a second car across town is not.
const StationRadiusMeters = 350.0

// GPSFirstResult is GPS-at-the-pump matching. It never writes Last Reading.
type GPSFirstResult struct {
	Matches   []model.GPSCardMatch
	Eras      []CardEra
	Calls     []RecordCall
	Stations  []GeocodedStation
	Pumps     int
	assigned  map[string]gpsHit // forward exclusive sits; same package only
	hasGPSPos bool              // visits had coordinates; TRACKER is not the coverage blocker
}

// CardEra is the GPS / station-ladder history of where a card sat.
type CardEra = model.CardEra

// RecordCall is what to call one swipe: GPS car name, not the DETAILS Vehicle column.
type RecordCall struct {
	CardID        string    `json:"card_id"`
	At            time.Time `json:"at"`
	Station       string    `json:"station,omitempty"`
	EnterpriseCar string    `json:"enterprise_car,omitempty"`
	CalledCar     string    `json:"called_car"`
	CalledName    string    `json:"called_name,omitempty"`
	Why           string    `json:"why"`
}

// GeocodedStation is a pump whose lat/lng came from GPS sits during a swipe.
type GeocodedStation struct {
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Hits    int     `json:"hits"`
}

type gpsHit struct {
	card, car, station, rec string
	at                      time.Time
	lat, lng                float64
	hasPos                  bool
}

type geoAcc struct {
	name, addr string
	lat, lng   float64
	n          int
}

func (g *geoAcc) add(lat, lng float64) {
	g.n++
	g.lat += (lat - g.lat) / float64(g.n)
	g.lng += (lng - g.lng) / float64(g.n)
}

// FillsWithFleetSight drops swipes whose FillDayWindow only has GPS from a
// handful of linked boxes. A watch-loop fetch of one car into August must not
// let GPS-first treat that car as exclusive at every pump.
func FillsWithFleetSight(txs []model.CardTx, visits []model.StopVisit, linked []model.OneStepDevice) []model.CardTx {
	nLinked := 0
	isLinked := map[string]struct{}{}
	for _, d := range linked {
		id := strings.TrimSpace(d.FactoryID)
		if id == "" || d.Dead || !d.Active {
			continue
		}
		if d.LinkedCarEFleetsID == nil || strings.TrimSpace(*d.LinkedCarEFleetsID) == "" {
			continue
		}
		if _, ok := isLinked[id]; ok {
			continue
		}
		isLinked[id] = struct{}{}
		nLinked++
	}
	need := 1
	if nLinked >= 8 {
		need = 8
	}
	if nLinked == 0 || need <= 1 {
		return txs
	}
	var out []model.CardTx
	for _, t := range txs {
		if t.At.IsZero() {
			out = append(out, t)
			continue
		}
		from, to := FillDayWindow(t.At)
		if from.IsZero() {
			out = append(out, t)
			continue
		}
		seen := map[string]struct{}{}
		for _, v := range visits {
			fid := strings.TrimSpace(v.FactoryID)
			if _, ok := isLinked[fid]; !ok {
				continue
			}
			start, end := v.From, v.To
			if start.IsZero() && end.IsZero() {
				continue
			}
			if end.IsZero() {
				end = start
			}
			if start.IsZero() {
				start = end
			}
			if end.Before(from) || start.After(to) {
				continue
			}
			seen[fid] = struct{}{}
			if len(seen) >= need {
				break
			}
		}
		if len(seen) >= need {
			out = append(out, t)
		}
	}
	return out
}

// MatchByStopTimes is the time-only exclusive matcher. GPS-first matching
// (region + station coordinates) lives in MatchGPSFirst.
func MatchByStopTimes(visits []model.StopVisit, txs []model.CardTx, slack time.Duration) []model.GPSCardMatch {
	return MatchGPSFirst(visits, txs, nil, slack).Matches
}

// MatchGPSFirst starts from GPS sits at pumps, then names the card that swiped
// there. A swipe votes only when exactly one GPS-linked car is a candidate:
//
//  1. Short stop (≤ MaxPumpStop) covering the swipe (± slack).
//  2. Prefer cars sitting at a GPS pump cluster (shared short-stop sites).
//  3. If the station is geocoded, the sit must be within StationRadiusMeters.
//  4. Else if the merchant state is known, prefer cars whose unit region is that state.
//  5. Two candidates still left → skip (do not guess).
//
// TRACKER / empty merchant *names* are not pumps, but an exclusive GPS sit at
// swipe time still names the card (station label is the GPS cluster, not TRACKER).
//
// A first exclusive hit geocodes that station from the box lat/lng, then a
// second pass retries swipes that were ambiguous on time alone. Last Reading
// does not use this.
func MatchGPSFirst(visits []model.StopVisit, txs []model.CardTx, fleet []model.Car, slack time.Duration) GPSFirstResult {
	if slack <= 0 {
		slack = DefaultStopSlack
	}
	nick := map[string]string{}
	region := map[string]string{}
	for _, c := range fleet {
		id := strings.TrimSpace(c.EFleetsID)
		if id == "" {
			continue
		}
		nick[id] = strings.TrimSpace(c.Nickname)
		r := strings.TrimSpace(c.Region)
		if r == "" {
			r = regionFromNickname(c.Nickname)
		}
		region[id] = strings.ToUpper(r)
	}

	buckets := bucketPumpVisits(visits, slack)
	pumps := clusterPumps(visits)
	geo := map[string]*geoAcc{}
	assigned := map[string]gpsHit{} // swipe key → hit
	hasGPSPos := false
	for _, v := range visits {
		if v.HasPos && strings.TrimSpace(v.EFleetsID) != "" && !isUnknownCar(v.EFleetsID) {
			hasGPSPos = true
			break
		}
	}

	assignPass := func(useGeo bool) {
		for _, t := range txs {
			card := strings.TrimSpace(t.CardID)
			if card == "" || t.At.IsZero() {
				continue
			}
			placeholder := skipStationName(t.StationName)
			sk := swipeKey(t)
			if _, ok := assigned[sk]; ok {
				continue
			}
			stKey := ""
			stState := ""
			if !placeholder {
				stKey = stationKey(t.StationName, t.StationAddress)
				_, stState = cityState(t.StationAddress)
				stState = strings.ToUpper(strings.TrimSpace(stState))
				if len(stState) > 2 {
					stState = stState[:2]
				}
			}
			cands := uniqueCarsAt(buckets, t.At, slack)
			if placeholder {
				// Fake merchant address must not geocode. Require a fuel-length
				// sit on a pump cluster, and do not paint one car onto a pile of
				// simultaneous TRACKER punches (synthetic 10:00 AM).
				if placeholderCardsAt(txs, t.At, slack) != 1 {
					continue
				}
				var fuel []model.StopVisit
				for _, v := range cands {
					if isFuelSit(v) && sitAtPump(v, pumps) {
						fuel = append(fuel, v)
					}
				}
				cands = fuel
			} else {
				cands = filterCandidates(cands, stKey, stState, region, geo, pumps, useGeo)
			}
			if len(cands) != 1 {
				continue
			}
			v := cands[0]
			station := firstNonEmpty(t.StationName, stKey)
			if placeholder {
				station = gpsPumpStation(v)
			}
			hit := gpsHit{
				card:    card,
				car:     strings.TrimSpace(v.EFleetsID),
				station: station,
				rec:     strings.TrimSpace(t.RecordedEFleetsID),
				at:      t.At,
				lat:     v.Lat,
				lng:     v.Lng,
				hasPos:  v.HasPos,
			}
			if hit.car == "" || isUnknownCar(hit.car) {
				continue
			}
			assigned[sk] = hit
			if stKey != "" && v.HasPos {
				g := geo[stKey]
				if g == nil {
					g = &geoAcc{name: strings.TrimSpace(t.StationName), addr: strings.TrimSpace(t.StationAddress)}
					geo[stKey] = g
				}
				g.add(v.Lat, v.Lng)
			}
		}
	}
	assignPass(false)
	if len(geo) > 0 {
		assignPass(true)
	}

	hits := make([]gpsHit, 0, len(assigned))
	for _, h := range assigned {
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].card != hits[j].card {
			return hits[i].card < hits[j].card
		}
		return hits[i].at.Before(hits[j].at)
	})

	matches := collapseMatches(hits)
	calls, eras := Backpropagate(assigned, txs, nil, nick)
	stations := flattenGeo(geo)
	return GPSFirstResult{
		Matches: matches, Eras: eras, Calls: calls, Stations: stations,
		Pumps: len(pumps), assigned: assigned, hasGPSPos: hasGPSPos,
	}
}

// ApplyCalls copies GPS-called cars onto swipes. RecordedEFleetsID is unchanged.
func ApplyCalls(txs []model.CardTx, calls []RecordCall) []model.CardTx {
	by := map[string]string{}
	for _, c := range calls {
		if c.CalledCar == "" {
			continue
		}
		by[c.CardID+"|"+c.At.UTC().Format(time.RFC3339Nano)] = c.CalledCar
		by[c.CardID+"|"+c.At.UTC().Format(time.RFC3339)] = c.CalledCar
	}
	out := append([]model.CardTx(nil), txs...)
	for i := range out {
		if car, ok := by[out[i].CardID+"|"+out[i].At.UTC().Format(time.RFC3339Nano)]; ok {
			out[i].CalledEFleetsID = car
			continue
		}
		if car, ok := by[out[i].CardID+"|"+out[i].At.UTC().Format(time.RFC3339)]; ok {
			out[i].CalledEFleetsID = car
		}
	}
	return out
}

func swipeKey(t model.CardTx) string {
	return strings.TrimSpace(t.CardID) + "|" + t.At.UTC().Format(time.RFC3339Nano) + "|" + strings.TrimSpace(t.RecordedEFleetsID)
}

func bucketPumpVisits(visits []model.StopVisit, slack time.Duration) map[int64][]model.StopVisit {
	out := map[int64][]model.StopVisit{}
	for _, v := range visits {
		if strings.TrimSpace(v.EFleetsID) == "" || isUnknownCar(v.EFleetsID) {
			continue
		}
		if v.From.IsZero() || v.To.IsZero() {
			continue
		}
		from, to := v.From.UTC(), v.To.UTC()
		if to.Before(from) {
			from, to = to, from
		}
		sit := to.Sub(from)
		if sit < 0 || sit > MaxPumpStop {
			continue
		}
		start := from.Add(-slack)
		end := to.Add(slack)
		for h := start.Unix() / 3600; h <= end.Unix()/3600; h++ {
			out[h] = append(out[h], v)
		}
	}
	return out
}

func uniqueCarsAt(buckets map[int64][]model.StopVisit, at time.Time, slack time.Duration) []model.StopVisit {
	at = at.UTC()
	h := at.Unix() / 3600
	seen := map[string]model.StopVisit{}
	order := []string{}
	for _, hr := range []int64{h - 1, h, h + 1} {
		for _, v := range buckets[hr] {
			if !stopCovers(v, at, slack) {
				continue
			}
			car := strings.TrimSpace(v.EFleetsID)
			if _, ok := seen[car]; ok {
				// Prefer a sit that has coordinates when the same car appears twice.
				if !seen[car].HasPos && v.HasPos {
					seen[car] = v
				}
				continue
			}
			seen[car] = v
			order = append(order, car)
		}
	}
	out := make([]model.StopVisit, 0, len(order))
	for _, car := range order {
		out = append(out, seen[car])
	}
	return out
}

func filterCandidates(cands []model.StopVisit, stKey, stState string, region map[string]string, geo map[string]*geoAcc, pumps []pumpCluster, useGeo bool) []model.StopVisit {
	if len(cands) <= 1 {
		return cands
	}
	if useGeo && stKey != "" {
		if g, ok := geo[stKey]; ok && g.n > 0 {
			var near []model.StopVisit
			for _, v := range cands {
				if !v.HasPos {
					continue
				}
				if metersBetween(v.Lat, v.Lng, g.lat, g.lng) <= StationRadiusMeters {
					near = append(near, v)
				}
			}
			if len(near) > 0 {
				cands = near
				if len(cands) <= 1 {
					return cands
				}
			}
		}
	}
	if len(pumps) > 0 {
		var at []model.StopVisit
		for _, v := range cands {
			if sitAtPump(v, pumps) {
				at = append(at, v)
			}
		}
		if len(at) > 0 {
			cands = at
			if len(cands) <= 1 {
				return cands
			}
		}
	}
	if stState != "" && len(region) > 0 {
		var same []model.StopVisit
		for _, v := range cands {
			if regionMatchesState(region[strings.TrimSpace(v.EFleetsID)], stState) {
				same = append(same, v)
			}
		}
		if len(same) > 0 {
			return same
		}
	}
	return cands
}

// pumpCluster is a GPS place many cars sit for a short time — usually a pump.
type pumpCluster struct {
	lat, lng float64
}

const pumpGridDeg = 0.002 // ~220m

func clusterPumps(visits []model.StopVisit) []pumpCluster {
	type acc struct {
		lat, lng float64
		n        int
		cars     map[string]struct{}
	}
	by := map[[2]int]*acc{}
	for _, v := range visits {
		if !v.HasPos || strings.TrimSpace(v.EFleetsID) == "" || isUnknownCar(v.EFleetsID) {
			continue
		}
		if v.From.IsZero() || v.To.IsZero() {
			continue
		}
		sit := v.To.Sub(v.From)
		if sit < 2*time.Minute || sit > MaxPumpStop {
			continue
		}
		key := [2]int{int(math.Round(v.Lat / pumpGridDeg)), int(math.Round(v.Lng / pumpGridDeg))}
		a := by[key]
		if a == nil {
			a = &acc{cars: map[string]struct{}{}}
			by[key] = a
		}
		a.n++
		a.lat += (v.Lat - a.lat) / float64(a.n)
		a.lng += (v.Lng - a.lng) / float64(a.n)
		a.cars[strings.TrimSpace(v.EFleetsID)] = struct{}{}
	}
	var out []pumpCluster
	for _, a := range by {
		if len(a.cars) >= 3 || a.n >= 8 {
			out = append(out, pumpCluster{lat: a.lat, lng: a.lng})
		}
	}
	return out
}

// gpsPumpStation is the ladder station key when DETAILS merchant is TRACKER/empty.
func gpsPumpStation(v model.StopVisit) string {
	if !v.HasPos {
		return "gps-stop"
	}
	return fmt.Sprintf("gps:%.4f,%.4f", v.Lat, v.Lng)
}

func sitAtPump(v model.StopVisit, pumps []pumpCluster) bool {
	if !v.HasPos || len(pumps) == 0 {
		return false
	}
	for _, p := range pumps {
		if metersBetween(v.Lat, v.Lng, p.lat, p.lng) <= StationRadiusMeters {
			return true
		}
	}
	return false
}

func isFuelSit(v model.StopVisit) bool {
	if v.From.IsZero() || v.To.IsZero() {
		return false
	}
	sit := v.To.Sub(v.From)
	if sit < 0 {
		sit = -sit
	}
	return sit >= MinFuelSit && sit <= MaxTrackerFuelSit
}

func placeholderCardsAt(txs []model.CardTx, at time.Time, slack time.Duration) int {
	if slack < 0 {
		slack = -slack
	}
	at = at.UTC()
	seen := map[string]struct{}{}
	for _, t := range txs {
		if !skipStationName(t.StationName) {
			continue
		}
		card := strings.TrimSpace(t.CardID)
		if card == "" || t.At.IsZero() {
			continue
		}
		d := t.At.UTC().Sub(at)
		if d < 0 {
			d = -d
		}
		if d <= slack {
			seen[card] = struct{}{}
		}
	}
	return len(seen)
}

func collapseMatches(hits []gpsHit) []model.GPSCardMatch {
	type key struct{ card, car string }
	n := map[key]int{}
	stations := map[key]map[string]struct{}{}
	enterprise := map[key]map[string]struct{}{}
	for _, h := range hits {
		k := key{h.card, h.car}
		n[k]++
		if stations[k] == nil {
			stations[k] = map[string]struct{}{}
		}
		if name := strings.TrimSpace(h.station); name != "" && !skipStationName(name) {
			stations[k][name] = struct{}{}
		}
		if enterprise[k] == nil {
			enterprise[k] = map[string]struct{}{}
		}
		if rec := strings.TrimSpace(h.rec); rec != "" && !isUnknownCar(rec) {
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

func splitEras(hits []gpsHit, nick map[string]string) []CardEra {
	byCard := map[string][]gpsHit{}
	for _, h := range hits {
		byCard[h.card] = append(byCard[h.card], h)
	}
	carsOf := map[string]map[string]struct{}{}
	var out []CardEra
	for card, list := range byCard {
		sort.Slice(list, func(i, j int) bool { return list[i].at.Before(list[j].at) })
		carsOf[card] = map[string]struct{}{}
		var cur *CardEra
		stSeen := map[string]struct{}{}
		flush := func() {
			if cur == nil {
				return
			}
			for s := range stSeen {
				cur.Stations = append(cur.Stations, s)
			}
			sort.Strings(cur.Stations)
			out = append(out, *cur)
			cur = nil
			stSeen = map[string]struct{}{}
		}
		for _, h := range list {
			carsOf[card][h.car] = struct{}{}
			if cur == nil || cur.EFleetsID != h.car {
				flush()
				era := CardEra{
					CardID:     card,
					EFleetsID:  h.car,
					Nickname:   nick[h.car],
					HolderType: HolderCar,
					HolderKey:  h.car,
					From:       h.at,
					To:         h.at,
					EvidenceN:  1,
				}
				cur = &era
			} else {
				cur.To = h.at
				cur.EvidenceN++
			}
			if name := strings.TrimSpace(h.station); name != "" {
				stSeen[name] = struct{}{}
			}
		}
		flush()
	}
	for i := range out {
		out[i].Split = len(carsOf[out[i].CardID]) > 1
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CardID != out[j].CardID {
			return out[i].CardID < out[j].CardID
		}
		return out[i].From.Before(out[j].From)
	})
	return out
}

func callRecords(txs []model.CardTx, assigned map[string]gpsHit, eras []CardEra, nick map[string]string) []RecordCall {
	erasByCard := map[string][]CardEra{}
	for _, e := range eras {
		erasByCard[e.CardID] = append(erasByCard[e.CardID], e)
	}
	var out []RecordCall
	for _, t := range txs {
		card := strings.TrimSpace(t.CardID)
		if card == "" || t.At.IsZero() {
			continue
		}
		station := firstNonEmpty(t.StationName, stationKey(t.StationName, t.StationAddress))
		rec := strings.TrimSpace(t.RecordedEFleetsID)
		if h, ok := assigned[swipeKey(t)]; ok {
			out = append(out, RecordCall{
				CardID:        card,
				At:            t.At,
				Station:       station,
				EnterpriseCar: rec,
				CalledCar:     h.car,
				CalledName:    firstNonEmpty(nick[h.car], h.car),
				Why:           "gps-stop",
			})
			continue
		}
		var cover []CardEra
		for _, e := range erasByCard[card] {
			if !t.At.Before(e.From) && !t.At.After(e.To) {
				cover = append(cover, e)
			}
		}
		if len(cover) != 1 {
			continue
		}
		e := cover[0]
		out = append(out, RecordCall{
			CardID:        card,
			At:            t.At,
			Station:       station,
			EnterpriseCar: rec,
			CalledCar:     e.EFleetsID,
			CalledName:    firstNonEmpty(e.Nickname, nick[e.EFleetsID], e.EFleetsID),
			Why:           "era",
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

func flattenGeo(geo map[string]*geoAcc) []GeocodedStation {
	out := make([]GeocodedStation, 0, len(geo))
	for k, g := range geo {
		if g == nil || g.n == 0 {
			continue
		}
		out = append(out, GeocodedStation{
			Key: k, Name: g.name, Address: g.addr,
			Lat: g.lat, Lng: g.lng, Hits: g.n,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Hits != out[j].Hits {
			return out[i].Hits > out[j].Hits
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// MetersBetween is haversine meters. Nearby hunt uses 1 mile; GPS-first pumps use StationRadiusMeters.
func MetersBetween(lat1, lng1, lat2, lng2 float64) float64 {
	return metersBetween(lat1, lng1, lat2, lng2)
}

func metersBetween(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371000.0
	p1 := lat1 * math.Pi / 180
	p2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func regionMatchesState(region, state string) bool {
	region = strings.ToUpper(strings.TrimSpace(region))
	state = strings.ToUpper(strings.TrimSpace(state))
	if region == "" || state == "" {
		return true
	}
	for _, s := range statesForRegion(region) {
		if s == state {
			return true
		}
	}
	return false
}

func statesForRegion(region string) []string {
	switch strings.ToUpper(strings.TrimSpace(region)) {
	case "VA":
		return []string{"VA"}
	case "CT":
		return []string{"CT"}
	case "DE":
		return []string{"DE"}
	case "MD":
		return []string{"MD"}
	case "PA":
		return []string{"PA"}
	case "NJ":
		return []string{"NJ"}
	case "OH":
		return []string{"OH"}
	case "FL":
		return []string{"FL"}
	case "NC":
		return []string{"NC"}
	case "SC":
		return []string{"SC"}
	case "GA":
		return []string{"GA"}
	case "WV":
		return []string{"WV"}
	case "NY", "WNY", "BING", "BK", "BRONX", "WESTCHESTER", "LI", "LONGISLAND":
		return []string{"NY"}
	default:
		return nil
	}
}

func regionFromNickname(nick string) string {
	var b strings.Builder
	for _, r := range nick {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			b.WriteRune(r)
			continue
		}
		break
	}
	return strings.ToUpper(b.String())
}
