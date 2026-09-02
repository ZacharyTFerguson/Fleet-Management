package oil

import "time"

const (
	// maxMPH is a common-sense cap when drive-stop miles are missing. Band rejects punches; it never invents Last Reading.
	maxMPH = 90.0
	// minBandMiles avoids a zero-width band on back-to-back punches a few minutes apart.
	minBandMiles = 25
)

// Band is the expected odometer window after a prior trusted reading.
type Band struct {
	Low  int
	High int
}

// ExpectedBand is prior trusted odo plus plausible miles (drive-stop if present, else 90 mph × hours).
func ExpectedBand(prior int, elapsed time.Duration, driveStop *float64) Band {
	hours := elapsed.Hours()
	if hours < 0 {
		hours = 0
	}
	capMiles := hours * maxMPH
	if capMiles < minBandMiles {
		capMiles = minBandMiles
	}
	high := float64(prior) + capMiles
	if driveStop != nil && *driveStop >= 0 {
		// Drive-stop is measured movement; allow slack so GPS undershoot does not reject a real punch.
		slack := *driveStop * 0.15
		if slack < 50 {
			slack = 50
		}
		dsHigh := float64(prior) + *driveStop + slack
		if dsHigh < high {
			high = dsHigh
		}
		// Still never below prior: a "drop" is not in-band.
	}
	return Band{Low: prior, High: int(high + 0.5)}
}

// Contains reports whether odo sits in the band, inclusive.
func (b Band) Contains(odo int) bool {
	return odo >= b.Low && odo <= b.High
}
