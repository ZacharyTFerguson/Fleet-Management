package oil

import (
	"fmt"
	"math"
	"time"

	"oilchange/internal/model"
)

// LastReading is the only place Last Reading miles are computed.
// Network and SQL stay out of this package so a helper cannot grab OneStep's device odometer.
func LastReading(enterpriseOdo int, fillTime time.Time, milesSince float64) (reading int, holds []model.Hold, err error) {
	if enterpriseOdo < 0 {
		return 0, nil, fmt.Errorf("enterprise odo %d is not a real odometer", enterpriseOdo)
	}
	if math.IsNaN(milesSince) || math.IsInf(milesSince, 0) {
		return 0, nil, fmt.Errorf("miles-since %v is not a measured distance", milesSince)
	}
	if milesSince < 0 {
		// Negative GPS sum is not a mile we can add; caller should HOLD NO_DRIVESTOP instead of inventing.
		return 0, []model.Hold{{Code: model.HoldNoDriveStop, Detail: "miles-since was negative"}}, nil
	}
	_ = fillTime // known second is required by the formula even though the add is odo + miles
	rounded := int(math.Round(milesSince))
	return enterpriseOdo + rounded, nil, nil
}
