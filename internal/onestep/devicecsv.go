package onestep

import (
	"encoding/csv"
	"io"

	"oilchange/internal/model"
)

// DeviceCSVHeaders is the inventory dump. display_name is a label only.
var DeviceCSVHeaders = []string{
	"factory_id",
	"device_id",
	"display_name",
	"linked_car_efleets_id",
	"status",
}

// DeviceStatus is active unless the box is dead or inactive.
func DeviceStatus(d model.OneStepDevice) string {
	if d.Dead || !d.Active {
		return "retired"
	}
	return "active"
}

// WriteDevicesCSV writes OneStep inventory. factory_id is the join key;
// display_name is never used as an eFleets join.
func WriteDevicesCSV(w io.Writer, devs []model.OneStepDevice) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(DeviceCSVHeaders); err != nil {
		return err
	}
	for _, d := range devs {
		link := ""
		if d.LinkedCarEFleetsID != nil {
			link = *d.LinkedCarEFleetsID
		}
		row := []string{
			d.FactoryID,
			d.DeviceID,
			d.DisplayName,
			link,
			DeviceStatus(d),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
