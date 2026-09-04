package cards

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"oilchange/internal/enterprise"
	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// NearbyRadiusMiles is the Generate Reports Nearby Address radius.
const NearbyRadiusMiles = 1.0

// NearbyRadiusMeters is 1 mile in meters (haversine).
const NearbyRadiusMeters = NearbyRadiusMiles * 1609.344

// Nearby certainty rungs: watch (any hit) → likely (2 exclusive fill-time) → certain (3).
const (
	NearbyLikelyFills  = 2
	NearbyCertainFills = 3
)

const (
	NearbyWatch   = "watch"
	NearbyLikely  = "likely"
	NearbyCertain = "certain"
)

// NearbyDevice is one factory_id seen near a card's pumps. display_name is not stored as identity.
type NearbyDevice struct {
	FactoryID      string   `json:"factory_id"`
	DeviceID       string   `json:"device_id,omitempty"`
	VIN            string   `json:"vin,omitempty"`
	LinkedCar      string   `json:"linked_car,omitempty"`
	Fills          int      `json:"fills"`
	AtFillFills    int      `json:"at_fill_fills"`
	ExclusiveFills int      `json:"exclusive_fills"`
	MinMiles       float64  `json:"min_miles,omitempty"`
	Stations       []string `json:"stations,omitempty"`
	Rank           string   `json:"rank"`
}

// NearbyCard is the hunt result for one unknown card.
type NearbyCard struct {
	CardID  string         `json:"card_id"`
	Fills   int            `json:"fills"`
	Watch   []NearbyDevice `json:"watch,omitempty"`
	Likely  []NearbyDevice `json:"likely,omitempty"`
	Certain []NearbyDevice `json:"certain,omitempty"`
	Why     string         `json:"why,omitempty"`
}

// NearbyResult is the operator watch list. It never writes Last Reading.
type NearbyResult struct {
	Cards   []NearbyCard `json:"cards"`
	Certain int          `json:"certain_n"`
	Likely  int          `json:"likely_n"`
	Watch   int          `json:"watch_n"`
}

type nearbyStation struct {
	key, name, addr string
	lat, lng        float64
	hasPos          bool
}

// FillDayWindow is Eastern calendar day-before 00:00 through day-after 23:59:59.999.
// fillAt must be the provider swipe second, not bank posting.
func FillDayWindow(fillAt time.Time) (from, to time.Time) {
	if fillAt.IsZero() {
		return time.Time{}, time.Time{}
	}
	ny := enterprise.NY()
	local := fillAt.In(ny)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, ny)
	from = day.AddDate(0, 0, -1)
	to = day.AddDate(0, 0, 2).Add(-time.Millisecond)
	return from.UTC(), to.UTC()
}

// HuntNearby lists factory_id candidates within 1 mile of each unknown card fill
// during fill-day ±1. One hit is a watch, not a join.
func HuntNearby(visits []model.StopVisit, txs []model.CardTx, stations []GeocodedStation, devices []model.OneStepDevice, slack time.Duration) NearbyResult {
	if slack <= 0 {
		slack = DefaultStopSlack
	}
	byStation := indexGeocoded(stations)
	devByFactory := map[string]model.OneStepDevice{}
	for _, d := range devices {
		if id := strings.TrimSpace(d.FactoryID); id != "" {
			devByFactory[id] = d
		}
	}
	unknown := unknownCardIDs(txs)
	type acc struct {
		fills    map[string]struct{}
		atFill   map[string]struct{}
		deviceID string
		vin      string
		minMiles float64
		hasMiles bool
		stations map[string]struct{}
	}
	type cardAcc struct {
		fills int
		dev   map[string]*acc
	}
	cardsAcc := map[string]*cardAcc{}

	for _, tx := range txs {
		card := strings.TrimSpace(tx.CardID)
		if card == "" {
			continue
		}
		if _, ok := unknown[card]; !ok {
			continue
		}
		if oil.HasLogisticsPersonnel(tx.DriverFirst, tx.DriverLast) {
			continue
		}
		st := resolveStation(tx, byStation)
		if !st.hasPos && strings.TrimSpace(st.addr) == "" {
			continue
		}
		from, to := FillDayWindow(tx.At)
		if from.IsZero() {
			continue
		}
		ca := cardsAcc[card]
		if ca == nil {
			ca = &cardAcc{dev: map[string]*acc{}}
			cardsAcc[card] = ca
		}
		ca.fills++
		fillKey := tx.At.UTC().Format(time.RFC3339Nano) + "|" + st.key
		for _, v := range visits {
			if !v.HasPos || strings.TrimSpace(v.FactoryID) == "" {
				continue
			}
			if v.To.Before(from) || v.From.After(to) {
				continue
			}
			if !st.hasPos {
				continue
			}
			meters := MetersBetween(v.Lat, v.Lng, st.lat, st.lng)
			if meters > NearbyRadiusMeters {
				continue
			}
			fid := strings.TrimSpace(v.FactoryID)
			a := ca.dev[fid]
			if a == nil {
				a = &acc{
					fills:    map[string]struct{}{},
					atFill:   map[string]struct{}{},
					deviceID: strings.TrimSpace(v.DeviceID),
					stations: map[string]struct{}{},
				}
				ca.dev[fid] = a
			}
			if a.deviceID == "" {
				a.deviceID = strings.TrimSpace(v.DeviceID)
			}
			miles := meters / 1609.344
			if !a.hasMiles || miles < a.minMiles {
				a.minMiles, a.hasMiles = miles, true
			}
			a.fills[fillKey] = struct{}{}
			if st.key != "" {
				a.stations[st.key] = struct{}{}
			}
			if stopOverlapsFill(v, tx.At, slack) {
				a.atFill[fillKey] = struct{}{}
			}
		}
	}

	var out NearbyResult
	for card, ca := range cardsAcc {
		nc := NearbyCard{CardID: card, Fills: ca.fills}
		fillFids := map[string]map[string]struct{}{}
		for fid, a := range ca.dev {
			for fk := range a.atFill {
				set := fillFids[fk]
				if set == nil {
					set = map[string]struct{}{}
					fillFids[fk] = set
				}
				set[fid] = struct{}{}
			}
		}
		exclusiveN := map[string]int{}
		for _, fids := range fillFids {
			if len(fids) != 1 {
				continue
			}
			for fid := range fids {
				exclusiveN[fid]++
			}
		}

		var devicesOut []NearbyDevice
		for fid, a := range ca.dev {
			d := NearbyDevice{
				FactoryID:      fid,
				DeviceID:       a.deviceID,
				Fills:          len(a.fills),
				AtFillFills:    len(a.atFill),
				ExclusiveFills: exclusiveN[fid],
			}
			if a.hasMiles {
				d.MinMiles = a.minMiles
			}
			if dev, ok := devByFactory[fid]; ok {
				d.VIN = normalizeNearbyVIN(dev.VIN)
				if oil.HasLogisticsPersonnel(dev.DisplayName) {
					continue
				}
				if dev.LinkedCarEFleetsID != nil {
					d.LinkedCar = strings.TrimSpace(*dev.LinkedCarEFleetsID)
				}
				if d.DeviceID == "" {
					d.DeviceID = strings.TrimSpace(dev.DeviceID)
				}
			}
			for s := range a.stations {
				d.Stations = append(d.Stations, s)
			}
			sort.Strings(d.Stations)
			switch {
			case d.ExclusiveFills >= NearbyCertainFills:
				d.Rank = NearbyCertain
			case d.ExclusiveFills >= NearbyLikelyFills:
				d.Rank = NearbyLikely
			default:
				d.Rank = NearbyWatch
			}
			devicesOut = append(devicesOut, d)
		}
		sort.Slice(devicesOut, func(i, j int) bool {
			if devicesOut[i].ExclusiveFills != devicesOut[j].ExclusiveFills {
				return devicesOut[i].ExclusiveFills > devicesOut[j].ExclusiveFills
			}
			if devicesOut[i].AtFillFills != devicesOut[j].AtFillFills {
				return devicesOut[i].AtFillFills > devicesOut[j].AtFillFills
			}
			if devicesOut[i].Fills != devicesOut[j].Fills {
				return devicesOut[i].Fills > devicesOut[j].Fills
			}
			return devicesOut[i].FactoryID < devicesOut[j].FactoryID
		})
		for _, d := range devicesOut {
			switch d.Rank {
			case NearbyCertain:
				nc.Certain = append(nc.Certain, d)
			case NearbyLikely:
				nc.Likely = append(nc.Likely, d)
			default:
				nc.Watch = append(nc.Watch, d)
			}
		}
		switch {
		case len(nc.Certain) > 0:
			nc.Why = fmt.Sprintf("exclusive factory_id %s at fill time on %d days", nc.Certain[0].FactoryID, nc.Certain[0].ExclusiveFills)
		case len(nc.Likely) > 0:
			nc.Why = fmt.Sprintf("likely factory_id %s at fill time on %d days — keep watching", nc.Likely[0].FactoryID, nc.Likely[0].ExclusiveFills)
		case len(nc.Watch) > 0:
			nc.Why = fmt.Sprintf("%d device(s) within 1 mile on fill-day ±1 — watch, not a join", len(nc.Watch))
		default:
			nc.Why = "no GPS box within 1 mile of a mapped pump on fill-day ±1"
		}
		out.Cards = append(out.Cards, nc)
		out.Certain += len(nc.Certain)
		out.Likely += len(nc.Likely)
		out.Watch += len(nc.Watch)
	}
	sort.Slice(out.Cards, func(i, j int) bool {
		rank := func(c NearbyCard) int {
			if len(c.Certain) > 0 {
				return 0
			}
			if len(c.Likely) > 0 {
				return 1
			}
			if len(c.Watch) > 0 {
				return 2
			}
			return 3
		}
		if rank(out.Cards[i]) != rank(out.Cards[j]) {
			return rank(out.Cards[i]) < rank(out.Cards[j])
		}
		return out.Cards[i].CardID < out.Cards[j].CardID
	})
	if out.Cards == nil {
		out.Cards = []NearbyCard{}
	}
	return out
}

func unknownCardIDs(txs []model.CardTx) map[string]struct{} {
	pairings := ScorePairings(txs, time.Time{})
	unknown := UnknownMatchups(txs, pairings)
	out := map[string]struct{}{}
	for _, u := range unknown {
		if strings.TrimSpace(u.CardID) != "" {
			out[u.CardID] = struct{}{}
		}
	}
	// Cards with no GPS-called car yet (exclusive sit missed them).
	called := map[string]struct{}{}
	for _, t := range txs {
		if strings.TrimSpace(t.CalledEFleetsID) != "" && !isUnknownCar(t.CalledEFleetsID) {
			called[t.CardID] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	for _, t := range txs {
		id := strings.TrimSpace(t.CardID)
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
	}
	for id := range seen {
		if _, ok := called[id]; !ok {
			out[id] = struct{}{}
		}
	}
	return out
}

func indexGeocoded(stations []GeocodedStation) map[string]nearbyStation {
	out := map[string]nearbyStation{}
	for _, s := range stations {
		k := stationKey(s.Name, s.Address)
		if k == "" {
			continue
		}
		out[k] = nearbyStation{key: k, name: s.Name, addr: s.Address, lat: s.Lat, lng: s.Lng, hasPos: s.Lat != 0 || s.Lng != 0}
	}
	return out
}

func resolveStation(tx model.CardTx, geo map[string]nearbyStation) nearbyStation {
	k := stationKey(tx.StationName, tx.StationAddress)
	if s, ok := geo[k]; ok {
		return s
	}
	return nearbyStation{
		key:  k,
		name: strings.TrimSpace(tx.StationName),
		addr: strings.TrimSpace(tx.StationAddress),
	}
}

func stopOverlapsFill(v model.StopVisit, fill time.Time, slack time.Duration) bool {
	if fill.IsZero() || v.From.IsZero() {
		return false
	}
	from := v.From.Add(-slack)
	to := v.To
	if to.IsZero() {
		to = v.From
	}
	to = to.Add(slack)
	return !fill.Before(from) && !fill.After(to)
}

func normalizeNearbyVIN(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// FormatNearby is the operator listing. factory_id is the identity; labels are omitted.
func FormatNearby(res NearbyResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "nearby certain=%d likely=%d watch=%d cards=%d radius=1mi window=fill-day±1\n",
		res.Certain, res.Likely, res.Watch, len(res.Cards))
	for _, c := range res.Cards {
		fmt.Fprintf(&b, "card=%s fills=%d certain=%d likely=%d watch=%d %s\n",
			c.CardID, c.Fills, len(c.Certain), len(c.Likely), len(c.Watch), c.Why)
		printDevs := func(label string, ds []NearbyDevice) {
			for _, d := range ds {
				car := d.LinkedCar
				if car == "" {
					car = "-"
				}
				fmt.Fprintf(&b, "  %s factory_id=%s device_id=%s car=%s fills=%d at_fill=%d exclusive=%d",
					label, d.FactoryID, d.DeviceID, car, d.Fills, d.AtFillFills, d.ExclusiveFills)
				if d.MinMiles > 0 {
					fmt.Fprintf(&b, " min_mi=%.2f", d.MinMiles)
				}
				b.WriteByte('\n')
			}
		}
		printDevs("CERTAIN", c.Certain)
		printDevs("LIKELY", c.Likely)
		const watchCap = 5
		w := c.Watch
		if len(w) > watchCap {
			w = w[:watchCap]
		}
		printDevs("WATCH", w)
	}
	return b.String()
}

// CertainLinkedCars are exclusive nearby matches that already have a factory_id→car link.
// Unpaired boxes are not returned — that would invent a join.
func CertainLinkedCars(res NearbyResult) []NearbyDevice {
	var out []NearbyDevice
	for _, c := range res.Cards {
		for _, d := range c.Certain {
			if strings.TrimSpace(d.LinkedCar) == "" {
				continue
			}
			if oil.HasLogisticsPersonnel(d.LinkedCar) {
				continue
			}
			out = append(out, d)
		}
	}
	return out
}
