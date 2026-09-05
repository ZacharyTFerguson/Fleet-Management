package onestep

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"oilchange/internal/model"
)

// ValidVIN is a 17-character OBD VIN, or empty. Display_name / plate never pass.
func ValidVIN(s string) string {
	v := normalizeVIN(s)
	if len(v) != vinLen {
		return ""
	}
	return v
}

func pickDeviceVIN(want model.OneStepDevice, devs []model.OneStepDevice) string {
	fid := strings.TrimSpace(want.FactoryID)
	did := strings.TrimSpace(want.DeviceID)
	if did == "" {
		did = fid
	}
	for _, d := range devs {
		if fid != "" && strings.TrimSpace(d.FactoryID) == fid {
			return ValidVIN(d.VIN)
		}
	}
	for _, d := range devs {
		if did != "" && strings.TrimSpace(d.DeviceID) == did {
			return ValidVIN(d.VIN)
		}
	}
	if len(devs) == 1 {
		return ValidVIN(devs[0].VIN)
	}
	return ""
}

// AskDeviceVIN GETs /device?device_id=&latest_point=true and returns the OBD
// device_state.vin for this box. If the API ignores device_id and returns the
// inventory, All is that list so the caller can pair in one batch.
// params.vin and display_name are not identity.
func (c *Client) AskDeviceVIN(ctx context.Context, d model.OneStepDevice) (vin string, all []model.OneStepDevice, err error) {
	if c == nil {
		return "", nil, fmt.Errorf("onestep: no client")
	}
	did := strings.TrimSpace(d.DeviceID)
	if did == "" {
		did = strings.TrimSpace(d.FactoryID)
	}
	if did == "" {
		return "", nil, fmt.Errorf("device VIN needs a device_id")
	}
	q := url.Values{}
	q.Set("device_id", did)
	q.Set("latest_point", "true")
	b, err := c.lockedGet(ctx, "/v3/api/public/device", q)
	if err != nil {
		return "", nil, err
	}
	devs, err := parseDevices(b)
	if err != nil {
		return "", nil, err
	}
	return pickDeviceVIN(d, devs), devs, nil
}
