package cards

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSnapshotJSONEmptySlicesAreArraysNotNull(t *testing.T) {
	snap := BuildSnapshotFull(nil, nil, nil, nil, nil, nil, time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC))
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"unknown", "stations", "cars_without_card", "gps_best"} {
		v, ok := raw[k]
		if !ok {
			t.Errorf("%s missing", k)
			continue
		}
		if v == nil {
			t.Errorf("%s marshaled as null (Card Desk crashes on .length)", k)
		}
		if _, isArr := v.([]any); !isArr {
			t.Errorf("%s want array, got %T", k, v)
		}
	}
}
