package cards

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oilchange/internal/model"
)

// A roster car with no DETAILS swipe and no GPS vote (new unit, NO_DEVICE car)
// must still be visible on the Cards desk. nicknames is the full roster from
// ListCars; cars_without_card is the only place such a car can appear.
func TestSnapshotListsRosterCarWithNoSwipesInCarsWithoutCard(t *testing.T) {
	txs := []model.CardTx{
		{CardID: "CARD-15", At: ny(2026, 8, 20, 10), StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA", RecordedEFleetsID: "27VA15"},
		{CardID: "CARD-15", At: ny(2026, 8, 22, 10), StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA", RecordedEFleetsID: "27VA15"},
	}
	roster := map[string]string{
		"27VA15": "VA15",
		"27NEW1": "VA40", // on the roster, never swiped, no GPS box
	}
	snap := BuildSnapshotFull(txs, nil, nil, nil, nil, roster, ny(2026, 8, 26, 0))
	found := false
	for _, id := range snap.CarsWithoutCard {
		if id == "27NEW1" {
			found = true
		}
		if id == "27VA15" {
			t.Fatalf("27VA15 has a BEST card, must not be listed: %+v", snap.CarsWithoutCard)
		}
	}
	if !found {
		t.Fatalf("roster car 27NEW1 with no swipes dropped from cars_without_card: %+v", snap.CarsWithoutCard)
	}
	if snap.Stats.CarsWithoutCard != len(snap.CarsWithoutCard) {
		t.Fatalf("stats %d vs list %d", snap.Stats.CarsWithoutCard, len(snap.CarsWithoutCard))
	}
}

// A swipe recorded on UNKNOWN / N/A / - is not a car. ScorePairings and
// MatchGPSFirst already refuse it (isUnknownCar); the desk lists must not
// invent a vehicle called UNKNOWN from the same row.
func TestCarsWithoutBestCardDoesNotInventUnknownCar(t *testing.T) {
	txs := []model.CardTx{{
		CardID: "CARD-X", At: ny(2026, 8, 20, 10),
		StationName: "SHELL", StationAddress: "1 MAIN ST, TOWN, VA",
		RecordedEFleetsID: "UNKNOWN",
	}}
	ps := ScorePairings(txs, ny(2026, 8, 26, 0))
	for _, id := range CarsWithoutBestCard(txs, ps) {
		if isUnknownCar(id) {
			t.Fatalf("invented car %q in cars_without_card", id)
		}
	}
	st := MapStations(txs)
	if len(st) != 1 || st[0].Cars != 0 {
		t.Fatalf("UNKNOWN counted as a car at the pump: %+v", st)
	}
}

func manyVisits(n int) []model.StopVisit {
	out := make([]model.StopVisit, 0, n)
	base := ny(2026, 8, 1, 6)
	for i := 0; i < n; i++ {
		from := base.Add(time.Duration(i) * 7 * time.Minute)
		out = append(out, model.StopVisit{
			EFleetsID: fmt.Sprintf("27CAR%03d", i%150),
			FactoryID: fmt.Sprintf("%010d", 3271116000+i%150),
			DeviceID:  fmt.Sprintf("DEV%06d", i%150),
			From:      from,
			To:        from.Add(9 * time.Minute),
			Lat:       37.5 + float64(i%97)/1000,
			Lng:       -77.4 - float64(i%89)/1000,
			HasPos:    i%5 != 0,
		})
	}
	return out
}

// data/runtime/gps-stops.json is written by `cards rebuild` and read by
// `cards split` / `cards call` / `cards rebuild --no-gps`, possibly from another
// terminal. WriteSnapshot and writeMirror go through tmp+rename under a mutex;
// this cache does not. A reader during a write sees a truncated file and either
// errors or silently matches against fewer stop windows.
func TestStopVisitsCacheConcurrentSaveLoadNeverTorn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gps-stops.json")
	visits := manyVisits(40000)
	if err := SaveStopVisits(path, visits); err != nil {
		t.Fatal(err)
	}

	var writers, readers sync.WaitGroup
	stop := make(chan struct{})
	var torn, reads atomic.Int32
	var firstErr atomic.Value
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				got, err := LoadStopVisits(path)
				reads.Add(1)
				if err != nil {
					torn.Add(1)
					firstErr.CompareAndSwap(nil, err.Error())
					continue
				}
				if len(got) != len(visits) {
					torn.Add(1)
					firstErr.CompareAndSwap(nil, fmt.Sprintf("read %d visits, cache holds %d", len(got), len(visits)))
				}
			}
		}()
	}
	for w := 0; w < 4; w++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for i := 0; i < 12; i++ {
				if err := SaveStopVisits(path, visits); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	writers.Wait()
	close(stop)
	readers.Wait()

	if reads.Load() == 0 {
		t.Fatal("no reads overlapped the writes; test cannot judge")
	}
	if n := torn.Load(); n > 0 {
		t.Fatalf("%d of %d cache reads were torn (first: %v)", n, reads.Load(), firstErr.Load())
	}
	final, err := LoadStopVisits(path)
	if err != nil || len(final) != len(visits) {
		t.Fatalf("final cache err=%v n=%d want %d", err, len(final), len(visits))
	}
}

// cards.json is replaced under writeMu via tmp+rename; concurrent rebuilds must
// leave one parseable file and no .tmp-* leftovers.
func TestConcurrentWriteSnapshotNoTornOrLeakedTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cards.json")
	const n = 24
	start := make(chan struct{})
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			snap := BuildSnapshotFull(nil, nil, nil, nil, nil, map[string]string{"27VA15": fmt.Sprintf("VA15-%d", i)}, ny(2026, 9, 4, 12))
			errCh <- WriteSnapshot(path, snap)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("torn cards.json: %v", err)
	}
	if got.Source != "card-swipes" {
		t.Fatalf("source %q", got.Source)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "cards.json" {
		t.Fatalf("temporary files leaked: %+v", entries)
	}
}
