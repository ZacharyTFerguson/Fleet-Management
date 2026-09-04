package cards

import (
	"testing"
	"time"

	"oilchange/internal/model"
)

func TestBackpropExtendsCardHistoryAcrossEarlierSwipes(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	v15, t15 := exclusiveSits("27VA15", "CARD-LOCK", 3, day, 37.54, -77.43)
	// Earlier swipe at a fourth station before the GPS anchor window — should inherit VA15.
	early := model.CardTx{
		CardID: "CARD-LOCK", At: day.Add(-72 * time.Hour),
		StationName: "EARLY", StationAddress: "9 MAIN ST, TOWN, VA",
		RecordedEFleetsID: "WRONG",
	}
	txs := append([]model.CardTx{early}, t15...)
	got := ClimbStationLadder(v15, txs, []model.Car{{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"}}, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Cars) != 1 || got.Cars[0].HolderKey != "27VA15" {
		t.Fatalf("ladder cars %+v", got.Cars)
	}
	foundBackprop := false
	for _, c := range got.GPS.Calls {
		if c.CardID == "CARD-LOCK" && c.At.Equal(early.At) && c.CalledCar == "27VA15" && c.Why == "backprop" {
			foundBackprop = true
		}
	}
	if !foundBackprop {
		t.Fatalf("want backprop call on early swipe, got %+v", got.GPS.Calls)
	}
}

// TestBackpropSplitDoesNotPaintLaterCarOverEarlierEra: same card, later exclusive
// car B after exclusive car A → SPLIT era; B must not back-propagate over A's window.
func TestBackpropSplitDoesNotPaintLaterCarOverEarlierEra(t *testing.T) {
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
	if len(got.Cars) != 1 || !got.Cars[0].Split {
		t.Fatalf("want SPLIT card, got %+v", got.Cars)
	}
	var va15Era, va19Era *CardEra
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
		t.Fatalf("want VA15 and VA19 eras, got %+v", got.Eras)
	}
	if va19Era.From.Before(t19[0].At) {
		t.Fatalf("VA19 era must not paint over VA15 window: from=%v first VA19=%v", va19Era.From, t19[0].At)
	}
	var earlyCall *RecordCall
	for i := range got.GPS.Calls {
		c := &got.GPS.Calls[i]
		if c.At.Equal(early.At) {
			earlyCall = c
			break
		}
	}
	if earlyCall == nil || earlyCall.CalledCar != "27VA15" || earlyCall.Why != "backprop" {
		t.Fatalf("early swipe must backprop VA15: %+v", earlyCall)
	}
	mid := t15[1].At
	for _, c := range got.GPS.Calls {
		if c.At.Equal(mid) && c.CalledCar == "27VA19" {
			t.Fatalf("mid VA15 window swipe must not be VA19: %+v", c)
		}
	}
}

// TestBackpropDriverKeptCardStaysOnPerson: driver kept the card → People;
// do not back-propagate a vehicle label onto earlier swipes.
func TestBackpropDriverKeptCardStaysOnPerson(t *testing.T) {
	day := ny(2026, 8, 1, 10)
	v15, t15 := exclusiveSits("27VA15", "CARD-DRV", 1, day, 37.54, -77.43)
	v19, t19 := exclusiveSits("27VA19", "CARD-DRV", 1, day.Add(48*time.Hour), 38.85, -77.05)
	t15[0].DriverFirst, t15[0].DriverLast = "PAT", "TECH"
	t19[0].DriverFirst, t19[0].DriverLast = "PAT", "TECH"
	early := model.CardTx{
		CardID: "CARD-DRV", At: day.Add(-24 * time.Hour),
		StationName: "WAWA", StationAddress: "1 MAIN, TOWN, VA",
		DriverFirst: "PAT", DriverLast: "TECH",
		RecordedEFleetsID: "27VA15",
	}
	txs := append(append(t15, t19...), early)
	fleet := []model.Car{
		{EFleetsID: "27VA15", Nickname: "VA15", Region: "VA"},
		{EFleetsID: "27VA19", Nickname: "VA19", Region: "VA"},
	}
	got := ClimbStationLadder(append(v15, v19...), txs, fleet, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.People) != 1 || got.People[0].HolderKey != "PAT TECH" {
		t.Fatalf("people %+v", got.People)
	}
	if len(got.Cars) != 0 {
		t.Fatalf("driver-kept must not lock as car: %+v", got.Cars)
	}
	for _, c := range got.GPS.Calls {
		if c.CardID == "CARD-DRV" && c.CalledCar != "" {
			t.Fatalf("driver-kept card must not get car backprop: %+v", c)
		}
	}
	for _, e := range got.Eras {
		if e.CardID == "CARD-DRV" && eraHolderType(e) == HolderCar {
			t.Fatalf("driver-kept must not persist car era: %+v", e)
		}
	}
}

// TestBackpropOfficeCardDoesNotBackpropToCar: office cards stay office;
// do not back-propagate a vehicle label.
func TestBackpropOfficeCardDoesNotBackpropToCar(t *testing.T) {
	at := ny(2026, 8, 12, 10)
	visits := []model.StopVisit{{
		EFleetsID: "27VA15", HasPos: true, Lat: 37.54, Lng: -77.43,
		From: at, To: at.Add(10 * time.Minute),
	}}
	txs := []model.CardTx{
		{
			CardID: "CARD-OFF", At: at.Add(-72 * time.Hour),
			StationName: "WAWA", StationAddress: "1 MAIN, TOWN, VA",
			RecordedEFleetsID: "PDI OFFICE", RecordedCVN: "HQ",
		},
		{
			CardID: "CARD-OFF", At: at.Add(2 * time.Minute),
			StationName: "WAWA", StationAddress: "1 MAIN, TOWN, VA",
			RecordedEFleetsID: "PDI OFFICE", RecordedCVN: "HQ",
		},
	}
	got := ClimbStationLadder(visits, txs, []model.Car{{EFleetsID: "27VA15", Nickname: "VA15"}}, nil, DefaultStopSlack, DefaultLadderRungs)
	if len(got.Offices) != 1 || got.Offices[0].CardID != "CARD-OFF" {
		t.Fatalf("offices %+v", got.Offices)
	}
	if len(got.Cars) != 0 {
		t.Fatalf("office card is not a car: %+v", got.Cars)
	}
	for _, c := range got.GPS.Calls {
		if c.CardID == "CARD-OFF" && c.CalledCar != "" {
			t.Fatalf("office card must not get car backprop: %+v", c)
		}
	}
	for _, e := range got.Eras {
		if e.CardID == "CARD-OFF" && eraHolderType(e) == HolderCar {
			t.Fatalf("office must not persist car era: %+v", e)
		}
	}
}
