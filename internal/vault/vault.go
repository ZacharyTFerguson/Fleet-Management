// Package vault encrypts desk secrets with AES-256-GCM.
// Plaintext never returns on list/get after save — only presence, byte length, and a short mask.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Field is one secret the Secrets page can store. Values stay server-side.
type Field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // password | text | textarea
	Hint  string `json:"hint,omitempty"`
}

// Fields is the locked catalog. Do not add customer numbers as hardcoded defaults.
var Fields = []Field{
	{Key: "onestep_api_key", Label: "OneStep API key", Kind: "password", Hint: "Public v3 api-key. Aliases: ONESTEP_API_KEY / OneStepAPIKEYTobeSigned."},
	{Key: "onestep_username", Label: "OneStep portal username", Kind: "text", Hint: "track.onestepgps.com login. Portal-only; not a substitute for the API key."},
	{Key: "onestep_password", Label: "OneStep portal password", Kind: "password"},
	{Key: "onestep_pem", Label: "OneStep JWT PEM (RS256)", Kind: "textarea", Hint: "Optional PKCS#8 private key. Public API usually needs the API key only."},
	{Key: "efleets_username", Label: "eFleets username", Kind: "text"},
	{Key: "efleets_password", Label: "eFleets password", Kind: "password"},
	{Key: "efleets_cust_num", Label: "eFleets customer number", Kind: "password"},
	{Key: "neon_database_url", Label: "Neon unpooled DATABASE_URL", Kind: "password", Hint: "Fleet_Manage_Oil, hostname without -pooler. Backup only."},
	{Key: "supabase_url", Label: "Supabase URL", Kind: "text", Hint: "ZacharyTFerguson's Project. Never XRAY."},
	{Key: "supabase_anon_key", Label: "Supabase publishable / anon key", Kind: "password"},
	{Key: "supabase_service_role", Label: "Supabase service role", Kind: "password", Hint: "Server-side fleet_cars upsert only. Never NEXT_PUBLIC_*."},
	{Key: "supabase_sync_secret", Label: "Supabase fleet-sync secret", Kind: "password"},
	{Key: "geocoder_api_key", Label: "Geocoder API key", Kind: "password", Hint: "Mapbox or Google. Nominatim (default) needs none."},
	{Key: "geocoder_provider", Label: "Geocoder provider", Kind: "text", Hint: "nominatim | mapbox | google"},
}

// Record is what the UI may see. Value is never the plaintext.
type Record struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Hint      string `json:"hint,omitempty"`
	Set       bool   `json:"set"`
	Bytes     int    `json:"bytes,omitempty"`
	Mask      string `json:"mask,omitempty"`
	Source    string `json:"source,omitempty"` // environment | vault
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Box holds the AES key. The key file is gitignored (data/runtime/vault.key).
type Box struct {
	key []byte
	mu  sync.Mutex
}

// Open loads OILCHANGE_VAULT_KEY or a 32-byte key file, generating the file if missing.
func Open(path string) (*Box, error) {
	if hexKey := strings.TrimSpace(os.Getenv("OILCHANGE_VAULT_KEY")); hexKey != "" {
		raw, err := decodeKey(hexKey)
		if err != nil {
			return nil, err
		}
		return &Box{key: raw}, nil
	}
	if path == "" {
		path = filepath.Join("data", "runtime", "vault.key")
	}
	if b, err := os.ReadFile(path); err == nil {
		raw, err := decodeKey(strings.TrimSpace(string(b)))
		if err != nil {
			return nil, fmt.Errorf("vault key file: %w", err)
		}
		return &Box{key: raw}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(raw)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return &Box{key: raw}, nil
}

func decodeKey(s string) ([]byte, error) {
	if raw, err := hex.DecodeString(s); err == nil && len(raw) == 32 {
		return raw, nil
	}
	sum := sha256.Sum256([]byte(s))
	return sum[:], nil
}

// Encrypt returns nonce + ciphertext. Never log either.
func (b *Box) Encrypt(plain string) (nonce, ct []byte, err error) {
	if b == nil || len(b.key) != 32 {
		return nil, nil, fmt.Errorf("vault not open")
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return nonce, gcm.Seal(nil, nonce, []byte(plain), nil), nil
}

// Decrypt returns plaintext for in-process use (OneStep client, geocoder). Callers must not write it to HTTP.
func (b *Box) Decrypt(nonce, ct []byte) (string, error) {
	if b == nil || len(b.key) != 32 {
		return "", fmt.Errorf("vault not open")
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Mask never returns the full secret. Short values become bullets only.
func Mask(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "••••"
	}
	return "••••" + v[len(v)-4:]
}

// Known reports whether key is in the catalog.
func Known(key string) bool {
	for _, f := range Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}
