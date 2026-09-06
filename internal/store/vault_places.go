package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"oilchange/internal/places"
)

type VaultRow struct {
	Key        string
	Nonce      []byte
	Ciphertext []byte
	UpdatedAt  string
}

func (s *Store) UpsertVaultSecret(ctx context.Context, key string, nonce, ct []byte) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `INSERT INTO vault_secrets (key, nonce, ciphertext, updated_at) VALUES (?,?,?,?)
		ON CONFLICT (key) DO UPDATE SET nonce=excluded.nonce, ciphertext=excluded.ciphertext, updated_at=excluded.updated_at`,
		key, nonce, ct, now)
	return err
}

func (s *Store) GetVaultSecret(ctx context.Context, key string) (*VaultRow, error) {
	row := s.queryRow(ctx, `SELECT key, nonce, ciphertext, COALESCE(updated_at,'') FROM vault_secrets WHERE key=?`, key)
	var r VaultRow
	if err := row.Scan(&r.Key, &r.Nonce, &r.Ciphertext, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListVaultKeys(ctx context.Context) ([]VaultRow, error) {
	rows, err := s.query(ctx, `SELECT key, nonce, ciphertext, COALESCE(updated_at,'') FROM vault_secrets ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VaultRow
	for rows.Next() {
		var r VaultRow
		if err := rows.Scan(&r.Key, &r.Nonce, &r.Ciphertext, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteVaultSecret(ctx context.Context, key string) error {
	_, err := s.exec(ctx, `DELETE FROM vault_secrets WHERE key=?`, key)
	return err
}

type DeskUser struct {
	Username     string
	PasswordHash string
	CreatedAt    string
	LastLoginAt  string
}

func (s *Store) CountDeskUsers(ctx context.Context) (int, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM desk_users`).Scan(&n)
	return n, err
}

func (s *Store) GetDeskUser(ctx context.Context, username string) (*DeskUser, error) {
	row := s.queryRow(ctx, `SELECT username, password_hash, COALESCE(created_at,''), COALESCE(last_login_at,'') FROM desk_users WHERE username=?`, username)
	var u DeskUser
	if err := row.Scan(&u.Username, &u.PasswordHash, &u.CreatedAt, &u.LastLoginAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) CreateDeskUser(ctx context.Context, username, hash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `INSERT INTO desk_users (username, password_hash, created_at) VALUES (?,?,?)`, username, hash, now)
	return err
}

func (s *Store) TouchDeskLogin(ctx context.Context, username string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `UPDATE desk_users SET last_login_at=? WHERE username=?`, now, username)
	return err
}

func (s *Store) InsertHeartbeat(ctx context.Context, target string, ok bool, detail string) error {
	flag := 0
	if ok {
		flag = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `INSERT INTO desk_heartbeats (target, ok, detail, at) VALUES (?,?,?,?)`, target, flag, detail, now)
	return err
}

func (s *Store) CountTable(ctx context.Context, name string) (int, error) {
	switch name {
	case "cars", "gas_stations", "places", "gas_station_marker_jobs", "desk_users", "vault_secrets", "mileage_ledger", "drive_stop_windows":
	default:
		return 0, fmt.Errorf("refusing count of %s", name)
	}
	var n int
	err := s.queryRow(ctx, "SELECT COUNT(*) FROM "+name).Scan(&n)
	return n, err
}

func (s *Store) LastGeneralCode(ctx context.Context) (string, error) {
	var code sql.NullString
	err := s.queryRow(ctx, `SELECT MAX(general_code) FROM places`).Scan(&code)
	if err != nil {
		return "", err
	}
	if !code.Valid {
		return "", nil
	}
	return code.String, nil
}

func (s *Store) UpsertPlace(ctx context.Context, p places.Place) error {
	if err := places.GasOnly(p.TypeCode); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `INSERT INTO places (general_code, type_code, brand_code, toptier, toptier_grade, label, name, address, merchant_id, lat, lng, onestep_marker_id, onestep_zone_id, hold_reason, source, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (general_code) DO UPDATE SET
			brand_code=excluded.brand_code, toptier=excluded.toptier, toptier_grade=excluded.toptier_grade,
			label=excluded.label, name=excluded.name, address=excluded.address, merchant_id=excluded.merchant_id,
			lat=COALESCE(excluded.lat, places.lat), lng=COALESCE(excluded.lng, places.lng),
			onestep_marker_id=COALESCE(excluded.onestep_marker_id, places.onestep_marker_id),
			onestep_zone_id=COALESCE(excluded.onestep_zone_id, places.onestep_zone_id),
			hold_reason=excluded.hold_reason, source=excluded.source, updated_at=excluded.updated_at`,
		p.GeneralCode, p.TypeCode, p.BrandCode, p.TopTier, p.TopTierGrade, p.Label, p.Name, p.Address, p.MerchantID,
		anyFloatPtr(p.Lat), anyFloatPtr(p.Lng), emptyNil(p.OneStepMarkerID), emptyNil(p.OneStepZoneID), emptyNil(p.HoldReason), p.Source, now, now)
	return err
}

func anyFloatPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func emptyNil(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func (s *Store) ListPlaces(ctx context.Context) ([]places.Place, error) {
	rows, err := s.query(ctx, `SELECT general_code, type_code, brand_code, toptier, toptier_grade, label, COALESCE(name,''), COALESCE(address,''), COALESCE(merchant_id,''), lat, lng, COALESCE(onestep_marker_id,''), COALESCE(onestep_zone_id,''), COALESCE(hold_reason,''), COALESCE(source,'')
		FROM places WHERE type_code=? ORDER BY general_code`, places.TypeGas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []places.Place
	for rows.Next() {
		var p places.Place
		var lat, lng sql.NullFloat64
		if err := rows.Scan(&p.GeneralCode, &p.TypeCode, &p.BrandCode, &p.TopTier, &p.TopTierGrade, &p.Label, &p.Name, &p.Address, &p.MerchantID, &lat, &lng, &p.OneStepMarkerID, &p.OneStepZoneID, &p.HoldReason, &p.Source); err != nil {
			return nil, err
		}
		if lat.Valid {
			v := lat.Float64
			p.Lat = &v
		}
		if lng.Valid {
			v := lng.Float64
			p.Lng = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPlace(ctx context.Context, code string) (*places.Place, error) {
	all, err := s.ListPlaces(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].GeneralCode == code {
			return &all[i], nil
		}
	}
	return nil, nil
}

type MarkerJob struct {
	ID                 string          `json:"id"`
	GeneralCode        string          `json:"general_code"`
	Stage              string          `json:"stage"`
	Draft              places.Draft    `json:"draft"`
	ReviewNotes        string          `json:"review_notes,omitempty"`
	ThirdPartyLat      *float64        `json:"third_party_lat,omitempty"`
	ThirdPartyLng      *float64        `json:"third_party_lng,omitempty"`
	ThirdPartyProvider string          `json:"third_party_provider,omitempty"`
	OneStepLat         *float64        `json:"onestep_lat,omitempty"`
	OneStepLng         *float64        `json:"onestep_lng,omitempty"`
	HasConfirm         bool            `json:"has_confirm"`
	LastError          string          `json:"last_error,omitempty"`
	CreatedAt          string          `json:"created_at,omitempty"`
	UpdatedAt          string          `json:"updated_at,omitempty"`
	MapURL             string          `json:"map_url,omitempty"`
}

func (s *Store) UpsertMarkerJob(ctx context.Context, id, code, stage, draftJSON, notes string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `INSERT INTO gas_station_marker_jobs (id, general_code, stage, draft_json, review_notes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT (id) DO UPDATE SET stage=excluded.stage, draft_json=excluded.draft_json, review_notes=excluded.review_notes, updated_at=excluded.updated_at`,
		id, code, stage, draftJSON, notes, now, now)
	return err
}

func (s *Store) UpdateMarkerJob(ctx context.Context, j MarkerJob, draftJSON, confirm string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.exec(ctx, `UPDATE gas_station_marker_jobs SET stage=?, draft_json=?, review_notes=?, third_party_lat=?, third_party_lng=?, third_party_provider=?, onestep_lat=?, onestep_lng=?, confirm_token=?, last_error=?, updated_at=? WHERE id=?`,
		j.Stage, draftJSON, j.ReviewNotes, anyFloatPtr(j.ThirdPartyLat), anyFloatPtr(j.ThirdPartyLng), emptyNil(j.ThirdPartyProvider),
		anyFloatPtr(j.OneStepLat), anyFloatPtr(j.OneStepLng), emptyNil(confirm), emptyNil(j.LastError), now, j.ID)
	return err
}

func (s *Store) GetMarkerJob(ctx context.Context, id string) (*MarkerJob, string, error) {
	row := s.queryRow(ctx, `SELECT id, general_code, stage, draft_json, COALESCE(review_notes,''), third_party_lat, third_party_lng, COALESCE(third_party_provider,''), onestep_lat, onestep_lng, COALESCE(confirm_token,''), COALESCE(last_error,''), COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM gas_station_marker_jobs WHERE id=?`, id)
	var j MarkerJob
	var draft, confirm string
	var tlat, tlng, olat, olng sql.NullFloat64
	if err := row.Scan(&j.ID, &j.GeneralCode, &j.Stage, &draft, &j.ReviewNotes, &tlat, &tlng, &j.ThirdPartyProvider, &olat, &olng, &confirm, &j.LastError, &j.CreatedAt, &j.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, "", nil
		}
		return nil, "", err
	}
	_ = json.Unmarshal([]byte(draft), &j.Draft)
	if tlat.Valid {
		v := tlat.Float64
		j.ThirdPartyLat = &v
	}
	if tlng.Valid {
		v := tlng.Float64
		j.ThirdPartyLng = &v
	}
	if olat.Valid {
		v := olat.Float64
		j.OneStepLat = &v
	}
	if olng.Valid {
		v := olng.Float64
		j.OneStepLng = &v
	}
	j.HasConfirm = confirm != ""
	if j.ThirdPartyLat != nil && j.ThirdPartyLng != nil {
		j.MapURL = places.OSMEmbed(*j.ThirdPartyLat, *j.ThirdPartyLng)
	} else if j.Draft.Lat != nil && j.Draft.Lng != nil {
		j.MapURL = places.OSMEmbed(*j.Draft.Lat, *j.Draft.Lng)
	}
	return &j, confirm, nil
}

func (s *Store) ListMarkerJobs(ctx context.Context) ([]MarkerJob, error) {
	rows, err := s.query(ctx, `SELECT id FROM gas_station_marker_jobs ORDER BY general_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []MarkerJob
	for _, id := range ids {
		j, _, err := s.GetMarkerJob(ctx, id)
		if err != nil {
			return nil, err
		}
		if j != nil {
			out = append(out, *j)
		}
	}
	return out, nil
}

func (s *Store) MarkerJobByCode(ctx context.Context, code string) (*MarkerJob, error) {
	var id string
	err := s.queryRow(ctx, `SELECT id FROM gas_station_marker_jobs WHERE general_code=?`, code).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j, _, err := s.GetMarkerJob(ctx, id)
	return j, err
}
