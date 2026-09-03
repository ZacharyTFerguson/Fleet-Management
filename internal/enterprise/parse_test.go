package enterprise

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"oilchange/internal/oil"
)

func testdata(t *testing.T, elem ...string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	return filepath.Join(append([]string{root}, elem...)...)
}

func TestParseDetailsByHeaderNotColumnLetter(t *testing.T) {
	f, err := os.Open(testdata(t, "enterprise", "details.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fills, stations, cards, err := ParseFills(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 3 {
		t.Fatalf("fills %d", len(fills))
	}
	if fills[0].EFleetsID != "27TESTA" || fills[0].Odometer == nil || *fills[0].Odometer != 100000 {
		t.Fatalf("first fill %+v", fills[0])
	}
	if fills[2].UnusualY != true {
		t.Fatal("unusual Y")
	}
	if len(stations) == 0 {
		t.Fatal("gas stations")
	}
	if len(cards) == 0 {
		t.Fatal("cards")
	}
}

func TestParseShopROOilSeedAndNotOilPan(t *testing.T) {
	f, err := os.Open(testdata(t, "enterprise", "maintenance.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ros, locs, oils, err := ParseShopROs(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(ros) != 2 {
		t.Fatalf("ros collapsed by RO ID, got %d", len(ros))
	}
	if len(locs) != 2 {
		t.Fatalf("locs %d", len(locs))
	}
	if len(oils) != 1 {
		t.Fatalf("oil changes %d (oil pan must not seed)", len(oils))
	}
	if oils[0].Miles != 100500 {
		t.Fatalf("miles %d", oils[0].Miles)
	}
	if !oil.IsOilChangeService("Full Synthetic Lube Oil Filter") {
		t.Fatal()
	}
}

func TestOpaquePDIID(t *testing.T) {
	f, err := os.Open(testdata(t, "enterprise", "fleetsummary.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cars, err := ParseVehicles(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(cars) != 2 {
		t.Fatalf("cars %d", len(cars))
	}
	if !strings.HasPrefix(cars[0].PDIID, "PDI-") {
		t.Fatalf("pdi %s", cars[0].PDIID)
	}
	if strings.Contains(cars[0].PDIID, "VA") {
		t.Fatalf("region leaked into pdi %s", cars[0].PDIID)
	}
	if cars[0].Region != "VA" || cars[0].Nickname != "VA19" {
		t.Fatalf("nick/region %+v", cars[0])
	}
	if cars[0].EFleetsID != "27TESTA" {
		t.Fatalf("join key %s", cars[0].EFleetsID)
	}
}

func TestParseWrongCardSynthetic(t *testing.T) {
	f, err := os.Open(testdata(t, "enterprise", "details_wrongcard.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fills, _, _, err := ParseFills(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 6 {
		t.Fatalf("fills %d", len(fills))
	}
	var mixOn19 int
	for _, fl := range fills {
		if fl.CardID == "CARD-MIX-99" && fl.EFleetsID == "27VA19" {
			mixOn19++
		}
	}
	if mixOn19 != 1 {
		t.Fatalf("want one CARD-MIX-99 swipe on 27VA19, got %d", mixOn19)
	}
}

func TestHeaderTrimCustName(t *testing.T) {
	idx := headerIndex([]string{"Cust Name ", "Vehicle"})
	if _, ok := idx["cust name"]; !ok {
		t.Fatal("trimmed Cust Name")
	}
}

func TestMissingHeadersRefused(t *testing.T) {
	_, _, err := readCSV(strings.NewReader("Foo,Bar\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = ParseFills(strings.NewReader("Foo,Bar\n1,2\n"))
	if err == nil {
		t.Fatal("should refuse")
	}
}
