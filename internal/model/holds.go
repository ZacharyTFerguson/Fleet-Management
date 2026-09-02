package model

// HOLD codes. LOGISTICS_PERSONNEL replaces the third-spec name RICH_TYLER_PAIRING
// so operators see logistics-personnel, not personal names, while keeping the same skip-write rule.
const (
	HoldUnusualY             = "UNUSUAL_Y"
	HoldOdoBackward          = "ODO_BACKWARD"
	HoldCardMix              = "CARD_MIX"
	HoldNoDevice             = "NO_DEVICE"
	HoldDeviceAmbiguous      = "DEVICE_AMBIGUOUS"
	HoldMultiDeviceFight     = "MULTI_DEVICE_FIGHT"
	HoldNoTrustedFill        = "NO_TRUSTED_FILL"
	HoldNoDriveStop          = "NO_DRIVESTOP"
	HoldLowerReadingRefused  = "LOWER_READING_REFUSED"
	HoldSameSecondFill       = "SAME_SECOND_FILL"
	HoldSpikeAbandoned       = "SPIKE_ABANDONED"
	HoldLogisticsPersonnel   = "LOGISTICS_PERSONNEL"
)

// SourceFuelDetails and SourceShopRO are the only legal last_reading_source values.
const (
	SourceFuelDetails = "fuel_details"
	SourceShopRO      = "shop_ro"
	DefaultInterval   = 5000
	ExitOK            = 0
	ExitError         = 1
	ExitHolds         = 2
)
