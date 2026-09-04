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
