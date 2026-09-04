package cards

import (
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// DefaultWatchFills is newest-first punches fetched per unknown card.
const DefaultWatchFills = 10

// NewestFillsFirst copies txs ordered by provider swipe time, newest first.
func NewestFillsFirst(txs []model.CardTx) []model.CardTx {
	out := append([]model.CardTx(nil), txs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].CardID < out[j].CardID
		}
		return out[i].At.After(out[j].At)
	})
	return out
}

// WatchFillBatch is the newest `max` punches (provider time, not bank posting).
func WatchFillBatch(txs []model.CardTx, max int) []model.CardTx {
	if max <= 0 {
		max = DefaultWatchFills
	}
	txs = NewestFillsFirst(txs)
	if len(txs) > max {
		return txs[:max]
	}
	return txs
}

// FillsForCard returns that card's swipes, newest first.
func FillsForCard(txs []model.CardTx, cardID string) []model.CardTx {
	cardID = strings.TrimSpace(cardID)
	var out []model.CardTx
	for _, t := range txs {
		if strings.TrimSpace(t.CardID) == cardID {
			out = append(out, t)
		}
	}
	return NewestFillsFirst(out)
}

// NicknameRegion is leading letters (VA15→VA, WNY8→WNY). 292NCX is VA.
func NicknameRegion(s string) string {
	u := strings.ToUpper(strings.TrimSpace(s))
	if len(u) == 6 && strings.HasSuffix(u, "NCX") {
		return "VA"
	}
	var b strings.Builder
	for _, r := range u {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
			continue
		}
		break
	}
	return b.String()
}

func looksVirginiaID(id string) bool {
	u := strings.ToUpper(strings.TrimSpace(id))
	if NicknameRegion(u) == "VA" {
		return true
	}
	// 27VA15 — year prefix then VA.
	if len(u) >= 4 && u[0] >= '0' && u[0] <= '9' && u[1] >= '0' && u[1] <= '9' && strings.HasPrefix(u[2:], "VA") {
		return true
	}
	return false
}

// IsVirginiaVehicle is a seed lock, not a join. DETAILS last-write-wins is
// trusted for VA nicknames/ids so we know which box to ask OneStep about.
func IsVirginiaVehicle(efleetsID, cvn string, cars []model.Car) bool {
	if looksVirginiaID(efleetsID) || looksVirginiaID(cvn) {
		return true
	}
	want := strings.TrimSpace(efleetsID)
	if want == "" {
		want = strings.TrimSpace(cvn)
	}
	for _, c := range cars {
		id := strings.TrimSpace(c.EFleetsID)
		nick := strings.TrimSpace(c.Nickname)
		if id != want && nick != want && nick != strings.TrimSpace(cvn) && id != strings.TrimSpace(cvn) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(c.Region), "VA") {
			return true
		}
		if NicknameRegion(nick) == "VA" || looksVirginiaID(id) {
			return true
		}
	}
	return false
}

// FactoryIDsForLinkedCar returns active GPS boxes already joined to this car.
// display_name is never used. Unpaired boxes are not invented.
func FactoryIDsForLinkedCar(devices []model.OneStepDevice, efleetsID string) []string {
	efleetsID = strings.TrimSpace(efleetsID)
	if efleetsID == "" || isUnknownCar(efleetsID) || oil.HasLogisticsPersonnel(efleetsID) {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, d := range devices {
		if d.Dead || !d.Active {
			continue
		}
		if oil.HasLogisticsPersonnel(d.DisplayName) {
			continue
		}
		if d.LinkedCarEFleetsID == nil || strings.TrimSpace(*d.LinkedCarEFleetsID) != efleetsID {
			continue
		}
		id := strings.TrimSpace(d.FactoryID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func nearbyDevicesForCard(prior NearbyResult, cardID string) []NearbyDevice {
	var out []NearbyDevice
	for _, c := range prior.Cards {
		if c.CardID != cardID {
			continue
		}
		out = append(out, c.Certain...)
		out = append(out, c.Likely...)
		out = append(out, c.Watch...)
	}
	return out
}

// SeedWatchedFactoryIDs is the API batch: 1-mile hunt hits, then VA recorded
// vehicles, then one hypothesis roster car if the list is still empty.
func SeedWatchedFactoryIDs(fills []model.CardTx, prior NearbyResult, devices []model.OneStepDevice, cars []model.Car) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	cardID := ""
	if len(fills) > 0 {
		cardID = strings.TrimSpace(fills[0].CardID)
	}
	for _, d := range nearbyDevicesForCard(prior, cardID) {
		add(d.FactoryID)
	}
	for _, t := range fills {
		rec := strings.TrimSpace(t.RecordedEFleetsID)
		if isUnknownCar(rec) {
			continue
		}
		if !IsVirginiaVehicle(rec, t.RecordedCVN, cars) {
			continue
		}
		for _, id := range FactoryIDsForLinkedCar(devices, rec) {
			add(id)
		}
	}
	if len(out) > 0 {
		sort.Strings(out)
		return out
	}
	for _, t := range fills {
		rec := strings.TrimSpace(t.RecordedEFleetsID)
		if isUnknownCar(rec) || oil.HasLogisticsPersonnel(rec) {
			continue
		}
		ids := FactoryIDsForLinkedCar(devices, rec)
		if len(ids) == 0 {
			continue
		}
		for _, id := range ids {
			add(id)
		}
		break
	}
	sort.Strings(out)
	return out
}

type watchCardRank struct {
	id        string
	exclusive int
	va        bool
	newest    time.Time
}

// WatchCardOrder processes likely/high-exclusive cards first, then Virginia
// recorded vehicles, then the rest by newest swipe.
func WatchCardOrder(txs []model.CardTx, prior NearbyResult, cars []model.Car) []string {
	by := map[string]*watchCardRank{}
	for _, t := range txs {
		id := strings.TrimSpace(t.CardID)
		if id == "" {
			continue
		}
		r := by[id]
		if r == nil {
			r = &watchCardRank{id: id}
			by[id] = r
		}
		if r.newest.IsZero() || t.At.After(r.newest) {
			r.newest = t.At
		}
		if IsVirginiaVehicle(t.RecordedEFleetsID, t.RecordedCVN, cars) {
			r.va = true
		}
	}
	for _, c := range prior.Cards {
		r := by[c.CardID]
		if r == nil {
			continue
		}
		for _, d := range nearbyDevicesForCard(prior, c.CardID) {
			if d.ExclusiveFills > r.exclusive {
				r.exclusive = d.ExclusiveFills
			}
		}
	}
	rows := make([]watchCardRank, 0, len(by))
	for _, r := range by {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].exclusive != rows[j].exclusive {
			return rows[i].exclusive > rows[j].exclusive
		}
		if rows[i].va != rows[j].va {
			return rows[i].va
		}
		if !rows[i].newest.Equal(rows[j].newest) {
			return rows[i].newest.After(rows[j].newest)
		}
		return rows[i].id < rows[j].id
	})
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.id)
	}
	return out
}

// DeviceCoveredInWindow is true when a visit spans the whole fetch window.
// A short stop inside a multi-day window is not coverage.
func DeviceCoveredInWindow(visits []model.StopVisit, factoryID string, from, to time.Time) bool {
	factoryID = strings.TrimSpace(factoryID)
	if factoryID == "" || from.IsZero() || to.IsZero() {
		return false
	}
	for _, v := range visits {
		if strings.TrimSpace(v.FactoryID) != factoryID {
			continue
		}
		start := v.From
		end := v.To
		if start.IsZero() && end.IsZero() {
			continue
		}
		if end.IsZero() {
			end = start
		}
		if start.IsZero() {
			start = end
		}
		if !start.After(from) && !end.Before(to) {
			return true
		}
	}
	return false
}

// WatchedCoverageComplete is true only when every watched factory_id spans from/to.
// An empty watch list is incomplete (nothing was asked).
func WatchedCoverageComplete(visits []model.StopVisit, factoryIDs []string, from, to time.Time) bool {
	if from.IsZero() || to.IsZero() || len(factoryIDs) == 0 {
		return false
	}
	for _, id := range factoryIDs {
		if !DeviceCoveredInWindow(visits, id, from, to) {
			return false
		}
	}
	return true
}

// UnionFillDayWindow is Eastern fill-day ±1 covering every swipe in txs.
func UnionFillDayWindow(txs []model.CardTx) (time.Time, time.Time) {
	var from, to time.Time
	for _, t := range txs {
		if t.At.IsZero() {
			continue
		}
		f, e := FillDayWindow(t.At)
		if from.IsZero() || f.Before(from) {
			from = f
		}
		if to.IsZero() || e.After(to) {
			to = e
		}
	}
	return from, to
}

func eraIsCar(e model.CardEra) bool {
	ht := strings.TrimSpace(e.HolderType)
	if ht == "" {
		ht = HolderCar
	}
	return ht == HolderCar
}

// PreserveUnladderedCarEras keeps GPS-named or nearby-certain car eras that
// rebuild's ladder did not replace, so a --no-gps rematch cannot wipe a
// watch-loop persist. Cards the ladder already named as a car are not copied.
func PreserveUnladderedCarEras(ladder, existing []model.CardEra) []model.CardEra {
	hasCar := map[string]struct{}{}
	for _, e := range ladder {
		if !eraIsCar(e) {
			continue
		}
		if id := strings.TrimSpace(e.CardID); id != "" {
			hasCar[id] = struct{}{}
		}
	}
	out := append([]model.CardEra(nil), ladder...)
	for _, e := range existing {
		if !eraIsCar(e) {
			continue
		}
		id := strings.TrimSpace(e.CardID)
		if id == "" {
			continue
		}
		if _, ok := hasCar[id]; ok {
			continue
		}
		out = append(out, e)
	}
	return out
}

// RewriteIncompleteWatchWhy replaces the fleet-coverage sentence after a
// watched-set hunt.
func RewriteIncompleteWatchWhy(res NearbyResult, complete bool) NearbyResult {
	if complete {
		return res
	}
	const msg = "incomplete GPS coverage for watched boxes — watch only until those factory_ids are fetched"
	for i := range res.Cards {
		if strings.Contains(res.Cards[i].Why, "incomplete") || res.Cards[i].Why == "" {
			res.Cards[i].Why = msg
		}
	}
	return res
}
