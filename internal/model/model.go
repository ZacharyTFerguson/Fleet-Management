package model

import "time"

// Car is one PDI unit. EFleetsID is the only join key; display_name is never a join key.
// PDIID is opaque (PDI-0042) and must not embed region.
type Car struct {
	PDIID              string
	EFleetsID          string
	Nickname           string
	Plate              string
	VIN                string
	Region             string
	LastOilMiles       *int
	LastOilDate        *time.Time
	LastReadingMiles   *int
	LastReadingAt      *time.Time
	LastReadingSource  *string // "fuel_details" | "shop_ro"
	HoldReason         *string
	IntervalMiles      int // 0 => 5000
}

// Hold is a skip-the-write reason. Operators must not see a Last Reading beside it.
type Hold struct {
	Code   string
	Detail string
}

// Card is a fuel card. linked_car_efleets_id is NULLed on CARD_MIX.
type Card struct {
	ID                    string
	CompanyVehicleNumber  string
	LinkedCarEFleetsID    *string
	Notes                 string
}

// GasStation is upserted from DETAILS merchant name/address for later matching, not Last Reading math.
type GasStation struct {
	ID         string
	Name       string
	Address    string
	MerchantID string
	Lat, Lng   *float64
}

// MaintenanceLocation is upserted from shop RO location. Real table, not a stub.
type MaintenanceLocation struct {
	ID      string
	Name    string
	Address string
	Notes   string
}

// Fill is one Fuel & Charging DETAILS punch. Trusted fill is the latest row that survives checks.
type Fill struct {
	EFleetsID                    string
	CardID                       string // Provider Card Number; empty if the export omitted it
	CardCompanyVehicleNumber     string
	ProviderCompanyVehicleNumber string
	Odometer                     *int
	UnusualY                     bool
	ProviderTransactionTime      time.Time // second precision; America/New_York if naive
	MerchantName                 string
	MerchantAddress              string
	Source                       string
	DriverFirst                  string
	DriverLast                   string
	Gallons                      *float64
	Amount                       *float64
	Plate                        string
}

// CardTx is one swipe in the card intelligence store (not last-write-wins on cards).
type CardTx struct {
	CardID            string
	At                time.Time
	StationName       string
	StationAddress    string
	Gallons           *float64
	Amount            *float64
	RecordedEFleetsID string
	RecordedCVN       string
	Plate             string
	DriverFirst       string
	DriverLast        string
	SourceRow         string
	Odometer          *int
}

// StopVisit is one GPS stop (car sitting still). Card matching uses the time
// window; Last Reading must not read this.
type StopVisit struct {
	EFleetsID string    `json:"efleets_id"`
	FactoryID string    `json:"factory_id"`
	DeviceID  string    `json:"device_id"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	HasPos    bool      `json:"has_pos"`
}

// GPSCardMatch is the card that swiped while a GPS-linked car was stopped.
type GPSCardMatch struct {
	EFleetsID      string   `json:"efleets_id"`
	CardID         string   `json:"card_id"`
	EvidenceN      int      `json:"evidence_n"`
	Stations       []string `json:"stations,omitempty"`
	EnterpriseCars []string `json:"enterprise_cars,omitempty"`
	Best           bool     `json:"best"`
}

// CardPairing is a scored car or person link for one card over the full history.
type CardPairing struct {
	CardID     string
	EntityType string // "car" or "person"
	EntityKey  string
	EvidenceN  int
	Score      float64
	Best       bool
}

// ShopRO is one repair order. Many line items share RO ID and one odometer.
type ShopRO struct {
	ROID         string
	EFleetsID    string
	Odometer     int
	At           time.Time
	LocationName string
	ServiceDesc  string
}

// OneStepDevice is a GPS box in the durable device registry.
// Pair by factory_id only. device_id is history identity. display_name is label only (never a join key).
type OneStepDevice struct {
	FactoryID          string
	DeviceID           string
	DisplayName        string
	LinkedCarEFleetsID *string
	LinkedCarPDIID     *string
	Dead               bool
	Active             bool
	RetiredAt          *time.Time
	LastSyncedAt       *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// HoldEvent is a persisted open/closed HOLD. Open holds skip Last Reading writes.
type HoldEvent struct {
	EFleetsID string
	Reason    string
	Detail    string
	At        time.Time
	Open      bool
}

// OilChange is a completed oil service. oil-done and oil/lube shop ROs write these; they do not change Last Reading.
type OilChange struct {
	EFleetsID string
	Miles     int
	Date      time.Time
	Location  string
	Source    string
}

// DriveStopMiles is GPS trip sum after a known second. Never stores device odometer.
type DriveStopMiles struct {
	FactoryID string
	Since     time.Time
	Miles     float64
}

// MileageEntry is context only. internal/oil must not read it.
type MileageEntry struct {
	EFleetsID string
	At        time.Time
	Odometer  int
	Source    string
}
