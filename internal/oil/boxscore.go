package oil

import (
	"fmt"
	"math"
	"sort"
	"time"

	"oilchange/internal/model"
)

const (
	LedgerTrusted   = "trusted"
	LedgerSuspect   = "suspect"
	LedgerHold      = "hold"
	LedgerDismissed = "dismissed"
	LedgerCorrected = "corrected"
	TrendUp         = "up"
	TrendDown       = "down"
	TrendFlat       = "flat"
	TrendHold       = "hold"
)

// GasCardPunch is one WEX/Enterprise fuel punch (recorded side of the box score).
type GasCardPunch struct {
	EFleetsID string    `json:"efleets_id"`
	CardID    string    `json:"card_id"`
	Recorded  int       `json:"recorded"`
	At        time.Time `json:"at"`
	UnusualY  bool      `json:"unusual_y"`
	Merchant  string    `json:"merchant"`
}

// BoxScoreIn is everything ScorePunches may see. No HTTP, no SQL, no invented miles.
type BoxScoreIn struct {
	EFleetsID    string
	Nickname     string
	MaintOdo     int       // last good shop/RO (or validated maintenance)
	MaintAt      time.Time // maintenance timestamp
	HasMaint     bool
	HasDevice    bool
	Punches      []GasCardPunch
	// MilesToPunch is measured drive-stop miles from MaintAt to that punch second.
	// Missing key → HOLD NO_DRIVESTOP (not zero).
	MilesToPunch map[int64]float64
}

// LedgerRow is one punch vs expected. Last Reading is not written here.
type LedgerRow struct {
	EFleetsID  string    `json:"efleets_id"`
	CardID     string    `json:"card_id"`
	PunchAt    time.Time `json:"punch_at"`
	Merchant   string    `json:"merchant"`
	Recorded   int       `json:"recorded"`
	MaintOdo   int       `json:"maint_odo"`
	MaintAt    time.Time `json:"maint_at"`
	MilesSince *float64  `json:"miles_since,omitempty"`
	Expected   *int      `json:"expected,omitempty"`
	Overage    int       `json:"overage"`
	Shortage   int       `json:"shortage"`
	AbsDiff    *int      `json:"abs_diff,omitempty"`
	Trend      string    `json:"trend"`
	Status     string    `json:"status"`
	HoldReason string    `json:"hold_reason,omitempty"`
	HoldDetail string    `json:"hold_detail,omitempty"`
	InTrend    bool      `json:"in_trend"`
}

// BoxScoreOut is per-vehicle ledger + rollup. Suspect/HOLD rows stay visible.
type BoxScoreOut struct {
	EFleetsID     string      `json:"efleets_id"`
	MaintOdo      int         `json:"maint_odo"`
	MaintAt       time.Time   `json:"maint_at"`
	HasMaint      bool        `json:"has_maint"`
	Rows          []LedgerRow `json:"rows"`
	TrendTrusted  int         `json:"trend_trusted"`
	TrendUp       int         `json:"trend_up"`
	TrendDown     int         `json:"trend_down"`
	TrendFlat     int         `json:"trend_flat"`
	SumOverage    int         `json:"sum_overage"`
	SumShortage   int         `json:"sum_shortage"`
	LatestAbsDiff *int        `json:"latest_abs_diff,omitempty"`
	LatestTrend   string      `json:"latest_trend,omitempty"`
}

// ScorePunches compares each gas card transaction to maintenance + measured drive-stop.
// Sign: overage = recorded − expected (card ahead); shortage = expected − recorded (card behind).
func ScorePunches(in BoxScoreIn) BoxScoreOut {
	out := BoxScoreOut{EFleetsID: in.EFleetsID, MaintOdo: in.MaintOdo, MaintAt: in.MaintAt, HasMaint: in.HasMaint}
	punches := append([]GasCardPunch(nil), in.Punches...)
	sort.Slice(punches, func(i, j int) bool {
		if punches[i].At.Equal(punches[j].At) {
			return punches[i].Recorded < punches[j].Recorded
		}
		return punches[i].At.Before(punches[j].At)
	})

	var prevAbs *int
	for _, p := range punches {
		row := scoreOne(in, p)
		if row.Status == LedgerTrusted && row.AbsDiff != nil {
			row.Trend, row.InTrend = trendVs(prevAbs, *row.AbsDiff)
			prevAbs = row.AbsDiff
			out.TrendTrusted++
			switch row.Trend {
			case TrendUp:
				out.TrendUp++
			case TrendDown:
				out.TrendDown++
			default:
				out.TrendFlat++
			}
			out.SumOverage += row.Overage
			out.SumShortage += row.Shortage
			out.LatestAbsDiff = row.AbsDiff
			out.LatestTrend = row.Trend
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

func scoreOne(in BoxScoreIn, p GasCardPunch) LedgerRow {
	row := LedgerRow{
		EFleetsID: in.EFleetsID,
		CardID:    p.CardID,
		PunchAt:   p.At.UTC().Truncate(time.Second),
		Merchant:  p.Merchant,
		Recorded:  p.Recorded,
		MaintOdo:  in.MaintOdo,
		MaintAt:   in.MaintAt,
		Trend:     TrendHold,
		Status:    LedgerHold,
	}
	if p.Recorded <= 0 || p.At.IsZero() {
		row.HoldReason = model.HoldNoTrustedFill
		row.HoldDetail = "gas card transaction missing odometer or punch time"
		return row
	}
	if p.UnusualY {
		row.Status = LedgerSuspect
		row.HoldReason = model.HoldUnusualY
		row.HoldDetail = "unusual-Y gas card transaction — visible, excluded from trend"
		return row
	}
	if HasLogisticsPersonnel(in.Nickname) {
		row.HoldReason = model.HoldLogisticsPersonnel
		row.HoldDetail = "logistics-personnel label; not a pairing key"
		return row
	}
	if !in.HasMaint || in.MaintOdo <= 0 || in.MaintAt.IsZero() {
		row.HoldReason = model.HoldNoTrustedFill
		row.HoldDetail = "no good maintenance odometer before this gas card transaction"
		return row
	}
	if p.At.Before(in.MaintAt) {
		row.Status = LedgerSuspect
		row.HoldReason = model.HoldNoTrustedFill
		row.HoldDetail = "gas card transaction is before the good maintenance stamp"
		return row
	}
	if !in.HasDevice {
		row.HoldReason = model.HoldNoDevice
		row.HoldDetail = "no live factory_id; cannot measure drive-stop since maintenance"
		return row
	}
	key := p.At.UTC().Truncate(time.Second).Unix()
	miles, ok := in.MilesToPunch[key]
	if !ok {
		row.HoldReason = model.HoldNoDriveStop
		row.HoldDetail = "no drive-stop miles from maintenance to this gas card transaction"
		return row
	}
	if math.IsNaN(miles) || math.IsInf(miles, 0) || miles < 0 {
		row.HoldReason = model.HoldNoDriveStop
		row.HoldDetail = "drive-stop miles-since was not a measured non-negative distance"
		return row
	}
	ms := miles
	row.MilesSince = &ms
	expected := in.MaintOdo + int(math.Round(miles))
	row.Expected = &expected
	diff := p.Recorded - expected
	if diff > 0 {
		row.Overage = diff
	} else if diff < 0 {
		row.Shortage = -diff
	}
	abs := diff
	if abs < 0 {
		abs = -abs
	}
	row.AbsDiff = &abs
	row.Status = LedgerTrusted
	row.HoldReason = ""
	row.HoldDetail = ""
	return row
}

func trendVs(prev *int, abs int) (string, bool) {
	if prev == nil {
		return TrendFlat, true
	}
	switch {
	case abs > *prev:
		return TrendUp, true
	case abs < *prev:
		return TrendDown, true
	default:
		return TrendFlat, true
	}
}

// GoodMaintenance picks the latest non-abandoned shop RO as the box-score base.
// Fuel punches are never the expected base. OneStep odometer is never consulted.
func GoodMaintenance(ros []model.ShopRO, fills []model.Fill) (odo int, at time.Time, ok bool) {
	shop := latestShop(ros)
	if shop == nil {
		return 0, time.Time{}, false
	}
	if shopLooksAbandoned(*shop, fills, shop.Odo) {
		return 0, time.Time{}, false
	}
	return shop.Odo, shop.At, true
}

func (r LedgerRow) Key() string {
	return fmt.Sprintf("%s|%s|%d", r.EFleetsID, r.PunchAt.UTC().Format(time.RFC3339), r.Recorded)
}
