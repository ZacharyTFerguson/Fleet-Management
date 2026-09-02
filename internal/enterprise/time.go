package enterprise

import (
	"fmt"
	"strings"
	"time"
)

// ny is the zone for naive eFleets stamps. Punches without a zone are eastern, not UTC and not local-machine.
var ny *time.Location

func init() {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		ny = time.FixedZone("America/New_York", -5*60*60)
		return
	}
	ny = loc
}

// NY returns the locked zone for naive timestamps.
func NY() *time.Location { return ny }

// ParseFillTime joins DETAILS date + time to the second. Naive values get America/New_York.
func ParseFillTime(date, clock string) (time.Time, error) {
	date = strings.TrimSpace(date)
	clock = strings.TrimSpace(clock)
	if date == "" {
		return time.Time{}, fmt.Errorf("empty provider transaction date")
	}
	raw := date
	if clock != "" {
		raw = date + " " + clock
	}
	layouts := []string{
		"01/02/2006 03:04:05 PM",
		"01/02/2006 3:04:05 PM",
		"01/02/2006 15:04:05",
		"01/02/2006 03:04 PM",
		"01/02/2006",
		"2006-01-02 15:04:05",
		"2006-01-02",
		time.RFC3339,
	}
	var last error
	for _, l := range layouts {
		t, err := time.ParseInLocation(l, raw, ny)
		if err == nil {
			return t.Truncate(time.Second), nil
		}
		last = err
	}
	return time.Time{}, fmt.Errorf("parse fill time %q: %w", raw, last)
}

// ParseDate is shop RO / oil-done dates. Naive -> America/New_York.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	layouts := []string{"01/02/2006", "2006-01-02", time.RFC3339}
	var last error
	for _, l := range layouts {
		t, err := time.ParseInLocation(l, s, ny)
		if err == nil {
			return t, nil
		}
		last = err
	}
	return time.Time{}, fmt.Errorf("parse date %q: %w", s, last)
}
