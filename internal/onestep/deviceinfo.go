package onestep

import (
	"os"

	"oilchange/internal/model"
)

// ParseDevices turns a /device JSON body or a Device Information generate-reports
// export (`{"data":[…]}` with imei + vin) into registry rows. Odometer is dropped.
// params.vin is not identity. display_name is a label only.
func ParseDevices(b []byte) ([]model.OneStepDevice, error) {
	return parseDevices(b)
}

// LoadDevicesJSON reads a saved Device Information / /device JSON file. No HTTP.
func LoadDevicesJSON(path string) ([]model.OneStepDevice, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseDevices(b)
}
