package queries

import (
	"context"
	"time"

	"github.com/hwcopeland/iac/kai/internal/db"
)

// APIKey is the public projection of the api_keys table.
// key_hash is intentionally excluded — it must never be returned to callers.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKey inserts a new api_key row and returns its UUID.
// keyHash must be hex(sha256(rawKey)); keyPrefix must be rawKey[:8].
func CreateAPIKey(ctx context.Context, pool *db.Pool, userID, name, keyHash, keyPrefix string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO api_keys (user_id, name, key_hash, key_prefix)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, name, keyHash, keyPrefix).Scan(&id)
	return id, err
}

// ListAPIKeys returns all API keys for a user ordered newest first.
// The key_hash column is never selected.
func ListAPIKeys(ctx context.Context, pool *db.Pool, userID string) ([]APIKey, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, key_prefix, created_at, last_used_at, expires_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(
			&k.ID, &k.Name, &k.KeyPrefix,
			&k.CreatedAt, &k.LastUsedAt, &k.ExpiresAt,
		); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey deletes an API key by ID, verifying it belongs to userID.
// The delete is scoped to (id, user_id) so a user can only revoke their own keys.
// If the key does not exist or belongs to another user, no rows are deleted
// and nil is returned (idempotent delete).
func RevokeAPIKey(ctx context.Context, pool *db.Pool, keyID, userID string) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM api_keys WHERE id = $1 AND user_id = $2
	`, keyID, userID)
	return err
}

// ValidateAPIKey looks up a key by its hash, updates last_used_at, and returns
// the associated user_id.
// Returns pgx.ErrNoRows (via Scan) if the key is not found or has expired.
func ValidateAPIKey(ctx context.Context, pool *db.Pool, keyHash string) (string, error) {
	var userID string
	err := pool.QueryRow(ctx, `
		UPDATE api_keys
		SET last_used_at = NOW()
		WHERE key_hash = $1
		  AND (expires_at IS NULL OR expires_at > NOW())
		RETURNING user_id
	`, keyHash).Scan(&userID)
	return userID, err
}
