package onestep

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"
)

// signAPIKeyJWT wraps the API key in a short-lived RS256 JWT (OneStep usage note).
// Claim is access_token; exp is at most a few minutes; a fresh token is signed per request.
func signAPIKeyJWT(privatePEM, apiKey string, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > 5*time.Minute {
		ttl = time.Minute
	}
	key, err := parseRSAPrivateKey([]byte(privatePEM))
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{
		"access_token": apiKey,
		"exp":          time.Now().Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("onestep jwt sign: %w", err)
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// parseRSAPrivateKey accepts PKCS#8 (BEGIN PRIVATE KEY) or PKCS#1 RSA PEMs from OneStep's note.
func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("onestep private key: no PEM block")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("onestep private key: not RSA")
		}
		return rsaKey, nil
	}
	k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("onestep private key: %w", err)
	}
	return k, nil
}
