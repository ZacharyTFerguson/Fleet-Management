package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// Headers are the only report columns. Remaining and due formula names are forbidden.
var Headers = []string{
	"efleets_id",
	"nickname",
	"pdi_id",
	"last_oil_miles",
	"last_oil_date",
	"last_reading_miles",
	"last_reading_at",
	"last_reading_source",
	"hold_reason",
}

// forbidden are names operators used to write into the live sheet. The exporter must never emit them.
var forbidden = []string{
	"Change oil at 0",
	"Mileage due at",
	"remaining",
	"due",
}

// WriteCSV emits a lean last-oil + last-reading file matched by eFleets ID.
func WriteCSV(w io.Writer, cars []model.Car, dueWithin int, intervalDefault int) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(Headers); err != nil {
		return err
	}
	for _, h := range Headers {
		for _, f := range forbidden {
			if h == f {
				return fmt.Errorf("exporter refused forbidden header %q", h)
			}
		}
	}
	for _, c := range cars {
		if dueWithin > 0 && c.LastOilMiles != nil && c.HoldReason == nil {
			due := oil.DueAt(*c.LastOilMiles, coalesce(c.IntervalMiles, intervalDefault))
			if c.LastReadingMiles != nil {
				left := due - *c.LastReadingMiles
				if left > dueWithin {
					continue
				}
			}
		}
		rec := []string{
			c.EFleetsID,
			c.Nickname,
			c.PDIID,
			fmtInt(c.LastOilMiles),
			fmtDay(c.LastOilDate),
			fmtInt(c.LastReadingMiles),
			fmtTime(c.LastReadingAt),
			fmtPtr(c.LastReadingSource),
			fmtPtr(c.HoldReason),
		}
		if c.HoldReason != nil && *c.HoldReason != "" {
			// Stale last_reading_* must not be presented as current odo.
			rec[5] = ""
			rec[6] = ""
			rec[7] = ""
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// coalesce uses per-car interval when set so due-within filtering does not invent 5000 for a 7500 car.
func coalesce(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

// fmtInt writes blank for nil so "no reading" is not 0 miles in the CSV.
func fmtInt(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// fmtPtr writes blank for nil hold_reason (car is not on HOLD).
func fmtPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// fmtDay is last oil date only; remaining/due stay out of the file.
func fmtDay(p *time.Time) string {
	if p == nil || p.IsZero() {
		return ""
	}
	return p.Format("2006-01-02")
}

// fmtTime is last reading timestamp in UTC so a naive local export cannot shift the known second.
func fmtTime(p *time.Time) string {
	if p == nil || p.IsZero() {
		return ""
	}
	return p.UTC().Format(time.RFC3339)
}

// HasForbiddenHeader reports remaining/due names so tests can lock the exporter.
func HasForbiddenHeader(headers []string) bool {
	for _, h := range headers {
		for _, f := range forbidden {
			if h == f {
				return true
			}
		}
	}
	return false
}
