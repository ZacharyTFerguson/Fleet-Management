package history

import (
	"strings"
	"time"

	"oilchange/internal/cards"
	"oilchange/internal/model"
)

// NearbyStop is one GPS-linked car sitting still at a swipe time.
type NearbyStop struct {
	EFleetsID string    `json:"efleets_id"`
	Nickname  string    `json:"nickname,omitempty"`
	PDIID     string    `json:"pdi_id,omitempty"`
	FactoryID string    `json:"factory_id,omitempty"`
	DeviceID  string    `json:"device_id,omitempty"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	HasPos    bool      `json:"has_pos"`
}

// FillEvidence is Devices-page GPS for one transaction. Cache only unless Probe is used.
type FillEvidence struct {
	TxKey               string       `json:"tx_key"`
	CardID              string       `json:"card_id"`
	At                  time.Time    `json:"at"`
	Station             string       `json:"station,omitempty"`
	EnterpriseEFleetsID string       `json:"enterprise_efleets_id,omitempty"`
	AssignedEFleetsID   string       `json:"assigned_efleets_id,omitempty"`
	GPSCalledEFleetsID  string       `json:"gps_called_efleets_id,omitempty"`
	GPSDisagrees        bool         `json:"gps_disagrees"`
	Nearby              []NearbyStop `json:"nearby"`
	Live                string       `json:"live_note"`
}

// BoxEvidence is Devices-page GPS for one factory_id / device.
type BoxEvidence struct {
	FactoryID          string `json:"factory_id"`
	DeviceID           string `json:"device_id"`
	DisplayName        string `json:"display_name,omitempty"`
	VIN                string `json:"vin,omitempty"`
	LinkedCarEFleetsID string `json:"linked_car_efleets_id,omitempty"`
	LinkedCarPDIID     string `json:"linked_car_pdi_id,omitempty"`
	Active             bool   `json:"active"`
	StopsInCache       int    `json:"stops_in_cache"`
}

// BuildFillEvidence lists cached short stops covering this swipe.
func BuildFillEvidence(tx model.CardTx, asg model.TxAssignment, visits []model.StopVisit, cars []model.Car) FillEvidence {
	nicks := map[string]model.Car{}
	for _, c := range cars {
		nicks[strings.TrimSpace(c.EFleetsID)] = c
	}
	called := strings.TrimSpace(asg.GPSCalledEFleetsID)
	if called == "" {
		called = strings.TrimSpace(tx.CalledEFleetsID)
	}
	out := FillEvidence{
		TxKey:               tx.Key(),
		CardID:              tx.CardID,
		At:                  tx.At.UTC(),
		Station:             tx.StationName,
		EnterpriseEFleetsID: tx.RecordedEFleetsID,
		AssignedEFleetsID:   asg.AssignedEFleetsID,
		GPSCalledEFleetsID:  called,
		GPSDisagrees:        asg.GPSDisagrees,
		Live:                "cache only — live OneStep is one fill or one box",
		Nearby:              []NearbyStop{},
	}
	for _, v := range cards.StopsCovering(visits, tx.At, cards.DefaultStopSlack) {
		c := nicks[strings.TrimSpace(v.EFleetsID)]
		out.Nearby = append(out.Nearby, NearbyStop{
			EFleetsID: v.EFleetsID,
			Nickname:  c.Nickname,
			PDIID:     c.PDIID,
			FactoryID: v.FactoryID,
			DeviceID:  v.DeviceID,
			From:      v.From,
			To:        v.To,
			HasPos:    v.HasPos,
		})
	}
	return out
}
