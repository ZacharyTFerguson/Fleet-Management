package places

import "testing"

func TestNextGeneralCodeAndLabel(t *testing.T) {
	n, err := NextGeneralCode("")
	if err != nil || n != "A000001" {
		t.Fatalf("first %s %v", n, err)
	}
	n, err = NextGeneralCode("A000001")
	if err != nil || n != "A000002" {
		t.Fatalf("next %s %v", n, err)
	}
	p, err := NewGasPlace("A000001", "SHELL #123", "1 Main St", "m1", "gas_stations")
	if err != nil {
		t.Fatal(err)
	}
	if p.Label != "A000001_001_SHELL_A_A" || p.TypeCode != TypeGas || p.BrandCode != "SHELL" {
		t.Fatalf("%+v", p)
	}
	if err := GasOnly("002"); err == nil {
		t.Fatal("shop type must be refused")
	}
}

func TestParseCanonLabelGasOnly(t *testing.T) {
	g, typ, brand, top, grade, ok := ParseCanonLabel("A000001_001_SHELL_A_A")
	if !ok || g != "A000001" || typ != TypeGas || brand != "SHELL" || top != "A" || grade != "A" {
		t.Fatalf("%s %s %s %s %s %v", g, typ, brand, top, grade, ok)
	}
	if _, _, _, _, _, ok := ParseCanonLabel("A000001_002_ACME_A_A"); ok {
		t.Fatal("type 002 must fail")
	}
	if _, _, _, _, _, ok := ParseCanonLabel("SHELL"); ok {
		t.Fatal("bare name must fail")
	}
}

func TestIsGasMerchantExcludesShops(t *testing.T) {
	if !IsGasMerchant("SHELL", "100 Main") {
		t.Fatal("shell")
	}
	if IsGasMerchant("JOE'S LUBE SHOP", "100 Main") {
		t.Fatal("shop leaked in")
	}
	if IsGasMerchant("PDI OFFICE", "HQ") {
		t.Fatal("office")
	}
	if IsGasMerchant("TRACKER", "") {
		t.Fatal("tracker")
	}
}
