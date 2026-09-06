package model

import (
	"sort"
	"strings"
	"time"
)

// CardTxLessDesc reports whether a should appear before b when sorting provider
// transactions newest-first (Provider Transaction Date+Time desc). Tie-breakers
// are stable: card_id, recorded efleets id, odometer, source_row.
func CardTxLessDesc(a, b CardTx) bool {
	atA, atB := a.At.UTC(), b.At.UTC()
	if !atA.Equal(atB) {
		return atA.After(atB)
	}
	if a.CardID != b.CardID {
		return a.CardID < b.CardID
	}
	if a.RecordedEFleetsID != b.RecordedEFleetsID {
		return a.RecordedEFleetsID < b.RecordedEFleetsID
	}
	odoA, odoB := cardTxOdo(a), cardTxOdo(b)
	if odoA != odoB {
		return odoA < odoB
	}
	return a.SourceRow < b.SourceRow
}

// SortCardTxsDesc sorts txs newest-first with stable ties.
func SortCardTxsDesc(txs []CardTx) {
	sort.SliceStable(txs, func(i, j int) bool {
		return CardTxLessDesc(txs[i], txs[j])
	})
}

// FillLessDesc reports whether a should appear before b when sorting fuel punches
// newest-first (Provider Transaction Date+Time desc). Tie-breakers: card cvn,
// efleets id, odometer.
func FillLessDesc(a, b Fill) bool {
	atA, atB := a.ProviderTransactionTime.UTC(), b.ProviderTransactionTime.UTC()
	if !atA.Equal(atB) {
		return atA.After(atB)
	}
	if a.CardCompanyVehicleNumber != b.CardCompanyVehicleNumber {
		return a.CardCompanyVehicleNumber < b.CardCompanyVehicleNumber
	}
	if a.EFleetsID != b.EFleetsID {
		return a.EFleetsID < b.EFleetsID
	}
	odoA, odoB := fillOdo(a), fillOdo(b)
	if odoA != odoB {
		return odoA < odoB
	}
	return a.MerchantName < b.MerchantName
}

// SortFillsDesc sorts fills newest-first with stable ties.
func SortFillsDesc(fills []Fill) {
	sort.SliceStable(fills, func(i, j int) bool {
		return FillLessDesc(fills[i], fills[j])
	})
}

// ProviderTimeLessDesc compares two provider transaction timestamps for display
// lists (newest first) with an optional final tie string (e.g. tx_key).
func ProviderTimeLessDesc(atA, atB time.Time, tieA, tieB string) bool {
	a, b := atA.UTC(), atB.UTC()
	if !a.Equal(b) {
		return a.After(b)
	}
	return tieA < tieB
}

func cardTxOdo(t CardTx) int {
	if t.Odometer == nil {
		return 0
	}
	return *t.Odometer
}

func fillOdo(f Fill) int {
	if f.Odometer == nil {
		return 0
	}
	return *f.Odometer
}

// FillBlockLessDesc is the History desk comparator for one swipe block.
func FillBlockLessDesc(atA, atB time.Time, cardA, cardB, keyA, keyB string) bool {
	return ProviderTimeLessDesc(atA, atB, strings.Join([]string{cardA, keyA}, "|"), strings.Join([]string{cardB, keyB}, "|"))
}
