package onestep

import "testing"

func TestMarkerPayloadGasOnly(t *testing.T) {
	ok := MarkerPayload{Name: "A000001_001_SHELL_A_A", Lat: 38.1, Lng: -77.2, Group: "Gas_Stations", Type: "gas"}
	if err := ok.ValidateGas(); err != nil {
		t.Fatal(err)
	}
	shop := ok
	shop.Group = "Shops"
	if err := shop.ValidateGas(); err == nil {
		t.Fatal("shop group must fail")
	}
	badType := ok
	badType.Name = "A000001_002_ACME__A_A"
	if err := badType.ValidateGas(); err == nil {
		t.Fatal("type 002 must fail")
	}
}

func TestParsePlaceList(t *testing.T) {
	items, keys := parsePlaceList([]byte(`{"result_list":[{"id":"z1","name":"A000001_001_SHELL_A_A","lat":1.5,"lng":-2.5}]}`))
	if len(items) != 1 || items[0].ID != "z1" || items[0].Lat == nil || *items[0].Lat != 1.5 {
		t.Fatalf("%+v keys=%s", items, keys)
	}
	if FindByName(items, "a000001_001_shell_a_a") == nil {
		t.Fatal("name match")
	}
}
