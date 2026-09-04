package cards

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// Holder types for a card era. car is the GPS-named vehicle; person/office
// are driver-kept or office cards and must never become a device↔car join.
const (
	HolderCar    = "car"
	HolderPerson = "person"
	HolderOffice = "office"
)

// DefaultLadderRungs is 3 exclusive pumps, then 5, then 10.
var DefaultLadderRungs = []int{3, 5, 10}

// KnownCoverageTarget is the operator goal: 95% of roster cars have both a
// factory_id link and at least one GPS-named car card era.
const KnownCoverageTarget = 95.0

// LadderCard is one card classified by exclusive pump sits.
type LadderCard struct {
	CardID    string   `json:"card_id"`
	Bucket    string   `json:"bucket"` // car, person, office
	HolderKey string   `json:"holder_key"`
	Nickname  string   `json:"nickname,omitempty"`
	Rung      int      `json:"rung,omitempty"`
	StationN  int      `json:"station_n"`
	Stations  []string `json:"stations,omitempty"`
	Split     bool     `json:"split"`
	EvidenceN int      `json:"evidence_n"`
	Cars      []string `json:"cars,omitempty"`
}

// LadderRung is the Cars / People / Offices list at one exclusive-station count.
type LadderRung struct {
	Stations int          `json:"stations"`
	Cars     []LadderCard `json:"cars"`
	People   []LadderCard `json:"people"`
	Offices  []LadderCard `json:"offices"`
}

// Coverage is roster known% = factory_id link AND a GPS-named car card era.
// Last Reading HOLDs are out of scope — missing miles stay HOLD.
type Coverage struct {
	RosterN          int      `json:"roster_n"`
	DeviceLinkedN    int      `json:"device_linked_n"`
	CardEraN         int      `json:"card_era_n"`
	KnownN           int      `json:"known_n"`
	LadderLockedN    int      `json:"ladder_locked_n"`
	DevicePct        float64  `json:"device_pct"`
	CardEraPct       float64  `json:"card_era_pct"`
	KnownPct         float64  `json:"known_pct"`
	UnknownRemaining []string `json:"unknown_remaining"`
	MissingDevice    []string `json:"missing_device"`
	MissingCardEra   []string `json:"missing_card_era"`
	Blocked          string   `json:"blocked,omitempty"`
}

// LadderResult is the station ladder plus coverage. It never writes Last Reading.
type LadderResult struct {
	Rungs    []LadderRung `json:"rungs"`
	Cars     []LadderCard `json:"cars"`
	People   []LadderCard `json:"people"`
	Offices  []LadderCard `json:"offices"`
	Eras     []CardEra    `json:"eras"`
	Coverage Coverage     `json:"coverage"`
	GPS      GPSFirstResult
}

// ClimbStationLadder starts from GPS exclusive pump sits, then names cards at
// 3 / 5 / 10 stations. Driver-kept and logistics cards stay on the person.
func ClimbStationLadder(visits []model.StopVisit, txs []model.CardTx, fleet []model.Car, devices []model.OneStepDevice, slack time.Duration, rungs []int) LadderResult {
	if slack <= 0 {
		slack = DefaultStopSlack
	}
	gps := MatchGPSFirst(visits, txs, fleet, slack)
	return ClassifyLadder(gps, txs, fleet, devices, rungs)
}

type cardInfo struct {
	logistics bool
	office    string
	person    string
	from, to  time.Time
	stations  map[string]struct{}
	n         int
}

type carHit struct {
	stations map[string]struct{}
	evidence int
	from, to time.Time
	nickname string
}

// ClassifyLadder buckets GPS-first exclusive sits into Cars / People / Offices.
func ClassifyLadder(gps GPSFirstResult, txs []model.CardTx, fleet []model.Car, devices []model.OneStepDevice, rungs []int) LadderResult {
	if len(rungs) == 0 {
		rungs = append([]int(nil), DefaultLadderRungs...)
	}
	nick := map[string]string{}
	for _, c := range fleet {
		id := strings.TrimSpace(c.EFleetsID)
		if id == "" {
			continue
		}
		nick[id] = strings.TrimSpace(c.Nickname)
	}

	info := map[string]*cardInfo{}
	touch := func(card string) *cardInfo {
		c := info[card]
		if c == nil {
			c = &cardInfo{stations: map[string]struct{}{}}
			info[card] = c
		}
		return c
	}
	for _, t := range txs {
		card := strings.TrimSpace(t.CardID)
		if card == "" {
			continue
		}
		c := touch(card)
		c.n++
		if c.from.IsZero() || t.At.Before(c.from) {
			c.from = t.At
		}
		if c.to.IsZero() || t.At.After(c.to) {
			c.to = t.At
		}
		if oil.HasLogisticsPersonnel(t.DriverFirst, t.DriverLast) {
			c.logistics = true
		}
		if p := personKey(t.DriverFirst, t.DriverLast); p != "" {
			if c.person == "" {
				c.person = p
			} else if c.person != p {
				c.person = c.person + "|" + p
			}
		}
		if off := officeHolder(t); off != "" {
			c.office = off
		}
		if st := firstNonEmpty(t.StationName, stationKey(t.StationName, t.StationAddress)); st != "" && !skipStationName(st) {
			c.stations[st] = struct{}{}
		}
	}

	byCardCar := map[string]map[string]*carHit{}
	for _, m := range gps.Matches {
		card := strings.TrimSpace(m.CardID)
		car := strings.TrimSpace(m.EFleetsID)
		if card == "" || car == "" || isUnknownCar(car) {
			continue
		}
		if byCardCar[card] == nil {
			byCardCar[card] = map[string]*carHit{}
		}
		h := byCardCar[card][car]
		if h == nil {
			h = &carHit{stations: map[string]struct{}{}, nickname: nick[car]}
			byCardCar[card][car] = h
		}
		h.evidence += m.EvidenceN
		for _, s := range m.Stations {
			s = strings.TrimSpace(s)
			if s == "" || skipStationName(s) {
				continue
			}
			h.stations[s] = struct{}{}
		}
	}
	for _, e := range gps.Eras {
		if eraHolderType(e) != HolderCar {
			continue
		}
		card := strings.TrimSpace(e.CardID)
		car := strings.TrimSpace(e.EFleetsID)
		if card == "" || car == "" {
			continue
		}
		if byCardCar[card] == nil {
			byCardCar[card] = map[string]*carHit{}
		}
		h := byCardCar[card][car]
		if h == nil {
			h = &carHit{stations: map[string]struct{}{}, nickname: firstNonEmpty(e.Nickname, nick[car])}
			byCardCar[card][car] = h
		}
		if h.from.IsZero() || e.From.Before(h.from) {
			h.from = e.From
		}
		if h.to.IsZero() || e.To.After(h.to) {
			h.to = e.To
		}
		if h.evidence < e.EvidenceN {
			h.evidence = e.EvidenceN
		}
		for _, s := range e.Stations {
			s = strings.TrimSpace(s)
			if s == "" || skipStationName(s) {
				continue
			}
			h.stations[s] = struct{}{}
		}
	}

	cardsSeen := map[string]struct{}{}
	for c := range info {
		cardsSeen[c] = struct{}{}
	}
	for c := range byCardCar {
		cardsSeen[c] = struct{}{}
	}

	var cars, people, offices []LadderCard
	classified := map[string]LadderCard{}
	for card := range cardsSeen {
		lc := classifyCard(card, info[card], byCardCar[card], rungs)
		classified[card] = lc
		switch lc.Bucket {
		case HolderCar:
			cars = append(cars, lc)
		case HolderPerson:
			people = append(people, lc)
		case HolderOffice:
			offices = append(offices, lc)
		}
	}
	sortLadderCards(cars)
	sortLadderCards(people)
	sortLadderCards(offices)

	var rungsOut []LadderRung
	for _, n := range rungs {
		rg := LadderRung{Stations: n, People: people, Offices: offices}
		for _, c := range cars {
			if c.StationN >= n {
				rg.Cars = append(rg.Cars, c)
			}
		}
		if rg.Cars == nil {
			rg.Cars = []LadderCard{}
		}
		if rg.People == nil {
			rg.People = []LadderCard{}
		}
		if rg.Offices == nil {
			rg.Offices = []LadderCard{}
		}
		rungsOut = append(rungsOut, rg)
	}

	eras := mergeLadderEras(gps.Eras, classified, info, byCardCar, nick)
	cov := RosterCoverage(fleet, devices, eras, cars)
	cov.Blocked = LadderBlocker(txs, nil)
	return LadderResult{
		Rungs:    rungsOut,
		Cars:     cars,
		People:   people,
		Offices:  offices,
		Eras:     eras,
		Coverage: cov,
		GPS:      gps,
	}
}

func classifyCard(card string, inf *cardInfo, hits map[string]*carHit, rungs []int) LadderCard {
	lc := LadderCard{CardID: card, Stations: []string{}, Cars: []string{}}
	minLock := 3
	if len(rungs) > 0 {
		minLock = rungs[0]
		for _, r := range rungs {
			if r > 0 && r < minLock {
				minLock = r
			}
		}
	}

	type scored struct {
		car string
		n   int
		h   *carHit
	}
	var locked []scored
	var any []scored
	for car, h := range hits {
		n := len(h.stations)
		any = append(any, scored{car, n, h})
		if n >= minLock {
			locked = append(locked, scored{car, n, h})
		}
	}
	sort.Slice(locked, func(i, j int) bool {
		if locked[i].n != locked[j].n {
			return locked[i].n > locked[j].n
		}
		return locked[i].car < locked[j].car
	})
	sort.Slice(any, func(i, j int) bool {
		if any[i].n != any[j].n {
			return any[i].n > any[j].n
		}
		return any[i].car < any[j].car
	})

	if inf != nil && inf.logistics {
		lc.Bucket = HolderPerson
		if inf.person != "" {
			lc.HolderKey = strings.Split(inf.person, "|")[0]
			lc.Nickname = lc.HolderKey
		} else {
			lc.HolderKey = "LOGISTICS_PERSONNEL"
			lc.Nickname = lc.HolderKey
		}
		lc.EvidenceN = inf.n
		lc.Stations = sortedKeys(inf.stations)
		lc.StationN = len(lc.Stations)
		for _, a := range any {
			lc.Cars = append(lc.Cars, a.car)
		}
		return lc
	}

	if len(locked) >= 2 {
		lc.Bucket = HolderCar
		lc.Split = true
		lc.HolderKey = locked[0].car
		lc.Nickname = locked[0].h.nickname
		lc.StationN = locked[0].n
		lc.Stations = sortedKeys(locked[0].h.stations)
		lc.EvidenceN = locked[0].h.evidence
		lc.Rung = rungFor(locked[0].n, rungs)
		for _, a := range locked {
			lc.Cars = append(lc.Cars, a.car)
		}
		return lc
	}
	if len(locked) == 1 {
		a := locked[0]
		lc.Bucket = HolderCar
		lc.HolderKey = a.car
		lc.Nickname = a.h.nickname
		lc.StationN = a.n
		lc.Stations = sortedKeys(a.h.stations)
		lc.EvidenceN = a.h.evidence
		lc.Rung = rungFor(a.n, rungs)
		lc.Cars = []string{a.car}
		return lc
	}

	// Below the first rung: driver-kept stays on the person; office cards stay office.
	if inf != nil && inf.person != "" && !strings.Contains(inf.person, "|") {
		lc.Bucket = HolderPerson
		lc.HolderKey = inf.person
		lc.Nickname = inf.person
		lc.EvidenceN = inf.n
		lc.Stations = sortedKeys(inf.stations)
		lc.StationN = len(lc.Stations)
		for _, a := range any {
			lc.Cars = append(lc.Cars, a.car)
		}
		return lc
	}
	if inf != nil && inf.office != "" {
		lc.Bucket = HolderOffice
		lc.HolderKey = inf.office
		lc.Nickname = inf.office
		lc.EvidenceN = inf.n
		lc.Stations = sortedKeys(inf.stations)
		lc.StationN = len(lc.Stations)
		return lc
	}
	lc.Bucket = ""
	if inf != nil {
		lc.EvidenceN = inf.n
		lc.Stations = sortedKeys(inf.stations)
		lc.StationN = len(lc.Stations)
	}
	for _, a := range any {
		lc.Cars = append(lc.Cars, a.car)
	}
	return lc
}

func mergeLadderEras(gpsEras []CardEra, classified map[string]LadderCard, info map[string]*cardInfo, hits map[string]map[string]*carHit, nick map[string]string) []CardEra {
	dropCar := map[string]struct{}{}
	for card, lc := range classified {
		if lc.Bucket == HolderPerson || lc.Bucket == HolderOffice {
			dropCar[card] = struct{}{}
		}
	}
	var out []CardEra
	splitCards := map[string]struct{}{}
	for _, lc := range classified {
		if lc.Split {
			splitCards[lc.CardID] = struct{}{}
		}
	}
	for _, e := range gpsEras {
		if _, drop := dropCar[e.CardID]; drop {
			continue
		}
		if eraHolderType(e) == HolderCar {
			if lc, ok := classified[e.CardID]; ok && lc.Bucket == HolderCar {
				e.HolderType = HolderCar
				e.HolderKey = firstNonEmpty(e.HolderKey, e.EFleetsID)
				e.Rung = lc.Rung
				if _, ok := splitCards[e.CardID]; ok {
					e.Split = true
				}
			}
		}
		out = append(out, e)
	}
	seenPerson := map[string]struct{}{}
	for card, lc := range classified {
		switch lc.Bucket {
		case HolderPerson, HolderOffice:
			if _, ok := seenPerson[card]; ok {
				continue
			}
			seenPerson[card] = struct{}{}
			inf := info[card]
			era := CardEra{
				CardID:     card,
				HolderType: lc.Bucket,
				HolderKey:  lc.HolderKey,
				Nickname:   lc.Nickname,
				EvidenceN:  lc.EvidenceN,
				Stations:   lc.Stations,
				Rung:       lc.Rung,
			}
			if inf != nil {
				era.From = inf.from
				era.To = inf.to
				if era.EvidenceN == 0 {
					era.EvidenceN = inf.n
				}
			}
			out = append(out, era)
		case HolderCar:
			if lc.Split {
				for _, car := range lc.Cars {
					h := hits[card][car]
					if h == nil {
						continue
					}
					already := false
					for _, e := range out {
						if e.CardID == card && e.EFleetsID == car && eraHolderType(e) == HolderCar {
							already = true
							break
						}
					}
					if already {
						continue
					}
					out = append(out, CardEra{
						CardID:     card,
						EFleetsID:  car,
						Nickname:   firstNonEmpty(h.nickname, nick[car]),
						HolderType: HolderCar,
						HolderKey:  car,
						From:       h.from,
						To:         h.to,
						EvidenceN:  h.evidence,
						Stations:   sortedKeys(h.stations),
						Split:      true,
						Rung:       rungFor(len(h.stations), DefaultLadderRungs),
					})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CardID != out[j].CardID {
			return out[i].CardID < out[j].CardID
		}
		if out[i].HolderType != out[j].HolderType {
			return out[i].HolderType < out[j].HolderType
		}
		if out[i].EFleetsID != out[j].EFleetsID {
			return out[i].EFleetsID < out[j].EFleetsID
		}
		return out[i].From.Before(out[j].From)
	})
	return out
}

// RosterCoverage counts roster cars with (a) factory_id link (b) a car-typed card era.
func RosterCoverage(fleet []model.Car, devices []model.OneStepDevice, eras []CardEra, locked []LadderCard) Coverage {
	linked := map[string]struct{}{}
	for _, d := range devices {
		if d.LinkedCarEFleetsID == nil {
			continue
		}
		car := strings.TrimSpace(*d.LinkedCarEFleetsID)
		if car == "" || isUnknownCar(car) {
			continue
		}
		if oil.HasLogisticsPersonnel(d.DisplayName) {
			continue
		}
		linked[car] = struct{}{}
	}
	eraCars := map[string]struct{}{}
	for _, e := range eras {
		if eraHolderType(e) != HolderCar {
			continue
		}
		car := strings.TrimSpace(e.EFleetsID)
		if car == "" || isUnknownCar(car) {
			continue
		}
		eraCars[car] = struct{}{}
	}
	lockedCars := map[string]struct{}{}
	for _, c := range locked {
		if c.Bucket != HolderCar {
			continue
		}
		for _, car := range c.Cars {
			if car != "" && !isUnknownCar(car) {
				lockedCars[car] = struct{}{}
			}
		}
		if c.HolderKey != "" && !isUnknownCar(c.HolderKey) {
			lockedCars[c.HolderKey] = struct{}{}
		}
	}

	var ids []string
	seen := map[string]struct{}{}
	for _, c := range fleet {
		id := strings.TrimSpace(c.EFleetsID)
		if id == "" || isUnknownCar(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	cov := Coverage{RosterN: len(ids)}
	for _, id := range ids {
		_, hasDev := linked[id]
		_, hasEra := eraCars[id]
		if hasDev {
			cov.DeviceLinkedN++
		} else {
			cov.MissingDevice = append(cov.MissingDevice, id)
		}
		if hasEra {
			cov.CardEraN++
		} else {
			cov.MissingCardEra = append(cov.MissingCardEra, id)
		}
		if _, ok := lockedCars[id]; ok {
			cov.LadderLockedN++
		}
		if hasDev && hasEra {
			cov.KnownN++
		} else {
			cov.UnknownRemaining = append(cov.UnknownRemaining, id)
		}
	}
	cov.DevicePct = pct(cov.DeviceLinkedN, cov.RosterN)
	cov.CardEraPct = pct(cov.CardEraN, cov.RosterN)
	cov.KnownPct = pct(cov.KnownN, cov.RosterN)
	if cov.MissingDevice == nil {
		cov.MissingDevice = []string{}
	}
	if cov.MissingCardEra == nil {
		cov.MissingCardEra = []string{}
	}
	if cov.UnknownRemaining == nil {
		cov.UnknownRemaining = []string{}
	}
	return cov
}

// LadderBlocker explains why live coverage can sit below 95% without inventing punches.
func LadderBlocker(txs []model.CardTx, visits []model.StopVisit) string {
	named, skipped := 0, 0
	for _, t := range txs {
		if strings.TrimSpace(t.CardID) == "" {
			continue
		}
		if skipStationName(t.StationName) {
			skipped++
			continue
		}
		named++
	}
	if named == 0 && skipped > 0 {
		return "all DETAILS merchants are TRACKER/empty; station ladder cannot name cards without real pump names"
	}
	if skipped > 0 && named < skipped {
		return fmt.Sprintf("DETAILS merchants are mostly TRACKER/empty (named=%d tracker_or_empty=%d); station ladder cannot name fleet cards without real pump names", named, skipped)
	}
	if named == 0 {
		return "no named fuel-card stations in DETAILS"
	}
	return ""
}

// FormatCoverage is the operator one-liner plus unknown remaining ids.
func FormatCoverage(c Coverage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "coverage roster=%d device_link=%d (%.1f%%) card_era=%d (%.1f%%) known=%d (%.1f%%) ladder_locked=%d target=%.0f%%\n",
		c.RosterN, c.DeviceLinkedN, c.DevicePct, c.CardEraN, c.CardEraPct, c.KnownN, c.KnownPct, c.LadderLockedN, KnownCoverageTarget)
	fmt.Fprintf(&b, "unknown remaining=%d missing_device=%d missing_card_era=%d\n",
		len(c.UnknownRemaining), len(c.MissingDevice), len(c.MissingCardEra))
	if c.Blocked != "" {
		fmt.Fprintf(&b, "blocked: %s\n", c.Blocked)
	}
	if c.KnownPct+1e-9 >= KnownCoverageTarget {
		b.WriteString("target met\n")
	} else {
		fmt.Fprintf(&b, "short of 95%% known")
		if c.Blocked != "" {
			b.WriteString(" (data, not a guess)\n")
		} else {
			b.WriteByte('\n')
		}
	}
	if n := len(c.UnknownRemaining); n > 0 {
		b.WriteString("unknown:")
		limit := n
		if limit > 40 {
			limit = 40
		}
		for _, id := range c.UnknownRemaining[:limit] {
			b.WriteByte(' ')
			b.WriteString(id)
		}
		if n > 40 {
			fmt.Fprintf(&b, " ... +%d", n-40)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func FormatLadder(res LadderResult) string {
	var b strings.Builder
	for _, rg := range res.Rungs {
		fmt.Fprintf(&b, "ladder rung=%d cars=%d people=%d offices=%d\n", rg.Stations, len(rg.Cars), len(rg.People), len(rg.Offices))
		for _, c := range rg.Cars {
			flag := "HOME"
			if c.Split {
				flag = "SPLIT"
			}
			fmt.Fprintf(&b, "  %s card=%s car=%s name=%s stations=%d rung=%d n=%d\n",
				flag, c.CardID, c.HolderKey, c.Nickname, c.StationN, c.Rung, c.EvidenceN)
		}
	}
	const peopleCap = 12
	people := res.People
	if len(people) > peopleCap {
		fmt.Fprintf(&b, "PERSON showing %d of %d\n", peopleCap, len(people))
		people = people[:peopleCap]
	}
	for _, c := range people {
		fmt.Fprintf(&b, "PERSON card=%s holder=%s stations=%d n=%d\n", c.CardID, c.HolderKey, c.StationN, c.EvidenceN)
	}
	offices := res.Offices
	if len(offices) > peopleCap {
		fmt.Fprintf(&b, "OFFICE showing %d of %d\n", peopleCap, len(offices))
		offices = offices[:peopleCap]
	}
	for _, c := range offices {
		fmt.Fprintf(&b, "OFFICE card=%s holder=%s stations=%d n=%d\n", c.CardID, c.HolderKey, c.StationN, c.EvidenceN)
	}
	b.WriteString(FormatCoverage(res.Coverage))
	return b.String()
}

func eraHolderType(e CardEra) string {
	if strings.TrimSpace(e.HolderType) == "" {
		return HolderCar
	}
	return e.HolderType
}

func rungFor(n int, rungs []int) int {
	got := 0
	for _, r := range rungs {
		if n >= r && r > got {
			got = r
		}
	}
	return got
}

func officeHolder(t model.CardTx) string {
	for _, s := range []string{t.RecordedEFleetsID, t.RecordedCVN} {
		if isOfficeLabel(s) {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func isOfficeLabel(s string) bool {
	u := strings.ToLower(strings.TrimSpace(s))
	if u == "" {
		return false
	}
	for _, tok := range []string{"office", "hq", "warehouse"} {
		if strings.Contains(u, tok) {
			return true
		}
	}
	if u == "yard" || strings.Contains(u, " yard") || strings.HasSuffix(u, "yard") {
		return true
	}
	return false
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortLadderCards(s []LadderCard) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].CardID != s[j].CardID {
			return s[i].CardID < s[j].CardID
		}
		return s[i].HolderKey < s[j].HolderKey
	})
}

func pct(n, d int) float64 {
	if d <= 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}
