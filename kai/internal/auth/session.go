package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	// CookieName is the HttpOnly session cookie name.
	CookieName = "kai_session"
	// SessionDuration is the default session lifetime when no config override is present.
	SessionDuration = 24 * time.Hour
)

// GenerateSessionToken returns a cryptographically random 32-byte session token
// (hex-encoded) and its SHA-256 hash (hex-encoded) for storage in the database.
// The raw token is placed in the cookie; only the hash is persisted.
func GenerateSessionToken() (token string, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = hex.EncodeToString(b)
	h := sha256.Sum256(b)
	hash = hex.EncodeToString(h[:])
	return
}

// HashToken derives the stored hash from a raw hex-encoded session token.
// Used on every authenticated request to look up the session.
func HashToken(token string) string {
	b, err := hex.DecodeString(token)
	if err != nil {
		// If the token isn't valid hex it can't match anything in the DB;
		// return a deterministic hash so the DB lookup simply finds nothing.
		h := sha256.Sum256([]byte(token))
		return hex.EncodeToString(h[:])
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// SetSessionCookie writes the kai_session HttpOnly cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie expires the kai_session cookie immediately.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
