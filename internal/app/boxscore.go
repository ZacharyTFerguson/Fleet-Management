package app

import (
	"context"
	"fmt"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// FleetBoxScore is the desk rollup. Last Reading is not written here.
type FleetBoxScore struct {
	At           string            `json:"at"`
	Note         string            `json:"note"`
	Vehicles     []oil.BoxScoreOut `json:"vehicles"`
	SumOverage   int               `json:"sum_overage"`
	SumShortage  int               `json:"sum_shortage"`
	TrendUp      int               `json:"trend_up"`
	TrendDown    int               `json:"trend_down"`
	TrendFlat    int               `json:"trend_flat"`
	SuspectN     int               `json:"suspect_n"`
	HoldN        int               `json:"hold_n"`
	TrustedN     int               `json:"trusted_n"`
}

// RebuildBoxScore scores every gas card transaction against good maintenance + stored drive-stop windows.
// Live fetch is opt-in per punch (MeasureBoxScorePunch). Missing miles → HOLD, never zero.
func (a *App) RebuildBoxScore(ctx context.Context) (FleetBoxScore, error) {
	cars, err := a.Store.ListCars(ctx)
	if err != nil {
		return FleetBoxScore{}, err
	}
	out := FleetBoxScore{
		At:   time.Now().UTC().Format(time.RFC3339),
		Note: "Recorded = gas card transaction odo + punch time. Expected = good maintenance odo + OneStep drive-stop since that stamp. Suspect/HOLD visible, excluded from trend. Never invent miles. Never OneStep odometer.",
	}
	for _, car := range cars {
		sc, err := a.scoreCar(ctx, car, false, time.Time{})
		if err != nil {
			return FleetBoxScore{}, err
		}
		out.Vehicles = append(out.Vehicles, sc)
		out.SumOverage += sc.SumOverage
		out.SumShortage += sc.SumShortage
		out.TrendUp += sc.TrendUp
		out.TrendDown += sc.TrendDown
		out.TrendFlat += sc.TrendFlat
		for _, r := range sc.Rows {
			switch r.Status {
			case oil.LedgerTrusted:
				out.TrustedN++
			case oil.LedgerSuspect:
				out.SuspectN++
			default:
				if r.Status == oil.LedgerHold {
					out.HoldN++
				}
			}
			if err := a.Store.UpsertLedgerRow(ctx, r); err != nil {
				return FleetBoxScore{}, err
			}
		}
	}
	return out, nil
}

func (a *App) ListBoxScore(ctx context.Context) (FleetBoxScore, error) {
	rows, err := a.Store.ListLedger(ctx, "")
	if err != nil {
		return FleetBoxScore{}, err
	}
	if len(rows) == 0 {
		return a.RebuildBoxScore(ctx)
	}
	byCar := map[string][]oil.LedgerRow{}
	var ids []string
	for _, r := range rows {
		if _, ok := byCar[r.EFleetsID]; !ok {
			ids = append(ids, r.EFleetsID)
		}
		byCar[r.EFleetsID] = append(byCar[r.EFleetsID], r)
	}
	out := FleetBoxScore{
		At:   time.Now().UTC().Format(time.RFC3339),
		Note: "Recorded = gas card transaction. Expected = maintenance + drive-stop. Trend uses trusted rows only.",
	}
	for _, id := range ids {
		rs := byCar[id]
		sc := oil.BoxScoreOut{EFleetsID: id, Rows: rs}
		for _, r := range rs {
			if r.MaintOdo > 0 {
				sc.MaintOdo = r.MaintOdo
				sc.MaintAt = r.MaintAt
				sc.HasMaint = true
			}
			switch r.Status {
			case oil.LedgerTrusted:
				out.TrustedN++
				sc.TrendTrusted++
				out.SumOverage += r.Overage
				out.SumShortage += r.Shortage
				sc.SumOverage += r.Overage
				sc.SumShortage += r.Shortage
				if r.AbsDiff != nil {
					sc.LatestAbsDiff = r.AbsDiff
					sc.LatestTrend = r.Trend
				}
				switch r.Trend {
				case oil.TrendUp:
					out.TrendUp++
					sc.TrendUp++
				case oil.TrendDown:
					out.TrendDown++
					sc.TrendDown++
				case oil.TrendFlat:
					out.TrendFlat++
					sc.TrendFlat++
				}
			case oil.LedgerSuspect:
				out.SuspectN++
			case oil.LedgerHold:
				out.HoldN++
			}
		}
		out.Vehicles = append(out.Vehicles, sc)
	}
	return out, nil
}

func (a *App) scoreCar(ctx context.Context, car model.Car, fetchLive bool, onlyPunch time.Time) (oil.BoxScoreOut, error) {
	fills, err := a.Store.ListFills(ctx, car.EFleetsID)
	if err != nil {
		return oil.BoxScoreOut{}, err
	}
	ros, err := a.Store.ListShopROs(ctx, car.EFleetsID)
	if err != nil {
		return oil.BoxScoreOut{}, err
	}
	devs, err := a.Store.ListDevicesForCar(ctx, car.EFleetsID)
	if err != nil {
		return oil.BoxScoreOut{}, err
	}
	live := liveLinked(devs)
	maintOdo, maintAt, hasMaint := oil.GoodMaintenance(ros, fills)
	in := oil.BoxScoreIn{
		EFleetsID:    car.EFleetsID,
		Nickname:     car.Nickname,
		MaintOdo:     maintOdo,
		MaintAt:      maintAt,
		HasMaint:     hasMaint,
		HasDevice:    len(live) > 0,
		MilesToPunch: map[int64]float64{},
	}
	for _, f := range fills {
		if f.Odometer == nil {
			continue
		}
		in.Punches = append(in.Punches, oil.GasCardPunch{
			EFleetsID: car.EFleetsID,
			CardID:    firstNonEmpty(f.CardID, f.CardCompanyVehicleNumber),
			Recorded:  *f.Odometer,
			At:        f.ProviderTransactionTime,
			UnusualY:  f.UnusualY,
			Merchant:  f.MerchantName,
		})
		if !hasMaint || len(live) == 0 {
			continue
		}
		to := f.ProviderTransactionTime.UTC().Truncate(time.Second)
		if !onlyPunch.IsZero() && !to.Equal(onlyPunch.UTC().Truncate(time.Second)) {
			continue
		}
		miles, ok, err := a.windowMiles(ctx, live, maintAt, to, fetchLive)
		if err != nil {
			return oil.BoxScoreOut{}, err
		}
		if ok {
			in.MilesToPunch[to.Unix()] = miles
		}
	}
	return oil.ScorePunches(in), nil
}

func liveLinked(devs []model.OneStepDevice) []model.OneStepDevice {
	var out []model.OneStepDevice
	for _, d := range devs {
		if d.Dead || d.FactoryID == "" {
			continue
		}
		if oil.HasLogisticsPersonnel(d.DisplayName) {
			continue
		}
		out = append(out, d)
	}
	return out
}


func (a *App) windowMiles(ctx context.Context, live []model.OneStepDevice, from, to time.Time, fetchLive bool) (float64, bool, error) {
	for _, d := range live {
		if m, ok, err := a.Store.GetDriveStopWindow(ctx, d.FactoryID, from, to); err != nil {
			return 0, false, err
		} else if ok {
			return m, true, nil
		}
	}
	if !fetchLive {
		return 0, false, nil
	}
	c := a.oneStepClient()
	if c == nil {
		return 0, false, nil
	}
	did := live[0].DeviceID
	if did == "" {
		did = live[0].FactoryID
	}
	n, err := c.DriveStopMilesWindow(ctx, did, from, to)
	if err != nil {
		return 0, false, err
	}
	if err := a.Store.SaveDriveStopWindow(ctx, live[0].FactoryID, from, to, n); err != nil {
		return 0, false, err
	}
	return n, true, nil
}

// MeasureBoxScorePunch fetches drive-stop for one punch window (maint → gas card time) and rebuilds that car.
func (a *App) MeasureBoxScorePunch(ctx context.Context, efleetsID string, punchAt time.Time) (oil.BoxScoreOut, error) {
	car, err := a.Store.CarByEFleets(ctx, efleetsID)
	if err != nil {
		return oil.BoxScoreOut{}, err
	}
	if car == nil {
		return oil.BoxScoreOut{}, fmt.Errorf("car %s not found", efleetsID)
	}
	sc, err := a.scoreCar(ctx, *car, true, punchAt)
	if err != nil {
		return oil.BoxScoreOut{}, err
	}
	for _, r := range sc.Rows {
		if err := a.Store.UpsertLedgerRow(ctx, r); err != nil {
			return oil.BoxScoreOut{}, err
		}
	}
	return sc, nil
}

func (a *App) DismissLedger(ctx context.Context, efleetsID, punchAt string, recorded int) error {
	return a.Store.SetLedgerStatus(ctx, efleetsID, punchAt, recorded, oil.LedgerDismissed, "operator dismissed — still visible, not in trend")
}

func (a *App) CorrectLedger(ctx context.Context, efleetsID, punchAt string, recorded int) error {
	return a.Store.SetLedgerStatus(ctx, efleetsID, punchAt, recorded, oil.LedgerCorrected, "operator marked gas card transaction corrected")
}
