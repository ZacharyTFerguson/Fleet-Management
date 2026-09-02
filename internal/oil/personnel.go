package oil

import (
	"strings"
	"unicode"
)

// logisticsNames are first/last tokens that identify PDI logistics personnel on a punch or GPS label.
// They are never vehicle owners and never GPS installers, so they must not create a device↔car link.
var logisticsNames = map[string]struct{}{
	"rich":  {},
	"tyler": {},
}

// HasLogisticsPersonnel reports a logistics-personnel name in free text so pairing code can refuse the link.
func HasLogisticsPersonnel(parts ...string) bool {
	for _, p := range parts {
		for _, tok := range splitNameTokens(p) {
			if _, ok := logisticsNames[tok]; ok {
				return true
			}
		}
	}
	return false
}

// splitNameTokens breaks a label into lowercase words so "Rich / Tyler" and "TYLER-BOX" both match.
func splitNameTokens(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}
