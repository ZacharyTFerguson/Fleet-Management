package model

import (
	"testing"
	"time"
)

func TestSortCardTxsDescNewestFirstStableTies(t *testing.T) {
	at := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	txs := []CardTx{
		{CardID: "C-B", At: at, RecordedEFleetsID: "27VA15", Odometer: intPtr(100)},
		{CardID: "C-A", At: at, RecordedEFleetsID: "27VA15", Odometer: intPtr(100)},
		{CardID: "C-Z", At: at.Add(2 * time.Hour), RecordedEFleetsID: "27VA15"},
		{CardID: "C-Y", At: at.Add(-time.Hour), RecordedEFleetsID: "27VA15"},
	}
	SortCardTxsDesc(txs)
	if txs[0].CardID != "C-Z" || txs[1].CardID != "C-A" || txs[2].CardID != "C-B" || txs[3].CardID != "C-Y" {
		t.Fatalf("order %+v", txs)
	}

	// Re-sort must not scramble equal-time stable order.
	SortCardTxsDesc(txs)
	if txs[1].CardID != "C-A" || txs[2].CardID != "C-B" {
		t.Fatalf("stable tie re-sort moved rows: %+v", txs)
	}
}

func TestSortFillsDescNewestFirst(t *testing.T) {
	older := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	fills := []Fill{
		{EFleetsID: "27TESTA", ProviderTransactionTime: older, Odometer: intPtr(100000)},
		{EFleetsID: "27TESTA", ProviderTransactionTime: newer, Odometer: intPtr(100200)},
	}
	SortFillsDesc(fills)
	if !fills[0].ProviderTransactionTime.Equal(newer) {
		t.Fatalf("newest first: %+v", fills)
	}
}

func intPtr(n int) *int { return &n }
