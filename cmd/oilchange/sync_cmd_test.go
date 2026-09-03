package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"oilchange/internal/config"
	"oilchange/internal/model"
)

func TestSyncCommandWritesLocalMirror(t *testing.T) {
	dir := t.TempDir()
	mirror := filepath.Join(dir, "cars.json")
	code := cmdSyncSupabase(context.Background(), config.Config{
		SQLitePath: filepath.Join(dir, "oil.sqlite"),
	}, []string{"--mirror", mirror})
	if code != model.ExitOK {
		t.Fatalf("sync exit %d", code)
	}
	if _, err := os.Stat(mirror); err != nil {
		t.Fatalf("sync mirror: %v", err)
	}
}
