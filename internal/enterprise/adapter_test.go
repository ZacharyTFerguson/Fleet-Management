package enterprise

import (
	"strings"
	"testing"

	"oilchange/internal/config"
)

func TestNewHTTPAdapterRequiresSecretsOffChat(t *testing.T) {
	_, err := NewHTTPAdapter("", "", "", "583424")
	if err == nil {
		t.Fatal("expected missing username/password")
	}
	if !strings.Contains(err.Error(), config.EFleetsSecretsHint) {
		t.Fatalf("hint missing: %v", err)
	}
	_, err = NewHTTPAdapter("", "user", "pass", "")
	if err == nil {
		t.Fatal("expected missing cust")
	}
	if !strings.Contains(err.Error(), "EFLEETS_CUST_NUM") {
		t.Fatalf("cust: %v", err)
	}
}
