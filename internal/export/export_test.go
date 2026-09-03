package export

import (
	"bytes"
	"strings"
	"testing"

	"oilchange/internal/model"
)

func TestNeverWritesRemainingDue(t *testing.T) {
	if HasForbiddenHeader(Headers) {
		t.Fatal("forbidden in default headers")
	}
	var buf bytes.Buffer
	hold := "UNUSUAL_Y"
	miles := 100000
	cars := []model.Car{{
		EFleetsID:        "27TESTA",
		Nickname:         "VA19",
		PDIID:            "PDI-0001",
		LastOilMiles:     &miles,
		LastReadingMiles: &miles,
		HoldReason:       &hold,
	}}
	if err := WriteCSV(&buf, cars, 0, 5000); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, f := range forbidden {
		if strings.Contains(s, f) {
			t.Fatalf("emitted %q", f)
		}
	}
	if strings.Contains(s, "100000") && strings.Contains(s, "UNUSUAL_Y") {
		// last reading must be blanked on HOLD
		lines := strings.Split(strings.TrimSpace(s), "\n")
		if len(lines) < 2 {
			t.Fatal(s)
		}
		if strings.Count(lines[1], "100000") != 1 {
			t.Fatalf("stale last reading on HOLD: %s", lines[1])
		}
	}
}
