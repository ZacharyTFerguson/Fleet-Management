package enterprise

import (
	"fmt"
	"strings"
)

// SessionConfig is the live eFleets knobs. Password is last-resort only.
type SessionConfig struct {
	CDPURL     string
	Username   string
	Password   string
	CustNum    string
	BaseURL    string
	DetailsURL string
	MaintURL   string
	FleetURL   string
}

// Select picks files → Chrome CDP session → password HTTP.
// File drop is the fallback when no session is configured. Password HTTP is
// not the lock; an open Chrome session must win when EFLEETS_CDP_URL is set.
func Select(files FileAdapter, cfg SessionConfig) (Adapter, error) {
	if files.Vehicles != "" || files.Fuel != "" || files.ShopRO != "" {
		return files, nil
	}
	if strings.TrimSpace(cfg.CDPURL) != "" {
		return NewChromeSessionAdapter(cfg.CDPURL, cfg.DetailsURL, cfg.MaintURL, cfg.FleetURL)
	}
	if cfg.Username != "" {
		h, err := NewHTTPAdapter(cfg.BaseURL, cfg.Username, cfg.Password, cfg.CustNum)
		if err != nil {
			return nil, err
		}
		h.DetailsURL = cfg.DetailsURL
		h.MaintURL = cfg.MaintURL
		h.FleetURL = cfg.FleetURL
		return h, nil
	}
	return nil, fmt.Errorf("pass --vehicles/--fuel-details, or set EFLEETS_CDP_URL (open Chrome session), or EFLEETS_USERNAME")
}
