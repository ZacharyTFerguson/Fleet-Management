package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"oilchange/internal/config"
	"oilchange/internal/store"
	"oilchange/internal/vault"
)

func testApp(t *testing.T) *App {
	t.Helper()
	st, err := store.Open("sqlite", filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := vault.Open(filepath.Join(t.TempDir(), "vault.key"))
	if err != nil {
		t.Fatal(err)
	}
	return &App{Cfg: config.Config{SQLitePath: "t"}, Store: st, Vault: box}
}

func TestPutSecretNeverEchoesPlaintext(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	view, err := a.PutSecret(ctx, "onestep_api_key", "super-secret-key-value")
	if err != nil {
		t.Fatal(err)
	}
	if view.Mask == "super-secret-key-value" || strings.Contains(view.Mask, "super-secret") {
		t.Fatalf("echoed %q", view.Mask)
	}
	if !view.Set || view.Bytes != len("super-secret-key-value") {
		t.Fatalf("%+v", view)
	}
	list, err := a.ListSecrets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range list {
		if f.Key == "onestep_api_key" {
			if strings.Contains(f.Mask, "super-secret-key") {
				t.Fatal("list echoed")
			}
		}
	}
}

func TestLoginBootstrapAndReject(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	u, boot, err := a.Login(ctx, "zach", "hunter2-long")
	if err != nil || !boot || u != "zach" {
		t.Fatalf("%s %v %v", u, boot, err)
	}
	if _, _, err := a.Login(ctx, "zach", "wrong-password"); err == nil {
		t.Fatal("bad password accepted")
	}
	if _, boot, err := a.Login(ctx, "zach", "hunter2-long"); err != nil || boot {
		t.Fatalf("second login %v %v", boot, err)
	}
}
