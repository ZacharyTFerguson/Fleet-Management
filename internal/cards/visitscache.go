package cards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"oilchange/internal/model"
)

var visitsCacheMu sync.RWMutex

type visitsFile struct {
	SavedAt time.Time         `json:"saved_at"`
	Visits  []model.StopVisit `json:"visits"`
}

// SaveStopVisits writes GPS stop windows so card rematch does not re-hit OneStep.
func SaveStopVisits(path string, visits []model.StopVisit) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(visitsFile{SavedAt: time.Now().UTC(), Visits: visits})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
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
	visitsCacheMu.Lock()
	defer visitsCacheMu.Unlock()
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

// LoadStopVisits reads a previous GPS stop dump.
func LoadStopVisits(path string) ([]model.StopVisit, error) {
	visitsCacheMu.RLock()
	b, err := os.ReadFile(path)
	visitsCacheMu.RUnlock()
	if err != nil {
		return nil, err
	}
	var f visitsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f.Visits, nil
}
