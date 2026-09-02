package oil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestGoldenJSONLastReading(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures", "lastreading_golden.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		EnterpriseOdo int     `json:"enterprise_odo"`
		MilesSince    float64 `json:"miles_since"`
		Reading       int     `json:"reading"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	got, holds, err := LastReading(g.EnterpriseOdo, time.Time{}, g.MilesSince)
	if err != nil || len(holds) != 0 {
		t.Fatalf("%v %v", err, holds)
	}
	if got != g.Reading {
		t.Fatalf("got %d want %d", got, g.Reading)
	}
}
