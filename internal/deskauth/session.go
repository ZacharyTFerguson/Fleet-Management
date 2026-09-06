// Package deskauth is the Oil Desk operator login. Passwords are bcrypt hashes in sqlite.
// Session is an HMAC cookie. Secrets and OneStep send require a valid session.
package deskauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	CookieName = "oilchange_desk"
	maxAge     = 12 * time.Hour
	cost       = bcrypt.DefaultCost
)

// Manager signs cookies. The key never goes to the client.
type Manager struct {
	key []byte
}

func NewManager(key []byte) *Manager {
	if len(key) == 0 {
		sum := sha256.Sum256([]byte("oilchange-desk-dev"))
		key = sum[:]
	}
	if hexKey := strings.TrimSpace(os.Getenv("OILCHANGE_SESSION_KEY")); hexKey != "" {
		if raw, err := hex.DecodeString(hexKey); err == nil && len(raw) >= 16 {
			key = raw
		} else {
			sum := sha256.Sum256([]byte(hexKey))
			key = sum[:]
		}
	}
	return &Manager{key: key}
}

// HashPassword is bcrypt. Never store plaintext.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), cost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func (m *Manager) Sign(username string, exp time.Time) string {
	payload := username + "|" + strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Manager) Verify(tok string) (username string, ok bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write(raw)
	if subtle.ConstantTimeCompare(mac.Sum(nil), want) != 1 {
		return "", false
	}
	user, ts, ok := strings.Cut(string(raw), "|")
	if !ok || user == "" {
		return "", false
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Now().Unix() > unix {
		return "", false
	}
	return user, true
}

func (m *Manager) SetCookie(w http.ResponseWriter, r *http.Request, username string) {
	exp := time.Now().Add(maxAge)
	c := &http.Cookie{
		Name:     CookieName,
		Value:    m.Sign(username, exp),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(maxAge.Seconds()),
		Secure:   r != nil && (r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")),
	}
	http.SetCookie(w, c)
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (m *Manager) UserFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	return m.Verify(c.Value)
}

// RandomConfirm is a one-shot send gate. It is not a secret credential.
func RandomConfirm() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func EnvDeskUser() (user, pass string) {
	return strings.TrimSpace(os.Getenv("DESK_USERNAME")), strings.TrimSpace(os.Getenv("DESK_PASSWORD"))
}

func ValidateUsername(s string) error {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 {
		return fmt.Errorf("username required")
	}
	for _, r := range s {
		if r == '|' || r == '\n' || r == '\r' {
			return fmt.Errorf("invalid username")
		}
	}
	return nil
}
