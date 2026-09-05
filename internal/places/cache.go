package places

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// JSONPlace is the on-disk / Neon-export shape. No new codes are minted here.
type JSONPlace struct {
	GeneralCode      string `json:"general_code"`
	TypeCode         string `json:"type_code"`
	BrandCode        string `json:"brand_code"`
	TopTier          string `json:"toptier"`
	TopTierGrade     string `json:"toptier_grade"`
	Label            string `json:"label"`
	Name             string `json:"name"`
	Address          string `json:"address"`
	City             string `json:"city"`
	State            string `json:"state"`
	MerchantLocation string `json:"merchant_location"`
	MerchantID       string `json:"merchant_id"`
	SiteKey          string `json:"site_key"`
	Active           *bool  `json:"active"`
}

// FromJSONFile reads a catalog dump. Rows without general_code are skipped.
func FromJSONFile(path string) ([]Place, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw []JSONPlace
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("places json: %w", err)
	}
	return FromJSON(raw), nil
}

// FromJSON converts export rows. Empty general_code is dropped, never invented.
func FromJSON(raw []JSONPlace) []Place {
	out := make([]Place, 0, len(raw))
	for _, r := range raw {
		p := placeFromJSON(r)
		if strings.TrimSpace(p.GeneralCode) == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func placeFromJSON(r JSONPlace) Place {
	active := true
	if r.Active != nil {
		active = *r.Active
	}
	return Place{
		GeneralCode:      strings.TrimSpace(r.GeneralCode),
		TypeCode:         strings.TrimSpace(r.TypeCode),
		BrandCode:        strings.TrimSpace(r.BrandCode),
		TopTier:          firstNonEmpty(r.TopTier, "A"),
		TopTierGrade:     firstNonEmpty(r.TopTierGrade, "A"),
		Label:            strings.TrimSpace(r.Label),
		Name:             strings.TrimSpace(r.Name),
		Address:          strings.TrimSpace(r.Address),
		City:             strings.TrimSpace(r.City),
		State:            strings.TrimSpace(r.State),
		MerchantLocation: strings.TrimSpace(r.MerchantLocation),
		MerchantID:       strings.TrimSpace(r.MerchantID),
		SiteKey:          strings.TrimSpace(r.SiteKey),
		Active:           active,
	}
}

// CopyFromNeon SELECT-only copies Fleet_Manage_Oil.places. It does not INSERT/UPDATE Neon.
func CopyFromNeon(ctx context.Context, databaseURL string) ([]Place, error) {
	dsn := strings.TrimSpace(databaseURL)
	if dsn == "" {
		return nil, fmt.Errorf("places: empty Neon URL")
	}
	dsn = stripChannelBinding(dsn)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("places neon open: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("places neon ping: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT general_code::text, type_code::text, brand_code::text,
		toptier::text, toptier_grade::text, COALESCE(label,''), COALESCE(name,''), COALESCE(address,''),
		COALESCE(city,''), COALESCE(state,''), COALESCE(merchant_location,''), COALESCE(merchant_id,''),
		COALESCE(site_key,''), active
		FROM places`)
	if err != nil {
		return nil, fmt.Errorf("places neon select: %w", err)
	}
	defer rows.Close()
	var out []Place
	for rows.Next() {
		var p Place
		if err := rows.Scan(&p.GeneralCode, &p.TypeCode, &p.BrandCode, &p.TopTier, &p.TopTierGrade,
			&p.Label, &p.Name, &p.Address, &p.City, &p.State, &p.MerchantLocation, &p.MerchantID,
			&p.SiteKey, &p.Active); err != nil {
			return nil, err
		}
		p.GeneralCode = strings.TrimSpace(p.GeneralCode)
		if p.GeneralCode == "" {
			continue
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("places neon: 0 rows — refusing empty overwrite")
	}
	return out, nil
}

func stripChannelBinding(dsn string) string {
	// pgx rejects some Neon pooler URIs that require channel_binding.
	dsn = strings.ReplaceAll(dsn, "channel_binding=require&", "")
	dsn = strings.ReplaceAll(dsn, "&channel_binding=require", "")
	dsn = strings.ReplaceAll(dsn, "?channel_binding=require", "?")
	return dsn
}
