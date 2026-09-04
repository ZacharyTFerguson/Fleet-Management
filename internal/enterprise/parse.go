package enterprise

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"io"
	"strconv"
	"strings"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/oil"
)

// ParseVehicles reads Fleet Summary (or Active Vehicles) by header name. Active Vehicles is often stale; prefer Fleet Summary.
func ParseVehicles(r io.Reader) ([]model.Car, error) {
	rows, idx, err := readCSV(r)
	if err != nil {
		return nil, err
	}
	if err := requireHeaders(idx, "Vehicle"); err != nil {
		return nil, err
	}
	var cars []model.Car
	seen := map[string]struct{}{}
	seq := 0
	for _, row := range rows {
		id := col(idx, row, "Vehicle")
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		seq++
		nick := col(idx, row, "Customer Vehicle ID", "Customer Vehicle ID**")
		cars = append(cars, model.Car{
			PDIID:     formatPDI(seq),
			EFleetsID: id,
			Nickname:  nick,
			Plate:     col(idx, row, "License Num"),
			VIN:       col(idx, row, "VIN"),
			Region:    regionFromNickname(nick),
		})
	}
	return cars, nil
}

// ParseFills reads Fuel & Charging DETAILS. Unusual Y and odometer come from named columns, never I/J.
func ParseFills(r io.Reader) ([]model.Fill, []model.GasStation, []model.Card, error) {
	rows, idx, err := readCSV(r)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := requireHeaders(idx, "Vehicle", "Provider Odometer", "Provider Transaction Date"); err != nil {
		return nil, nil, nil, err
	}
	var fills []model.Fill
	stations := map[string]model.GasStation{}
	cards := map[string]model.Card{}
	for _, row := range rows {
		id := col(idx, row, "Vehicle")
		if id == "" {
			continue
		}
		at, err := ParseFillTime(col(idx, row, "Provider Transaction Date"), col(idx, row, "Provider Transaction Time"))
		if err != nil {
			continue
		}
		var odo *int
		if s := col(idx, row, "Provider Odometer"); s != "" {
			n, err := parseInt(s)
			if err == nil {
				odo = &n
			}
		}
		unusual := strings.EqualFold(col(idx, row, "Provider Unusual Odometer Flag"), "Y")
		addr := strings.TrimSpace(strings.Join([]string{
			col(idx, row, "Provider Transaction Site Address"),
			col(idx, row, "Provider Transaction Site City"),
			col(idx, row, "Provider Transaction Site State"),
		}, ", "))
		merchant := col(idx, row, "Provider Location")
		cardID := col(idx, row, "Provider Card Number")
		if cardID == "" {
			cardID = col(idx, row, "Provider Vehicle Number")
		}
		f := model.Fill{
			EFleetsID:                    id,
			CardID:                       cardID,
			CardCompanyVehicleNumber:     col(idx, row, "Provider Company Vehicle Number"),
			ProviderCompanyVehicleNumber: col(idx, row, "Provider Company Vehicle Number"),
			Odometer:                     odo,
			UnusualY:                     unusual,
			ProviderTransactionTime:      at,
			MerchantName:                 merchant,
			MerchantAddress:              addr,
			Source:                       model.SourceFuelDetails,
			DriverFirst:                  col(idx, row, "Provider Driver First Name"),
			DriverLast:                   col(idx, row, "Provider Driver Last Name"),
			Plate:                        col(idx, row, "License Num"),
			Gallons:                      parseOptFloat(col(idx, row, "Provider Units Purchased")),
			Amount:                       parseOptFloat(col(idx, row, "Provider Net Dollars", "Provider Gross Dollars")),
		}
		fills = append(fills, f)
		if merchant != "" {
			sid := stableID("gs", merchant, addr)
			stations[sid] = model.GasStation{ID: sid, Name: merchant, Address: addr}
		}
		if cardID != "" {
			cvn := f.ProviderCompanyVehicleNumber
			c := model.Card{ID: cardID, CompanyVehicleNumber: cvn}
			if !oil.HasLogisticsPersonnel(f.DriverFirst, f.DriverLast) {
				eid := id
				c.LinkedCarEFleetsID = &eid
			} else {
				c.Notes = "logistics personnel on punch; not a pairing key"
			}
			cards[cardID] = c
		}
	}
	return fills, mapVals(stations), mapVals(cards), nil
}

// ParseShopROs reads Maintenance Detail. One RO ID with many line items is still one odometer at one shop.
func ParseShopROs(r io.Reader) ([]model.ShopRO, []model.MaintenanceLocation, []model.OilChange, error) {
	rows, idx, err := readCSV(r)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := requireHeaders(idx, "Vehicle", "Odometer"); err != nil {
		return nil, nil, nil, err
	}
	var ros []model.ShopRO
	seenRO := map[string]struct{}{}
	locs := map[string]model.MaintenanceLocation{}
	oilByRO := map[string]model.OilChange{}
	for _, row := range rows {
		id := col(idx, row, "Vehicle")
		if id == "" {
			continue
		}
		odo, err := parseInt(col(idx, row, "Odometer"))
		if err != nil {
			continue
		}
		dateStr := col(idx, row, "RO Completed Date", "RO Complete Date")
		if emptyCell(dateStr) {
			dateStr = col(idx, row, "RO Created Date")
		}
		at, err := ParseDate(dateStr)
		if err != nil {
			at = time.Time{}
		}
		roid := col(idx, row, "RO ID")
		shop := col(idx, row, "Shop Name")
		desc := col(idx, row, "Service Desc")
		key := id + "|" + roid
		if _, ok := seenRO[key]; !ok {
			seenRO[key] = struct{}{}
			ros = append(ros, model.ShopRO{
				ROID:         roid,
				EFleetsID:    id,
				Odometer:     odo,
				At:           at,
				LocationName: shop,
				ServiceDesc:  desc,
			})
		}
		if shop != "" {
			lid := stableID("ml", shop)
			locs[lid] = model.MaintenanceLocation{ID: lid, Name: shop}
		}
		if oil.IsOilChangeService(desc) && roid != "" && !at.IsZero() {
			oilByRO[key] = model.OilChange{
				EFleetsID: id,
				Miles:     odo,
				Date:      at,
				Location:  shop,
				Source:    "shop_ro",
			}
		}
	}
	return ros, mapVals(locs), mapVals(oilByRO), nil
}

// ParseMileage is context only. Callers must not feed it to LastReading.
func ParseMileage(r io.Reader) ([]model.MileageEntry, error) {
	rows, idx, err := readCSV(r)
	if err != nil {
		return nil, err
	}
	var out []model.MileageEntry
	for _, row := range rows {
		id := col(idx, row, "Vehicle")
		odoStr := col(idx, row, "Odometer", "Odometer Reading")
		if odoStr == "" {
			continue
		}
		odo, err := parseInt(odoStr)
		if err != nil {
			continue
		}
		ds := col(idx, row, "Date", "Last Effective Date")
		at, _ := ParseDate(ds)
		out = append(out, model.MileageEntry{
			EFleetsID: id,
			At:        at,
			Odometer:  odo,
			Source:    col(idx, row, "Odometer Source", "Source"),
		})
	}
	return out, nil
}

// readCSV indexes the first row by name so a shifted column cannot become odometer.
func readCSV(r io.Reader) ([][]string, map[string]int, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	all, err := cr.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("empty csv")
	}
	return all[1:], headerIndex(all[0]), nil
}

// parseInt accepts eFleets "71306.0" and "100,000" without inventing a mile from junk text.
func emptyCell(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || s == "-"
}

func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" || s == "-" {
		return 0, fmt.Errorf("empty int")
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return 0, err
		}
		return int(f), nil
	}
	return n, nil
}

func parseOptFloat(s string) *float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" || s == "-" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// formatPDI builds an opaque id. Region is not encoded here so VA/CT cannot leak into the primary key.
func formatPDI(seq int) string {
	return fmt.Sprintf("PDI-%04d", seq)
}

// regionFromNickname takes leading letters (VA18 -> VA, WNY8 -> WNY). Plate state is the wrong region.
func regionFromNickname(nick string) string {
	var b strings.Builder
	for _, r := range nick {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			b.WriteRune(r)
			continue
		}
		break
	}
	return strings.ToUpper(b.String())
}

func stableID(prefix string, parts ...string) string {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(p))))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("%s-%x", prefix, h.Sum64())
}

// mapVals flattens de-duplicated stations/cards so upsert is one row per merchant.
func mapVals[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
