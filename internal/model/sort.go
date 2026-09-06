package model

import (
	"sort"
	"time"
)

// This file is the single source of truth for how provider transactions are
// ordered when displayed newest-first (History board, cards watch loop, desk
// lists). The canonical rule: Provider Transaction Date+Time descending; a
// same-second tie breaks by higher odometer first (missing odometer last),
// then card id, then the remaining stable keys. store.ListCardTxs mirrors it:
//
//	ORDER BY at DESC, COALESCE(odometer,-1) DESC, card_id, recorded_efleets_id, source_row
//
// so a list read straight from SQL and a list re-sorted in memory agree.

// CardTxLessDesc reports whether a should appear before b when sorting provider
// transactions newest-first.
func CardTxLessDesc(a, b CardTx) bool {
	atA, atB := a.At.UTC(), b.At.UTC()
	if !atA.Equal(atB) {
		return atA.After(atB)
	}
	if oa, ob := odoOrMissing(a.Odometer), odoOrMissing(b.Odometer); oa != ob {
		return oa > ob
	}
	if a.CardID != b.CardID {
		return a.CardID < b.CardID
	}
	if a.RecordedEFleetsID != b.RecordedEFleetsID {
		return a.RecordedEFleetsID < b.RecordedEFleetsID
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
// newest-first, with the same tie rule as CardTxLessDesc.
func FillLessDesc(a, b Fill) bool {
	atA, atB := a.ProviderTransactionTime.UTC(), b.ProviderTransactionTime.UTC()
	if !atA.Equal(atB) {
		return atA.After(atB)
	}
	if oa, ob := odoOrMissing(a.Odometer), odoOrMissing(b.Odometer); oa != ob {
		return oa > ob
	}
	if a.CardCompanyVehicleNumber != b.CardCompanyVehicleNumber {
		return a.CardCompanyVehicleNumber < b.CardCompanyVehicleNumber
	}
	if a.EFleetsID != b.EFleetsID {
		return a.EFleetsID < b.EFleetsID
	}
	return a.MerchantName < b.MerchantName
}

// SortFillsDesc sorts fills newest-first with stable ties.
func SortFillsDesc(fills []Fill) {
	sort.SliceStable(fills, func(i, j int) bool {
		return FillLessDesc(fills[i], fills[j])
	})
}

// FillBlockLessDesc is the History desk comparator for one swipe block —
// the same canonical rule, expressed on the block fields the board carries.
func FillBlockLessDesc(atA, atB time.Time, odoA, odoB *int, cardA, cardB, keyA, keyB string) bool {
	a, b := atA.UTC(), atB.UTC()
	if !a.Equal(b) {
		return a.After(b)
	}
	if oa, ob := odoOrMissing(odoA), odoOrMissing(odoB); oa != ob {
		return oa > ob
	}
	if cardA != cardB {
		return cardA < cardB
	}
	return keyA < keyB
}

// odoOrMissing maps a missing odometer below any real reading so it sorts last
// in a newest-first list (SQL side: COALESCE(odometer,-1)).
func odoOrMissing(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}
