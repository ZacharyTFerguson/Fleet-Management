package main

import (
	"testing"
	"time"
)

func TestProbeWindowsHours(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	w, err := probeWindows("6,24", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 2 {
		t.Fatalf("%d", len(w))
	}
	if w[0].to != now || now.Sub(w[0].from) != 6*time.Hour {
		t.Fatalf("6h window %+v", w[0])
	}
	if now.Sub(w[1].from) != 24*time.Hour {
		t.Fatalf("24h window %+v", w[1])
	}
}

func TestProbeWindowsExplicitFrom(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	w, err := probeWindows("48", "2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z", now)
	if err != nil || len(w) != 1 {
		t.Fatalf("%+v %v", w, err)
	}
	if w[0].from.UTC().Day() != 1 || w[0].to.UTC().Day() != 2 {
		t.Fatalf("%+v", w[0])
	}
}
