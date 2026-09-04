package cards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"oilchange/internal/model"
)

type visitsFile struct {
	SavedAt time.Time          `json:"saved_at"`
	Visits  []model.StopVisit  `json:"visits"`
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
	return os.WriteFile(path, b, 0o644)
}

// LoadStopVisits reads a previous GPS stop dump.
func LoadStopVisits(path string) ([]model.StopVisit, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f visitsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f.Visits, nil
}
