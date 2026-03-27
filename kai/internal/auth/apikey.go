package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// GenerateAPIKey creates a new crypto-random API key.
// Returns rawKey (shown to the user once), keyHash (stored in DB), and
// keyPrefix (the first 8 chars of rawKey — always visible in UI).
//
// Format: "kai_" + base64url(32 random bytes, no padding)
// The key is 256-bit random, so SHA-256 (not bcrypt) is appropriate per the schema notes.
func GenerateAPIKey() (rawKey, keyHash, keyPrefix string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	rawKey = "kai_" + base64.RawURLEncoding.EncodeToString(b)
	keyHash = HashAPIKey(rawKey)
	keyPrefix = rawKey[:8]
	return
}

// HashAPIKey returns hex(sha256(rawKey)).
// Used both when storing a newly created key and when validating an incoming Bearer token.
func HashAPIKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
