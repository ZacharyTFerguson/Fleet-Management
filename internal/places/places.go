// Package places joins eFleets DETAILS punches to the Canon places catalog.
// Labels use locked 7_3_5_1_1: GeneralCode_Type_Branding_TopTier_TopTierGrade.
// Lookup never invents a general_code, street number, or Canon name.
package places

import (
	"strings"
	"unicode"
)

// UnmatchedLabel is the UI fallback when a punch does not hit the catalog.
const UnmatchedLabel = "unmatched"

const (
	MatchOK        = "ok"
	MatchUnmatched = "unmatched"
)

// Place is one catalog row. Prefer Label; otherwise build from the five parts.
type Place struct {
	GeneralCode      string
	TypeCode         string
	BrandCode        string
	TopTier          string
	TopTierGrade     string
	Label            string
	Name             string
	Address          string
	City             string
	State            string
	MerchantLocation string
	MerchantID       string
	SiteKey          string
	Active           bool
}

// CanonLabel is the stored composite when present, else the locked 7_3_5_1_1 build.
// Empty general_code is not a place — callers must not invent one.
func CanonLabel(p Place) string {
	if s := strings.TrimSpace(p.Label); s != "" {
		return s
	}
	g := strings.TrimSpace(p.GeneralCode)
	if g == "" {
		return ""
	}
	return strings.Join([]string{
		g,
		strings.TrimSpace(p.TypeCode),
		strings.TrimSpace(p.BrandCode),
		strings.TrimSpace(p.TopTier),
		strings.TrimSpace(p.TopTierGrade),
	}, "_")
}

// Hit is a catalog lookup result. Unmatched punches stay unmatched — never a fake A######.
type Hit struct {
	Matched     bool
	Label       string
	Match       string
	GeneralCode string
	SiteKey     string
	Place       Place
}

// Catalog is an in-memory index of existing places. It does not allocate codes.
type Catalog struct {
	bySite map[string]Place
	n      int
}

// NewCatalog indexes rows by stored site_key, or NAME|STREET|CITY|STATE when site_key is empty.
func NewCatalog(rows []Place) *Catalog {
	c := &Catalog{bySite: make(map[string]Place, len(rows))}
	for _, p := range rows {
		key := strings.TrimSpace(p.SiteKey)
		if key == "" {
			key = ComposeSiteKey(firstNonEmpty(p.MerchantLocation, p.Name), p.Address, p.City, p.State)
		} else {
			key = NormalizeSiteKey(key)
		}
		if key == "" || strings.TrimSpace(p.GeneralCode) == "" {
			continue
		}
		c.bySite[key] = p
		c.n++
	}
	return c
}

// Len is how many joinable catalog rows were indexed.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return c.n
}

// Lookup matches a DETAILS merchant + "street, city, ST" address to one catalog place.
func (c *Catalog) Lookup(merchant, address string) Hit {
	key := SiteKey(merchant, address)
	if key == "" || c == nil {
		return Hit{Matched: false, Label: UnmatchedLabel, Match: MatchUnmatched, SiteKey: key}
	}
	p, ok := c.bySite[key]
	if !ok {
		return Hit{Matched: false, Label: UnmatchedLabel, Match: MatchUnmatched, SiteKey: key}
	}
	label := CanonLabel(p)
	if label == "" {
		return Hit{Matched: false, Label: UnmatchedLabel, Match: MatchUnmatched, SiteKey: key}
	}
	return Hit{
		Matched:     true,
		Label:       label,
		Match:       MatchOK,
		GeneralCode: strings.TrimSpace(p.GeneralCode),
		SiteKey:     key,
		Place:       p,
	}
}

// Display is the Canon name, or unmatched when the punch cannot join.
func (c *Catalog) Display(merchant, address string) string {
	return c.Lookup(merchant, address).Label
}

// SiteKey is NAME|STREET|CITY|ST from a DETAILS punch. Empty when the punch
// has no real merchant or a parseable street/city/state (TRACKER / UNKNOWN / HOLD).
func SiteKey(merchant, address string) string {
	name := NormalizeToken(merchant)
	if skipMerchant(name) {
		return ""
	}
	street, city, state := SplitAddress(address)
	return ComposeSiteKey(name, street, city, state)
}

// ComposeSiteKey builds the catalog join key. Missing parts yield empty (no guess).
func ComposeSiteKey(name, street, city, state string) string {
	n := NormalizeToken(name)
	st := NormalizeToken(street)
	ci := NormalizeToken(city)
	ss := NormalizeState(state)
	if n == "" || st == "" || ci == "" || ss == "" {
		return ""
	}
	return n + "|" + st + "|" + ci + "|" + ss
}

// NormalizeSiteKey uppercases and collapses spaces on an existing catalog key.
func NormalizeSiteKey(key string) string {
	parts := strings.Split(key, "|")
	if len(parts) != 4 {
		return strings.ToUpper(collapseSpace(strings.TrimSpace(key)))
	}
	return ComposeSiteKey(parts[0], parts[1], parts[2], parts[3])
}

// SplitAddress parses DETAILS "STREET, CITY, ST" without inventing a street number.
func SplitAddress(addr string) (street, city, state string) {
	parts := splitComma(addr)
	if len(parts) < 3 {
		return "", "", ""
	}
	state = NormalizeState(parts[len(parts)-1])
	city = NormalizeToken(parts[len(parts)-2])
	street = NormalizeToken(strings.Join(parts[:len(parts)-2], " "))
	return street, city, state
}

func skipMerchant(name string) bool {
	switch name {
	case "", "TRACKER", "UNKNOWN", "N/A", "-":
		return true
	default:
		return false
	}
}

// NormalizeToken is uppercase collapsed whitespace. No street-number invention.
func NormalizeToken(s string) string {
	return strings.ToUpper(collapseSpace(strings.TrimSpace(s)))
}

// NormalizeState keeps the two-letter US code from a DETAILS state token.
func NormalizeState(s string) string {
	s = NormalizeToken(s)
	var letters []rune
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters = append(letters, r)
		}
	}
	if len(letters) < 2 {
		return ""
	}
	return string(letters[:2])
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func splitComma(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
