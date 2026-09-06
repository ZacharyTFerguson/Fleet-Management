package onestep

import (
	"context"
	"strings"
	"testing"
)

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

func TestParsePlaceListNestedDetail(t *testing.T) {
	raw := `{"result_list":[{"zone_id":"6ldgUl0NN2euKF81f07-1k","display_name":"A000001_001_SHELL_A_A","zone_type":"polygon","zone_group_id_list":["other","6ldgMVSEPkDtz-81f07-1k"],"shape_data":{"vertices":[38.1,-77.2,38.2,-77.1]},"detail":{"lat_lng":{"lat":38.2,"lng":-77.1},"custom_fields":{"address":{"value":"2 Pike"}}}}]}`
	items, _ := parsePlaceList([]byte(raw))
	if len(items) != 1 || items[0].ID != "6ldgUl0NN2euKF81f07-1k" || items[0].Address != "2 Pike" {
		t.Fatalf("%+v", items)
	}
	if items[0].Lat == nil || *items[0].Lat != 38.2 || items[0].Lng == nil || *items[0].Lng != -77.1 {
		t.Fatalf("latlng %+v", items[0])
	}
	if items[0].Kind != "polygon" || len(items[0].Vertices) != 4 {
		t.Fatalf("polygon %+v", items[0])
	}
	want := map[string]struct{}{"nope": {}}
	if !keepGasStationZone(items[0], "6ldgMVSEPkDtz-81f07-1k", want) {
		t.Fatal("must keep when zone_group_id_list contains Gas_Stations id")
	}
	if keepGasStationZone(items[0], "missing", want) {
		t.Fatal("must not keep a shop/other group")
	}
}

func TestPickGasGroup(t *testing.T) {
	id, zids, err := pickGasGroup([]map[string]any{
		{"display_name": "Shops", "id": "nope", "zone_id_list": []any{"x"}},
		{"display_name": "Gas_Stations", "zone_group_id": "g1", "zone_id_list": []any{"a", "b"}},
	})
	if err != nil || id != "g1" || len(zids) != 2 {
		t.Fatalf("%s %v %v", id, zids, err)
	}
}

func TestCreateMarkerPortalFirst(t *testing.T) {
	t.Setenv("ONESTEP_WRITE_PROVEN", "")
	c := &Client{Token: "x"}
	p := MarkerPayload{Name: "A000001_001_SHELL_A_A", Lat: 1, Lng: 2, Group: "Gas_Stations", Type: "gas"}
	if _, _, _, err := c.CreateMarker(context.Background(), p); err == nil || !strings.Contains(err.Error(), "portal") {
		t.Fatalf("want portal-first: %v", err)
	}
	if _, _, err := c.CreateZoneNear(context.Background(), p.Name, 1, 2, 25); err == nil || !strings.Contains(err.Error(), "portal") {
		t.Fatalf("zone want portal-first: %v", err)
	}
}
