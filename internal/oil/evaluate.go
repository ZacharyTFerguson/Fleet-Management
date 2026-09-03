package oil

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
)

// ComputeIn is everything EvaluateHolds may see. No HTTP, no SQL.
type ComputeIn struct {
	Nickname          string
	Fills             []model.Fill
	ShopROs           []model.ShopRO
	Devices           []model.OneStepDevice
	MilesSince        []model.DriveStopMiles
	StoredLastReading *int
	OverrideLower     bool
}

// ComputeOut is the decision for one car. SkipWrite means leave last_reading_* unchanged.
type ComputeOut struct {
	SkipWrite     bool
	Holds         []model.Hold
	EnterpriseOdo int
	FillTime      time.Time
	MilesSince    float64
	Reading       int
	Source        string
}

type trusted struct {
	Odo    int
	At     time.Time
	Source string
}

// EvaluateHolds picks a trusted enterprise odo and decides HOLDs.
// LastReading only does the add; this sibling owns fill picker, band, shop-vs-fuel, and skip-write.
func EvaluateHolds(in ComputeIn) ComputeOut {
	var holds []model.Hold

	if HasLogisticsPersonnel(in.Nickname) {
		holds = append(holds, model.Hold{Code: model.HoldLogisticsPersonnel, Detail: "nickname is a logistics-personnel label; not a pairing key"})
	}

	chain, fillHolds := walkFills(in.Nickname, in.Fills)
	holds = append(holds, fillHolds...)
	if hasCode(holds, model.HoldSameSecondFill) {
		return skip(holds)
	}

	shop := latestShop(in.ShopROs)
	var chosen *trusted
	if len(chain) > 0 {
		last := chain[len(chain)-1]
		chosen = &last
	}

	if shop != nil && chosen != nil {
		// Shop typically beats fuel, then both must survive band. Shop can be wrong (VA10 spike).
		shopBand := ExpectedBand(chosen.Odo, shop.At.Sub(chosen.At), nil)
		if !shopBand.Contains(shop.Odo) && shopLooksAbandoned(*shop, in.Fills, chosen.Odo) {
			holds = append(holds, model.Hold{Code: model.HoldSpikeAbandoned, Detail: fmt.Sprintf("shop RO %d abandoned by later in-band fills", shop.Odo)})
		} else if shop.At.After(chosen.At) || shopBand.Contains(shop.Odo) || shop.At.Equal(chosen.At) {
			if !shopLooksAbandoned(*shop, in.Fills, chosen.Odo) {
				chosen = shop
			}
		}
	} else if shop != nil && chosen == nil {
		if shopLooksAbandoned(*shop, in.Fills, shop.Odo) {
			holds = append(holds, model.Hold{Code: model.HoldSpikeAbandoned, Detail: "shop RO is a spike with no surviving fill"})
		} else {
			chosen = shop
		}
	}

	if chosen == nil {
		if hasCode(holds, model.HoldUnusualY) || anyUnusual(in.Fills) {
			if !hasCode(holds, model.HoldUnusualY) {
				holds = append(holds, model.Hold{Code: model.HoldUnusualY, Detail: "no surviving fill; unusual-Y punches were not trusted"})
			}
			return skip(holds)
		}
		if hasCode(holds, model.HoldCardMix) {
			return skip(holds)
		}
		holds = append(holds, model.Hold{Code: model.HoldNoTrustedFill, Detail: "no DETAILS fill or shop RO survived checks"})
		return skip(holds)
	}

	live := liveDevices(in.Devices)
	if len(live) == 0 {
		holds = append(holds, model.Hold{Code: model.HoldNoDevice, Detail: "no live factory_id linked; dead boxes are not summed"})
		return skipTrusted(holds, chosen)
	}
	miles, mh := pickMilesSince(live, in.MilesSince, chosen.At)
	holds = append(holds, mh...)
	if hasCode(holds, model.HoldMultiDeviceFight) || hasCode(holds, model.HoldDeviceAmbiguous) || hasCode(holds, model.HoldNoDriveStop) {
		return skipTrusted(holds, chosen)
	}

	reading, lrHolds, err := LastReading(chosen.Odo, chosen.At, miles)
	if err != nil {
		holds = append(holds, model.Hold{Code: model.HoldNoDriveStop, Detail: err.Error()})
		return skipTrusted(holds, chosen)
	}
	holds = append(holds, lrHolds...)
	if hasCode(holds, model.HoldNoDriveStop) {
		return skipTrusted(holds, chosen)
	}

	if in.StoredLastReading != nil && reading < *in.StoredLastReading && !in.OverrideLower {
		holds = append(holds, model.Hold{
			Code:   model.HoldLowerReadingRefused,
			Detail: fmt.Sprintf("computed %d is lower than stored %d; pass --override-lower to write", reading, *in.StoredLastReading),
		})
		return skipTrusted(holds, chosen)
	}

	out := ComputeOut{
		SkipWrite:     len(blocking(holds)) > 0,
		Holds:         holds,
		EnterpriseOdo: chosen.Odo,
		FillTime:      chosen.At,
		MilesSince:    miles,
		Reading:       reading,
		Source:        chosen.Source,
	}
	if out.SkipWrite {
		out.Reading = 0
	}
	return out
}

// walkFills builds a time-ordered trusted chain. Latest raw punch is often a fat-finger.
func walkFills(nickname string, fills []model.Fill) ([]trusted, []model.Hold) {
	sorted := append([]model.Fill(nil), fills...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ProviderTransactionTime.Before(sorted[j].ProviderTransactionTime)
	})

	var holds []model.Hold
	if h := sameSecondConflict(sorted); h != nil {
		return nil, []model.Hold{*h}
	}
	var chain []trusted

	for _, f := range sorted {
		if f.Odometer == nil || *f.Odometer <= 0 {
			continue
		}
		if HasLogisticsPersonnel(f.DriverFirst, f.DriverLast) {
			// Ignore as pairing; the punch may still be a real fill for the car.
		}
		prov := strings.TrimSpace(f.ProviderCompanyVehicleNumber)
		nick := strings.TrimSpace(nickname)
		if nick != "" && prov != "" && !sameVehicleLabel(nick, prov) {
			holds = append(holds, model.Hold{Code: model.HoldCardMix, Detail: fmt.Sprintf("card %s is not car %s", prov, nick)})
			continue
		}

		odo := *f.Odometer
		at := f.ProviderTransactionTime.Truncate(time.Second)

		if len(chain) > 0 {
			prev := chain[len(chain)-1]
			elapsed := at.Sub(prev.At)
			band := ExpectedBand(prev.Odo, elapsed, nil)
			if odo < prev.Odo {
				repaired, ok := repairDigitSwap(odo, f.UnusualY, band)
				if ok {
					odo = repaired
				} else {
					holds = append(holds, model.Hold{Code: model.HoldOdoBackward, Detail: fmt.Sprintf("%d drops from %d", odo, prev.Odo)})
					continue
				}
			}
			if f.UnusualY {
				if !band.Contains(odo) {
					repaired, ok := repairDigitSwap(odo, true, band)
					if ok {
						odo = repaired
					} else {
						holds = append(holds, model.Hold{Code: model.HoldUnusualY, Detail: "unusual-Y punch not in band and not digit-swap repairable"})
						continue
					}
				}
			}
			if !band.Contains(odo) {
				if spikeAbandoned(odo, prev, sorted, at) {
					holds = append(holds, model.Hold{Code: model.HoldSpikeAbandoned, Detail: fmt.Sprintf("%d jumped out of band and later fills returned", odo)})
					continue
				}
				// Out of band without later confirmation: skip as untrusted, keep looking.
				continue
			}
		} else if f.UnusualY {
			holds = append(holds, model.Hold{Code: model.HoldUnusualY, Detail: "unusual-Y with no prior trusted fill to repair against"})
			continue
		}

		chain = append(chain, trusted{Odo: odo, At: at, Source: model.SourceFuelDetails})
	}
	return chain, uniqueHolds(holds)
}

// spikeAbandoned is the VA10 rule: a far jump not confirmed by the next 1–2 in-band readings is junk.
func spikeAbandoned(spike int, prev trusted, fills []model.Fill, spikeAt time.Time) bool {
	confirmed := 0
	for _, f := range fills {
		if f.Odometer == nil || !f.ProviderTransactionTime.After(spikeAt) {
			continue
		}
		later := *f.Odometer
		// Later reading in the old band proves the spike never happened. Odometers do not drop; this is not a rollback.
		oldBand := ExpectedBand(prev.Odo, f.ProviderTransactionTime.Sub(prev.At), nil)
		if oldBand.Contains(later) {
			confirmed++
		}
		if confirmed >= 1 {
			return true
		}
		if confirmed == 0 && later >= spike-minBandMiles {
			// Next punch stayed up at the spike — treat as real until more evidence.
			return false
		}
	}
	return false
}

// shopLooksAbandoned reports a shop odo that later fills walk away from (VA10).
func shopLooksAbandoned(shop trusted, fills []model.Fill, prior int) bool {
	for _, f := range fills {
		if f.Odometer == nil {
			continue
		}
		if f.ProviderTransactionTime.Before(shop.At) {
			continue
		}
		later := *f.Odometer
		// Later punch near the old chain and far below the shop spike proves the shop number never happened.
		if later < shop.Odo-500 && later <= prior+20000 {
			return true
		}
	}
	return false
}

// latestShop collapses line items to the newest RO by completed time.
func latestShop(ros []model.ShopRO) *trusted {
	var best *trusted
	for _, r := range ros {
		if r.Odometer <= 0 {
			continue
		}
		t := trusted{Odo: r.Odometer, At: r.At.Truncate(time.Second), Source: model.SourceShopRO}
		if best == nil || t.At.After(best.At) || (t.At.Equal(best.At) && t.Odo > best.Odo) {
			cp := t
			best = &cp
		}
	}
	return best
}

// sameSecondConflict HOLDs when two candidate punches share a second but not an odometer.
func sameSecondConflict(fills []model.Fill) *model.Hold {
	odos := map[int64]map[int]struct{}{}
	for _, f := range fills {
		if f.Odometer == nil || *f.Odometer <= 0 {
			continue
		}
		sec := f.ProviderTransactionTime.Truncate(time.Second).Unix()
		if odos[sec] == nil {
			odos[sec] = map[int]struct{}{}
		}
		odos[sec][*f.Odometer] = struct{}{}
	}
	for sec, set := range odos {
		if len(set) > 1 {
			return &model.Hold{Code: model.HoldSameSecondFill, Detail: fmt.Sprintf("two fills at unix %d with different odo", sec)}
		}
	}
	return nil
}

// liveDevices drops dead factory_ids so a retired box cannot add miles.
func liveDevices(devs []model.OneStepDevice) []model.OneStepDevice {
	var live []model.OneStepDevice
	for _, d := range devs {
		if d.Dead || d.FactoryID == "" {
			continue
		}
		if HasLogisticsPersonnel(d.DisplayName) {
			continue
		}
		live = append(live, d)
	}
	return live
}

// pickMilesSince uses one live box's stored trip sum after the trusted second. Missing measurement is HOLD, not zero.
func pickMilesSince(live []model.OneStepDevice, rows []model.DriveStopMiles, since time.Time) (float64, []model.Hold) {
	byID := map[string]float64{}
	since = since.Truncate(time.Second)
	for _, r := range rows {
		if r.Since.Truncate(time.Second).Equal(since) {
			byID[r.FactoryID] = r.Miles
		}
	}
	var vals []float64
	for _, d := range live {
		if m, ok := byID[d.FactoryID]; ok {
			vals = append(vals, m)
		}
	}
	if len(live) > 1 && len(vals) == 0 {
		return 0, []model.Hold{{Code: model.HoldDeviceAmbiguous, Detail: "multiple live factory_id with no miles-since"}}
	}
	if len(vals) == 0 {
		return 0, []model.Hold{{Code: model.HoldNoDriveStop, Detail: "no drive-stop miles after the trusted second"}}
	}
	if len(vals) > 1 {
		min, max := vals[0], vals[0]
		for _, v := range vals[1:] {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		if max-min > 50 && (min == 0 || max/min > 1.5) {
			return 0, []model.Hold{{Code: model.HoldMultiDeviceFight, Detail: fmt.Sprintf("live devices miles-since %.1f vs %.1f", min, max)}}
		}
	}
	best := vals[0]
	for _, v := range vals[1:] {
		if v > best {
			best = v
		}
	}
	return best, nil
}

// skip is the HOLD path: no last_reading_* write, so operators cannot trust a flagged number.
func skip(holds []model.Hold) ComputeOut {
	return ComputeOut{SkipWrite: true, Holds: uniqueHolds(holds)}
}

// skipTrusted keeps the trusted Enterprise anchor available to SyncOneStep
// while still refusing a Last Reading write. The first GPS fetch necessarily
// starts with no stored miles-since, so dropping FillTime here would prevent
// the fetch that resolves NO_DRIVESTOP.
func skipTrusted(holds []model.Hold, t *trusted) ComputeOut {
	out := skip(holds)
	if t != nil {
		out.EnterpriseOdo = t.Odo
		out.FillTime = t.At
		out.Source = t.Source
	}
	return out
}

// hasCode lets a later rule see that an earlier check already decided skip-write.
func hasCode(holds []model.Hold, code string) bool {
	for _, h := range holds {
		if h.Code == code {
			return true
		}
	}
	return false
}

// blocking is the HOLD set that forbids writing Last Reading (all current codes do).
func blocking(holds []model.Hold) []model.Hold {
	var out []model.Hold
	for _, h := range holds {
		switch h.Code {
		case model.HoldUnusualY, model.HoldOdoBackward, model.HoldCardMix,
			model.HoldNoDevice, model.HoldDeviceAmbiguous, model.HoldMultiDeviceFight,
			model.HoldNoTrustedFill, model.HoldNoDriveStop, model.HoldLowerReadingRefused,
			model.HoldSameSecondFill, model.HoldSpikeAbandoned, model.HoldLogisticsPersonnel:
			out = append(out, h)
		}
	}
	return out
}

// uniqueHolds drops duplicate codes from walking many punches so the operator sees one reason.
func uniqueHolds(holds []model.Hold) []model.Hold {
	seen := map[string]struct{}{}
	var out []model.Hold
	for _, h := range holds {
		k := h.Code + "\x00" + h.Detail
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, h)
	}
	return out
}

// anyUnusual prefers UNUSUAL_Y over generic NO_TRUSTED_FILL when every punch was flagged.
func anyUnusual(fills []model.Fill) bool {
	for _, f := range fills {
		if f.UnusualY {
			return true
		}
	}
	return false
}

// sameVehicleLabel compares nickname vs card company vehicle number without treating spaces as a different car.
func sameVehicleLabel(a, b string) bool {
	na := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(a), " ", ""))
	nb := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(b), " ", ""))
	return na == nb
}
