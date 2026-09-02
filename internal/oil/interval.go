package oil

import "oilchange/internal/model"

// IntervalMiles uses the per-car override when set so v1 needs no make/model table.
func IntervalMiles(perCar int) int {
	if perCar <= 0 {
		return model.DefaultInterval
	}
	return perCar
}

// DueAt is last oil plus interval. Computed in-app for report filters; never written as a remaining/due column.
func DueAt(lastOilMiles, perCarInterval int) int {
	return lastOilMiles + IntervalMiles(perCarInterval)
}
