package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestReplaceErasPersistsPersonAndSplitCarHistory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "eras.sqlite")
	s, err := Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	from := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	rows := []model.CardEra{
		{
			CardID: "CARD-MIX", EFleetsID: "27VA15", Nickname: "VA15",
			HolderType: "car", HolderKey: "27VA15",
			From: from, To: from.Add(24 * time.Hour), EvidenceN: 3,
			Stations: []string{"PUMP01", "PUMP02"}, Split: true, Rung: 3,
		},
		{
			CardID: "CARD-TYLER", HolderType: "person", HolderKey: "TYLER SPARE",
			Nickname: "TYLER SPARE", From: from, To: from.Add(2 * time.Hour),
			EvidenceN: 2, Stations: []string{"SHELL"},
		},
	}
	if err := s.ReplaceEras(ctx, rows); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListEras(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].CardID != "CARD-MIX" || !got[0].Split || got[0].HolderType != "car" || got[0].Rung != 3 {
		t.Fatalf("car era %+v", got[0])
	}
	if got[1].HolderType != "person" || got[1].EFleetsID != "" {
		t.Fatalf("person era must not invent a car: %+v", got[1])
	}
	if err := s.ReplaceEras(ctx, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListEras(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("replace empty: %d %v", len(got), err)
	}
}
