// Package history builds the owner fill board. One drag moves one transaction.
// Storage is car-keyed; the desk shows one region at a time (turnstile).
package history

import (
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
)

const unsetRegion = "UNSET"

// FillBlock is one transaction on the History page.
type FillBlock struct {
	TxKey               string    `json:"tx_key"`
	CardID              string    `json:"card_id"`
	At                  time.Time `json:"at"`
	Station             string    `json:"station"`
	Address             string    `json:"address,omitempty"`
	Gallons             *float64  `json:"gallons,omitempty"`
	Odometer            *int      `json:"odometer,omitempty"`
	Driver              string    `json:"driver,omitempty"`
	EnterpriseEFleetsID string    `json:"enterprise_efleets_id,omitempty"`
	EnterprisePDIID     string    `json:"enterprise_pdi_id,omitempty"`
	EnterpriseName      string    `json:"enterprise_name,omitempty"`
	GPSCalledEFleetsID  string    `json:"gps_called_efleets_id,omitempty"`
	GPSCalledName       string    `json:"gps_called_name,omitempty"`
	AssignedEFleetsID   string    `json:"assigned_efleets_id,omitempty"`
	AssignedPDIID       string    `json:"assigned_pdi_id,omitempty"`
	AssignedName        string    `json:"assigned_name,omitempty"`
	GPSDisagrees        bool      `json:"gps_disagrees"`
	Source              string    `json:"source,omitempty"`
}

// CarColumn is one roster car in the current region.
type CarColumn struct {
	PDIID     string      `json:"pdi_id"`
	EFleetsID string      `json:"efleets_id"`
	Nickname  string      `json:"nickname"`
	Region    string      `json:"region"`
	Plate     string      `json:"plate,omitempty"`
	Fills     []FillBlock `json:"fills"`
}

// Board is the turnstile payload: one region of car columns plus a tray.
type Board struct {
	SyncedAt     time.Time   `json:"synced_at"`
	Region       string      `json:"region"`
	Regions      []string    `json:"regions"`
	Cars         []CarColumn `json:"cars"`
	Unassigned   []FillBlock `json:"unassigned"`
	AssignedN    int         `json:"assigned_n"`
	UnassignedN  int         `json:"unassigned_n"`
	GPSFlagN     int         `json:"gps_flag_n"`
	Swipes       int         `json:"swipes"`
}

type carMeta struct {
	pdi, nick, region, plate string
}

// BuildBoard files each fill under its owner-assigned car (or the tray).
// Region pick is display-only; the database stays car-keyed.
func BuildBoard(cars []model.Car, txs []model.CardTx, assigns []model.TxAssignment, region string, now time.Time) Board {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	byE := map[string]carMeta{}
	regions := map[string]struct{}{}
	for _, c := range cars {
		id := strings.TrimSpace(c.EFleetsID)
		if id == "" {
			continue
		}
		r := strings.TrimSpace(c.Region)
		if r == "" {
			r = unsetRegion
		}
		regions[r] = struct{}{}
		byE[id] = carMeta{pdi: c.PDIID, nick: c.Nickname, region: r, plate: c.Plate}
	}
	asg := map[string]model.TxAssignment{}
	for _, a := range assigns {
		asg[a.TxKey] = a
	}

	regionList := make([]string, 0, len(regions))
	for r := range regions {
		regionList = append(regionList, r)
	}
	sort.Strings(regionList)
	want := strings.TrimSpace(region)
	if want == "" && len(regionList) > 0 {
		want = regionList[0]
	}

	var all []FillBlock
	for _, t := range txs {
		if strings.TrimSpace(t.CardID) == "" || t.At.IsZero() {
			continue
		}
		a := asg[t.Key()]
		called := strings.TrimSpace(a.GPSCalledEFleetsID)
		if called == "" {
			called = strings.TrimSpace(t.CalledEFleetsID)
		}
		ent := strings.TrimSpace(t.RecordedEFleetsID)
		b := FillBlock{
			TxKey:               t.Key(),
			CardID:              t.CardID,
			At:                  t.At.UTC(),
			Station:             t.StationName,
			Address:             t.StationAddress,
			Gallons:             t.Gallons,
			Odometer:            t.Odometer,
			Driver:              strings.TrimSpace(t.DriverFirst + " " + t.DriverLast),
			EnterpriseEFleetsID: ent,
			EnterprisePDIID:     byE[ent].pdi,
			EnterpriseName:      label(byE, ent),
			GPSCalledEFleetsID:  called,
			GPSCalledName:       label(byE, called),
			AssignedEFleetsID:   strings.TrimSpace(a.AssignedEFleetsID),
			AssignedPDIID:       strings.TrimSpace(a.AssignedPDIID),
			AssignedName:        label(byE, a.AssignedEFleetsID),
			GPSDisagrees:        a.GPSDisagrees,
			Source:              a.Source,
		}
		if b.AssignedPDIID == "" && b.AssignedEFleetsID != "" {
			b.AssignedPDIID = byE[b.AssignedEFleetsID].pdi
		}
		all = append(all, b)
	}

	board := Board{SyncedAt: now, Region: want, Regions: regionList, Swipes: len(all)}
	cols := map[string]*CarColumn{}
	for _, c := range cars {
		id := strings.TrimSpace(c.EFleetsID)
		if id == "" {
			continue
		}
		r := byE[id].region
		if r != want {
			continue
		}
		col := &CarColumn{
			PDIID: c.PDIID, EFleetsID: id, Nickname: c.Nickname, Region: r, Plate: c.Plate,
			Fills: []FillBlock{},
		}
		cols[id] = col
	}
	for _, b := range all {
		if b.AssignedEFleetsID != "" {
			board.AssignedN++
			if b.GPSDisagrees {
				board.GPSFlagN++
			}
			if col, ok := cols[b.AssignedEFleetsID]; ok {
				col.Fills = append(col.Fills, b)
			}
			continue
		}
		board.UnassignedN++
		if fillBelongsInRegion(b, byE, want) {
			board.Unassigned = append(board.Unassigned, b)
		}
	}
	board.Cars = make([]CarColumn, 0, len(cols))
	for _, c := range cars {
		id := strings.TrimSpace(c.EFleetsID)
		col, ok := cols[id]
		if !ok {
			continue
		}
		sort.Slice(col.Fills, func(i, j int) bool { return col.Fills[i].At.Before(col.Fills[j].At) })
		board.Cars = append(board.Cars, *col)
	}
	sort.Slice(board.Unassigned, func(i, j int) bool { return board.Unassigned[i].At.After(board.Unassigned[j].At) })
	if board.Unassigned == nil {
		board.Unassigned = []FillBlock{}
	}
	return board
}

func fillBelongsInRegion(b FillBlock, byE map[string]carMeta, region string) bool {
	if region == "" {
		return true
	}
	for _, id := range []string{b.AssignedEFleetsID, b.GPSCalledEFleetsID, b.EnterpriseEFleetsID} {
		if id == "" {
			continue
		}
		if byE[id].region == region {
			return true
		}
	}
	// No car hint — keep the block on every turnstile stop so it is not lost.
	return b.EnterpriseEFleetsID == "" && b.GPSCalledEFleetsID == ""
}

func label(byE map[string]carMeta, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	m := byE[id]
	if m.nick != "" {
		return m.nick + " · " + id
	}
	return id
}
