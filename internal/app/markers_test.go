package app

import (
	"context"
	"testing"

	"oilchange/internal/model"
	"oilchange/internal/places"
)

func TestPullSkipsShopsAndMintsCanon(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	if err := a.Store.UpsertStation(ctx, model.GasStation{ID: "s1", Name: "SHELL 99", Address: "1 Main St"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertStation(ctx, model.GasStation{ID: "s2", Name: "JOE LUBE SHOP", Address: "2 Shop Rd"}); err != nil {
		t.Fatal(err)
	}
	list, err := a.PullGasCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if list.Places != 1 || len(list.Jobs) != 1 {
		t.Fatalf("%+v", list)
	}
	j := list.Jobs[0]
	if j.Draft.Group != places.GroupGas || !contains(j.Draft.Name, "_001_") {
		t.Fatalf("draft %+v", j.Draft)
	}
	if _, err := a.SendMarker(ctx, j.ID, "SEND_TO_ONESTEP", "nope"); err == nil {
		t.Fatal("send without approve")
	}
}

func TestSendRequiresConfirmPhrase(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	_ = a.Store.UpsertStation(ctx, model.GasStation{ID: "s1", Name: "WAWA", Address: "10 Pike"})
	list, err := a.PullGasCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := list.Jobs[0].ID
	if _, err := a.ReviewMarker(ctx, id, "approve", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendMarker(ctx, id, "please", "x"); err == nil {
		t.Fatal("phrase skipped")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()))
}
