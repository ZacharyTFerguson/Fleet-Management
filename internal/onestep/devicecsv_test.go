package onestep

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"oilchange/internal/model"
)

func TestWriteDevicesCSVDisplayNameIsLabelOnly(t *testing.T) {
	a := "27TESTA"
	devs := []model.OneStepDevice{
		{FactoryID: "SAME_A", DeviceID: "DA", DisplayName: "Shared Label", LinkedCarEFleetsID: &a, Active: true},
		{FactoryID: "SAME_B", DeviceID: "DB", DisplayName: "Shared Label", Active: true},
		{FactoryID: "DEAD1", DeviceID: "DX", DisplayName: "Old", Dead: true, Active: false},
	}
	var buf bytes.Buffer
	if err := WriteDevicesCSV(&buf, devs); err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(buf.String()))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("header + 3 devices, got %d\n%s", len(rows), buf.String())
	}
	want := strings.Join(DeviceCSVHeaders, ",")
	if got := strings.Join(rows[0], ","); got != want {
		t.Fatalf("header %q want %q", got, want)
	}
	byFact := map[string][]string{}
	for _, row := range rows[1:] {
		byFact[row[0]] = row
	}
	if byFact["SAME_A"][2] != "Shared Label" || byFact["SAME_B"][2] != "Shared Label" {
		t.Fatalf("display_name is a label, both rows keep it: %+v", byFact)
	}
	if byFact["SAME_A"][3] != "27TESTA" {
		t.Fatalf("SAME_A links by factory_id: %v", byFact["SAME_A"])
	}
	if byFact["SAME_B"][3] != "" {
		t.Fatalf("same display_name must not copy the car link: %v", byFact["SAME_B"])
	}
	if byFact["SAME_A"][4] != "active" || byFact["DEAD1"][4] != "retired" {
		t.Fatalf("status %+v", byFact)
	}
}
