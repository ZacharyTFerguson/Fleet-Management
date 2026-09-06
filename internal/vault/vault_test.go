package vault

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	box, err := Open(filepath.Join(t.TempDir(), "vault.key"))
	if err != nil {
		t.Fatal(err)
	}
	nonce, ct, err := box.Encrypt("super-secret-pem")
	if err != nil {
		t.Fatal(err)
	}
	got, err := box.Decrypt(nonce, ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != "super-secret-pem" {
		t.Fatalf("round trip %q", got)
	}
	if strings.Contains(string(ct), "super") {
		t.Fatal("ciphertext leaked plaintext")
	}
}

func TestMaskNeverEchoesFullSecret(t *testing.T) {
	if Mask("") != "" {
		t.Fatal("empty")
	}
	if Mask("ab") != "••••" {
		t.Fatalf("short %q", Mask("ab"))
	}
	m := Mask("onestep-api-key-xyz9")
	if m != "••••xyz9" {
		t.Fatalf("mask %q", m)
	}
	if strings.Contains(m, "onestep-api-key") {
		t.Fatal("mask leaked prefix")
	}
}

func TestKnownCatalog(t *testing.T) {
	if !Known("onestep_api_key") || Known("not_a_secret") {
		t.Fatal("catalog")
	}
}
