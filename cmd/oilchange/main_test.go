package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oilchange/internal/model"
)

func chdirTemp(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func captureOutput(t *testing.T, stream **os.File, fn func()) string {
	t.Helper()
	old := *stream
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	*stream = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	defer func() {
		_ = w.Close()
		*stream = old
	}()
	fn()
	_ = w.Close()
	*stream = old
	return <-done
}

func TestRunNoArgsAndUnknownExitError(t *testing.T) {
	chdirTemp(t)
	var noArgs, unknown int
	errOut := captureOutput(t, &os.Stderr, func() {
		noArgs = run(nil)
		unknown = run([]string{"not-a-real-command"})
	})
	if noArgs != model.ExitError || unknown != model.ExitError {
		t.Fatalf("exits no-args=%d unknown=%d", noArgs, unknown)
	}
	if !strings.Contains(errOut, "oilchange") {
		t.Fatalf("usage missing from stderr")
	}
}

func TestEnvOmitsSecretValues(t *testing.T) {
	chdirTemp(t)
	t.Setenv("OILCHANGE_DB", filepath.Join(".", "t.sqlite"))
	t.Setenv("SUPABASE_URL", "https://hdtwfdjdvdzdxfdriyzn.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE", "super-secret-service-role-token")
	t.Setenv("SUPABASE_SYNC_SECRET", "super-secret-sync-token-value")
	t.Setenv("ONESTEP_API_TOKEN", "super-secret-onestep-token")
	t.Setenv("EFLEETS_PASSWORD", "super-secret-efleets-pass")

	var code int
	out := captureOutput(t, &os.Stdout, func() {
		code = run([]string{"env"})
	})
	if code != model.ExitOK {
		t.Fatalf("exit %d", code)
	}
	for _, secret := range []string{
		"super-secret-service-role-token",
		"super-secret-sync-token-value",
		"super-secret-onestep-token",
		"super-secret-efleets-pass",
	} {
		if strings.Contains(out, secret) {
			t.Fatal("env leaked a secret value")
		}
	}
}

func TestSyncRefusesXRAYWithoutLeakingSecret(t *testing.T) {
	chdirTemp(t)
	t.Setenv("OILCHANGE_DB", filepath.Join(".", "t.sqlite"))
	t.Setenv("SUPABASE_URL", "https://CHJQCZNYXVTJBAMTTQDJ.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE", "super-secret-service-role-token")
	t.Setenv("SUPABASE_SYNC_SECRET", "")

	var code int
	errOut := captureOutput(t, &os.Stderr, func() {
		code = run([]string{"sync", "--mirror", "cars.json"})
	})
	if code != model.ExitError {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(errOut, "super-secret-service-role-token") {
		t.Fatal("sync leaked service role")
	}
}
