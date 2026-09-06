package app

import (
	"context"
	"fmt"
	"strings"

	"oilchange/internal/config"
	"oilchange/internal/vault"
)

// SecretView is a masked field. Value is never included after save.
type SecretView struct {
	vault.Record
}

func (a *App) vaultGet(ctx context.Context, key string) (string, string, error) {
	if a.Vault == nil || a.Store == nil {
		return "", "", fmt.Errorf("vault not available")
	}
	row, err := a.Store.GetVaultSecret(ctx, key)
	if err != nil || row == nil {
		return "", "", err
	}
	plain, err := a.Vault.Decrypt(row.Nonce, row.Ciphertext)
	if err != nil {
		return "", "", err
	}
	return plain, row.UpdatedAt, nil
}

func envForSecret(cfg config.Config, key string) string {
	switch key {
	case "onestep_api_key":
		return cfg.OneStepToken
	case "onestep_pem":
		return cfg.OneStepPrivateKey
	case "efleets_username":
		return cfg.EFleetsUser
	case "efleets_password":
		return cfg.EFleetsPass
	case "efleets_cust_num":
		return cfg.EFleetsCust
	case "neon_database_url":
		return cfg.DatabaseURL
	case "supabase_url":
		return cfg.SupabaseURL
	case "supabase_anon_key":
		return cfg.SupabaseAnonKey
	case "supabase_service_role":
		return cfg.ServiceRole
	case "supabase_sync_secret":
		return cfg.SyncSecret
	default:
		return ""
	}
}

func applySecret(cfg *config.Config, key, val string) {
	switch key {
	case "onestep_api_key":
		cfg.OneStepToken = val
	case "onestep_pem":
		cfg.OneStepPrivateKey = val
	case "efleets_username":
		cfg.EFleetsUser = val
	case "efleets_password":
		cfg.EFleetsPass = val
	case "efleets_cust_num":
		cfg.EFleetsCust = val
	case "neon_database_url":
		cfg.DatabaseURL = val
	case "supabase_url":
		cfg.SupabaseURL = val
	case "supabase_anon_key":
		cfg.SupabaseAnonKey = val
	case "supabase_service_role":
		cfg.ServiceRole = val
	case "supabase_sync_secret":
		cfg.SyncSecret = val
	}
}

// OverlayVault fills empty Config fields from the encrypted store. Env still wins.
func (a *App) OverlayVault(ctx context.Context) error {
	if a.Vault == nil || a.Store == nil {
		return nil
	}
	rows, err := a.Store.ListVaultKeys(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if envForSecret(a.Cfg, row.Key) != "" {
			continue
		}
		plain, err := a.Vault.Decrypt(row.Nonce, row.Ciphertext)
		if err != nil {
			continue
		}
		applySecret(&a.Cfg, row.Key, plain)
	}
	return nil
}

func (a *App) ListSecrets(ctx context.Context) ([]SecretView, error) {
	out := make([]SecretView, 0, len(vault.Fields))
	for _, f := range vault.Fields {
		rec := vault.Record{Key: f.Key, Label: f.Label, Kind: f.Kind, Hint: f.Hint}
		if ev := envForSecret(a.Cfg, f.Key); ev != "" {
			rec.Set = true
			rec.Bytes = len(ev)
			rec.Mask = vault.Mask(ev)
			rec.Source = "environment"
		}
		if a.Store != nil && a.Vault != nil {
			plain, updated, err := a.vaultGet(ctx, f.Key)
			if err == nil && plain != "" {
				if !rec.Set {
					rec.Set = true
					rec.Bytes = len(plain)
					rec.Mask = vault.Mask(plain)
					rec.Source = "vault"
				} else if rec.Source == "environment" {
					rec.Source = "environment"
				}
				rec.UpdatedAt = updated
			}
		}
		if f.Key == "geocoder_api_key" || f.Key == "geocoder_provider" || f.Key == "onestep_username" || f.Key == "onestep_password" {
			if a.Store != nil && a.Vault != nil {
				plain, updated, err := a.vaultGet(ctx, f.Key)
				if err == nil && plain != "" {
					rec.Set = true
					rec.Bytes = len(plain)
					rec.Mask = vault.Mask(plain)
					rec.Source = "vault"
					rec.UpdatedAt = updated
				}
			}
		}
		out = append(out, SecretView{Record: rec})
	}
	return out, nil
}

func (a *App) PutSecret(ctx context.Context, key, value string) (SecretView, error) {
	if !vault.Known(key) {
		return SecretView{}, fmt.Errorf("unknown secret key")
	}
	if a.Vault == nil || a.Store == nil {
		return SecretView{}, fmt.Errorf("vault not available — set OILCHANGE_DB")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		if err := a.Store.DeleteVaultSecret(ctx, key); err != nil {
			return SecretView{}, err
		}
		return SecretView{Record: vault.Record{Key: key, Set: false}}, nil
	}
	nonce, ct, err := a.Vault.Encrypt(value)
	if err != nil {
		return SecretView{}, err
	}
	if err := a.Store.UpsertVaultSecret(ctx, key, nonce, ct); err != nil {
		return SecretView{}, err
	}
	applySecret(&a.Cfg, key, value)
	return SecretView{Record: vault.Record{
		Key:    key,
		Set:    true,
		Bytes:  len(value),
		Mask:   vault.Mask(value),
		Source: "vault",
	}}, nil
}

func (a *App) SecretPlain(ctx context.Context, key string) string {
	if a == nil {
		return ""
	}
	if ev := envForSecret(a.Cfg, key); ev != "" {
		return ev
	}
	plain, _, _ := a.vaultGet(ctx, key)
	return plain
}
