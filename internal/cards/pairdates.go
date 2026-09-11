package cards

import (
	"sort"
	"time"

	"oilchange/internal/model"
)

// ApplyPairDates fills pair_started / switched_at / next_pair_at on car eras.
// PairStarted defaults to From (first swipe evidence). SwitchedAt and NextPairAt
// are the first GPS anchor of the next car era when a card splits across vehicles.
func ApplyPairDates(eras []model.CardEra) []model.CardEra {
	byCard := map[string][]int{}
	for i, e := range eras {
		if eraHolderType(e) != HolderCar {
			continue
		}
		byCard[e.CardID] = append(byCard[e.CardID], i)
	}
	for _, idxs := range byCard {
		sort.Slice(idxs, func(i, j int) bool {
			a, b := eras[idxs[i]], eras[idxs[j]]
			ka, kb := eraPairSortKey(a), eraPairSortKey(b)
			if !ka.Equal(kb) {
				return ka.Before(kb)
			}
			return eras[idxs[i]].EFleetsID < eras[idxs[j]].EFleetsID
		})
		for j, cur := range idxs {
			if eras[cur].PairStarted.IsZero() {
				eras[cur].PairStarted = eras[cur].From
			}
			if j+1 >= len(idxs) {
				continue
			}
			next := idxs[j+1]
			if eras[cur].EFleetsID == eras[next].EFleetsID {
				continue
			}
			if eras[cur].SwitchedAt != nil && eras[cur].NextPairAt != nil {
				continue
			}
			sw := eraAnchorStart(eras[next])
			if sw.IsZero() {
				continue
			}
			t := sw
			eras[cur].SwitchedAt = &t
			eras[cur].NextPairAt = &t
		}
	}
	return eras
}

func eraPairSortKey(e model.CardEra) time.Time {
	if !e.PairStarted.IsZero() {
		return e.PairStarted
	}
	return e.From
}

func eraAnchorStart(e model.CardEra) time.Time {
	if !e.From.IsZero() {
		return e.From
	}
	return e.PairStarted
}

// FormatPairDates renders pair/switch columns for CLI output.
func FormatPairDates(e model.CardEra) string {
	ps := "-"
	if !e.PairStarted.IsZero() {
		ps = e.PairStarted.UTC().Format(time.RFC3339)
	}
	sw := "-"
	if e.SwitchedAt != nil && !e.SwitchedAt.IsZero() {
		sw = e.SwitchedAt.UTC().Format(time.RFC3339)
	}
	np := "-"
	if e.NextPairAt != nil && !e.NextPairAt.IsZero() {
		np = e.NextPairAt.UTC().Format(time.RFC3339)
	}
	return "pair_started=" + ps + " switched=" + sw + " next_pair=" + np
}
