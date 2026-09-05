package places

import (
	"context"
	"fmt"
	"strings"
)

// Store is the sqlite (or pgx) cache surface used by serve and places-cache.
type Store interface {
	ListPlaces(ctx context.Context) ([]Place, error)
	CountPlaces(ctx context.Context) (int, error)
	ReplacePlaces(ctx context.Context, rows []Place) error
}

// Loaded is a catalog plus where the rows came from.
type Loaded struct {
	Catalog *Catalog
	Count   int
	Source  string // sqlite | json | neon
}

// Load reads the local cache. It does not talk to Neon or invent codes.
func Load(ctx context.Context, st Store) (Loaded, error) {
	rows, err := st.ListPlaces(ctx)
	if err != nil {
		return Loaded{Catalog: NewCatalog(nil)}, err
	}
	return Loaded{Catalog: NewCatalog(rows), Count: len(rows), Source: "sqlite"}, nil
}

// Refresh copies existing catalog rows into sqlite from JSON and/or Neon.
// Source priority: --json path, then Neon URL. Never mints general_codes.
func Refresh(ctx context.Context, st Store, jsonPath, neonURL string) (Loaded, error) {
	var rows []Place
	src := ""
	jsonPath = strings.TrimSpace(jsonPath)
	neonURL = strings.TrimSpace(neonURL)
	switch {
	case jsonPath != "":
		got, err := FromJSONFile(jsonPath)
		if err != nil {
			return Loaded{Catalog: NewCatalog(nil)}, err
		}
		rows, src = got, "json"
	case neonURL != "":
		got, err := CopyFromNeon(ctx, neonURL)
		if err != nil {
			return Loaded{Catalog: NewCatalog(nil)}, err
		}
		rows, src = got, "neon"
	default:
		return Loaded{Catalog: NewCatalog(nil)}, fmt.Errorf("places: need --json PATH or Neon DATABASE_URL")
	}
	if err := st.ReplacePlaces(ctx, rows); err != nil {
		return Loaded{Catalog: NewCatalog(nil)}, err
	}
	return Loaded{Catalog: NewCatalog(rows), Count: len(rows), Source: src}, nil
}

// Ensure returns the sqlite cache, refreshing from JSON/Neon only when empty.
func Ensure(ctx context.Context, st Store, jsonPath, neonURL string) (Loaded, error) {
	got, err := Load(ctx, st)
	if err != nil {
		return got, err
	}
	if got.Count > 0 {
		return got, nil
	}
	if strings.TrimSpace(jsonPath) == "" && strings.TrimSpace(neonURL) == "" {
		return got, nil
	}
	return Refresh(ctx, st, jsonPath, neonURL)
}
