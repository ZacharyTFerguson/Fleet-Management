package cards

import "testing"

func TestMapStationsCountsCarsAndCards(t *testing.T) {
	st := MapStations(syntheticWrongCard())
	if len(st) != 2 {
		t.Fatalf("stations %d", len(st))
	}
	if st[0].Name != "SHELL" || st[0].Swipes < 5 {
		t.Fatalf("top station %+v", st[0])
	}
	if st[0].Cars < 2 || st[0].Cards < 2 {
		t.Fatalf("SHELL should map mixed cars/cards: %+v", st[0])
	}
}

func TestUnknownMatchupsStartsWithSuspectAndSingleton(t *testing.T) {
	txs := syntheticWrongCard()
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	u := UnknownMatchups(txs, ps)
	kinds := map[string]string{}
	for _, m := range u {
		kinds[m.CardID] = m.Kind
	}
	if kinds["CARD-MIX-99"] != "suspect" {
		t.Fatalf("CARD-MIX-99 kind %q %+v", kinds["CARD-MIX-99"], u)
	}
	if kinds["CARD-19"] != "singleton" {
		t.Fatalf("CARD-19 should be a one-swipe unknown, got %q", kinds["CARD-19"])
	}
	var mix UnknownMatchup
	for _, m := range u {
		if m.CardID == "CARD-MIX-99" {
			mix = m
		}
	}
	found := false
	for _, n := range mix.Neighbors {
		if n.EFleetsID == "27VA15" && n.CardID == "CARD-15" {
			found = true
		}
	}
	if !found {
		t.Fatalf("suspect should carry SHELL neighbor 27VA15/CARD-15: %+v", mix.Neighbors)
	}
}

func TestCarsWithoutBestCard(t *testing.T) {
	txs := syntheticWrongCard()
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	missing := CarsWithoutBestCard(txs, ps)
	// 27VA19 is on the mixed swipe and CARD-19, so it may have a BEST (CARD-19).
	// 27VA15 is BEST for CARD-MIX-99 and CARD-15.
	for _, id := range missing {
		if id == "27VA15" {
			t.Fatalf("27VA15 is BEST for CARD-MIX-99, should not be missing")
		}
	}
}
