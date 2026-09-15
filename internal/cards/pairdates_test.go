package cards

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestApplyPairDatesSplitCardMix99(t *testing.T) {
	va15Start := ny(2026, 8, 1, 10)
	va19Start := ny(2026, 8, 11, 10)
	eras := []model.CardEra{
		{
			CardID: "CARD-MIX-99", EFleetsID: "27VA15", HolderType: HolderCar, HolderKey: "27VA15",
			From: va15Start.Add(-48 * time.Hour), To: va15Start, PairStarted: va15Start.Add(-48 * time.Hour),
			Split: true,
		},
		{
			CardID: "CARD-MIX-99", EFleetsID: "27VA19", HolderType: HolderCar, HolderKey: "27VA19",
			From: va19Start, To: va19Start.Add(24 * time.Hour), PairStarted: va19Start,
			Split: true,
		},
	}
	got := ApplyPairDates(eras)
	if got[0].SwitchedAt == nil || !got[0].SwitchedAt.Equal(va19Start) {
		t.Fatalf("VA15 switched %+v want %v", got[0].SwitchedAt, va19Start)
	}
	if got[0].NextPairAt == nil || !got[0].NextPairAt.Equal(va19Start) {
		t.Fatalf("VA15 next_pair %+v want %v", got[0].NextPairAt, va19Start)
	}
	if got[1].SwitchedAt != nil || got[1].NextPairAt != nil {
		t.Fatalf("current VA19 era must not switch: %+v", got[1])
	}
}

func TestBackpropSplitSetsPairDatesFromAnchors(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	v15, t15 := exclusiveSits("27VA15", "CARD-MIX-99", 3, day, 37.54, -77.43)
	v19, t19 := exclusiveSits("27VA19", "CARD-MIX-99", 3, day.Add(10*24*time.Hour), 38.85, -77.05)
	early := model.CardTx{
		CardID: "CARD-MIX-99", At: day.Add(-48 * time.Hour),
		StationName: "WAWA", StationAddress: "9 OLD RD, TOWN, VA",
		RecordedEFleetsID: "27VA15",
	}
	txs := append(append(t15, t19...), early)
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	got := ClimbStationLadder(append(v15, v19...), txs, fleet, nil, DefaultStopSlack, DefaultLadderRungs)
	var va15Era, va19Era *model.CardEra
	for i := range got.Eras {
		e := &got.Eras[i]
		if e.CardID != "CARD-MIX-99" || eraHolderType(*e) != HolderCar {
			continue
		}
		switch e.EFleetsID {
		case "27VA15":
			va15Era = e
		case "27VA19":
			va19Era = e
		}
	}
	if va15Era == nil || va19Era == nil {
		t.Fatalf("eras %+v", got.Eras)
	}
	if !va15Era.PairStarted.Equal(early.At) {
		t.Fatalf("pair_started %v want %v", va15Era.PairStarted, early.At)
	}
	if va15Era.SwitchedAt == nil || !va15Era.SwitchedAt.Equal(t19[0].At) {
		t.Fatalf("switched %+v want %v", va15Era.SwitchedAt, t19[0].At)
	}
	if va19Era.SwitchedAt != nil {
		t.Fatalf("current era switched must be nil: %+v", va19Era.SwitchedAt)
	}
}
