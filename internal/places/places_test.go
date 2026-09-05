package places

import "testing"

func TestSiteKeyFromDetailsAddress(t *testing.T) {
	got := SiteKey("WAWA", "2977 ROUTE 611, TANNERSVILLE, PA")
	want := "WAWA|2977 ROUTE 611|TANNERSVILLE|PA"
	if got != want {
		t.Fatalf("site key %q want %q", got, want)
	}
}

func TestLookupPrefersStoredLabel(t *testing.T) {
	cat := NewCatalog([]Place{{
		GeneralCode:  "A000001",
		TypeCode:     "001",
		BrandCode:    "WAWAA",
		TopTier:      "A",
		TopTierGrade: "A",
		Label:        "A000001_001_WAWAA_A_A",
		Name:         "WAWA",
		Address:      "423 RT 42",
		City:         "TURNERSVILLE",
		State:        "NJ",
		SiteKey:      "WAWA|423 RT 42|TURNERSVILLE|NJ",
		Active:       true,
	}})
	hit := cat.Lookup("wawa", "423 RT 42, Turnersville, NJ")
	if !hit.Matched || hit.Label != "A000001_001_WAWAA_A_A" || hit.GeneralCode != "A000001" {
		t.Fatalf("%+v", hit)
	}
}

func TestLookupBuildsLabelFromPartsWhenEmpty(t *testing.T) {
	cat := NewCatalog([]Place{{
		GeneralCode:  "A000002",
		TypeCode:     "001",
		BrandCode:    "RUTTR",
		TopTier:      "A",
		TopTierGrade: "A",
		Name:         "RUTTERS",
		Address:      "15475 KUTZTOWN RD",
		City:         "KUTZTOWN",
		State:        "PA",
		SiteKey:      "RUTTERS|15475 KUTZTOWN RD|KUTZTOWN|PA",
	}})
	hit := cat.Lookup("RUTTERS", "15475 KUTZTOWN RD, KUTZTOWN, PA")
	if !hit.Matched || hit.Label != "A000002_001_RUTTR_A_A" {
		t.Fatalf("%+v", hit)
	}
}

func TestLookupUnmatchedNeverInventsCanon(t *testing.T) {
	cat := NewCatalog(nil)
	cases := []struct{ name, addr string }{
		{"TRACKER", "1 MAIN, TOWN, VA"},
		{"WAWA", "1 MAIN"},
		{"WAWA", ""},
		{"UNKNOWN", "246 MAIN ST, BUZZARDS BAY, MA"},
		{"WAWA", "99999 NEVER ROAD, NOWHERE, ZZ"},
	}
	for _, c := range cases {
		hit := cat.Lookup(c.name, c.addr)
		if hit.Matched || hit.Label != UnmatchedLabel || hit.GeneralCode != "" {
			t.Fatalf("%s / %s => %+v", c.name, c.addr, hit)
		}
	}
}
