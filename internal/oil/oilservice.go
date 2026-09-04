package oil

import "strings"

// oilNeedles are eFleets Service Desc fragments that mean an oil change happened.
// Last oil comes from these maintenance records, not from fuel punches or OneStep.
var oilNeedles = []string{
	"lube oil filter",
	"full synthetic engine oil",
	"semi synthetic engine oil",
	"semi-synthetic engine oil",
	"synthetic lube",
	"oil change",
	"conventional lube oil",
	"conventional engine oil",
}

// oilExcludes are maintenance rows that mention oil but are not an oil change.
var oilExcludes = []string{
	"oil pan",
	"oil leak",
	"oil pressure",
	"oil light",
	"oil cooler",
	"drain plug",
	"oil filter surcharge",
	"oil surcharge",
	"refrigerant oil",
	"camshaft oil",
	"oil temperature",
	"oil feed",
	"lube chassis",
}

// IsOilChangeService reports whether a shop RO line is an oil change we may seed last_oil_* from.
func IsOilChangeService(desc string) bool {
	d := strings.ToLower(strings.TrimSpace(desc))
	if d == "" {
		return false
	}
	for _, ex := range oilExcludes {
		if strings.Contains(d, ex) {
			return false
		}
	}
	for _, n := range oilNeedles {
		if strings.Contains(d, n) {
			return true
		}
	}
	return false
}
