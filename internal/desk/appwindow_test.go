package desk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppWindowCmdUsesFirstBrowserOnPATH(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "chromium")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	cmd := AppWindowCmd("http://127.0.0.1:4739/history/", []string{"chromium"})
	if cmd == nil {
		t.Fatal("expected command")
	}
	if !strings.HasSuffix(cmd.Path, "chromium") {
		t.Fatalf("path %s", cmd.Path)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "--app=http://127.0.0.1:4739/history/" {
		t.Fatalf("args %+v", cmd.Args)
	}
}

func TestAppWindowCmdMissingBrowser(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if cmd := AppWindowCmd("http://127.0.0.1:4739/", []string{"definitely-not-a-browser"}); cmd != nil {
		t.Fatalf("got %+v", cmd)
	}
}
