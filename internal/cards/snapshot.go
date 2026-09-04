package cards

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"oilchange/internal/model"
)

// Snapshot is the Cards desk payload. Never includes Last Reading math.
type Snapshot struct {
	SyncedAt         time.Time         `json:"synced_at"`
	Source           string            `json:"source"`
	Stats            SnapshotStats     `json:"stats"`
	Unknown          []UnknownMatchup     `json:"unknown"`
	Stations         []StationSummary     `json:"stations"`
	CarsWithoutCard  []string             `json:"cars_without_card"`
	GPSBest          []model.GPSCardMatch `json:"gps_best"`
	Nicknames        map[string]string    `json:"nicknames,omitempty"`
}

// SnapshotStats is the header strip.
type SnapshotStats struct {
	Cards           int `json:"cards"`
	Stations        int `json:"stations"`
	Unknown         int `json:"unknown"`
	Suspects        int `json:"suspects"`
	Ambiguous       int `json:"ambiguous"`
	Singletons      int `json:"singletons"`
	CarsWithoutCard int `json:"cars_without_card"`
	Swipes          int `json:"swipes"`
	GPSBest         int `json:"gps_best"`
	GPSMatches      int `json:"gps_matches"`
}

// BuildSnapshot scores pairings if needed and maps stations + unknowns.
func BuildSnapshot(txs []model.CardTx, pairings []model.CardPairing, gps []model.GPSCardMatch, nicknames map[string]string, now time.Time) Snapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if len(pairings) == 0 {
		pairings = ScorePairings(txs, now)
	}
	unknown := UnknownMatchups(txs, pairings)
	stations := MapStations(txs)
	missing := CarsWithoutBestCard(txs, pairings)
	cards := map[string]struct{}{}
	for _, t := range txs {
		if t.CardID != "" {
			cards[t.CardID] = struct{}{}
		}
	}
	gpsBestN, gpsHits := 0, 0
	for _, g := range gps {
		gpsHits += g.EvidenceN
		if g.Best {
			gpsBestN++
		}
	}
	stats := SnapshotStats{
		Cards:           len(cards),
		Stations:        len(stations),
		Unknown:         len(unknown),
		CarsWithoutCard: len(missing),
		Swipes:          len(txs),
		GPSBest:         gpsBestN,
		GPSMatches:      gpsHits,
	}
	for _, u := range unknown {
		switch u.Kind {
		case "suspect":
			stats.Suspects++
		case "ambiguous":
			stats.Ambiguous++
		case "singleton":
			stats.Singletons++
		}
	}
	return Snapshot{
		SyncedAt:        now,
		Source:          "card-swipes",
		Stats:           stats,
		Unknown:         unknown,
		Stations:        stations,
		CarsWithoutCard: missing,
		GPSBest:         gps,
		Nicknames:       nicknames,
	}
}

var writeMu sync.Mutex

// WriteSnapshot atomically replaces dest (Windows-safe rename).
func WriteSnapshot(path string, snap Snapshot) error {
	if path == "" {
		return errors.New("empty cards snapshot path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}
