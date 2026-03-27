package queries

import (
	"context"
	"time"

	"github.com/hwcopeland/iac/kai/internal/db"
)

// Session mirrors the sessions table.
// TokenHash is SHA-256(raw_cookie_value), hex-encoded.
// The raw cookie value is never persisted; see docs/tdd/kai-data.md §3.2.
type Session struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	UserAgent string
	IPAddr    string
}

// CreateSession inserts a new session row for the given user.
// tokenHash must be SHA-256(raw_cookie_value) in hex.
func CreateSession(ctx context.Context, pool *db.Pool, userID, tokenHash, userAgent, ipAddr string, expiresAt time.Time) (Session, error) {
	row := pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, created_at, user_agent, ip_addr)
		VALUES ($1, $2, $3, NOW(), $4, $5)
		RETURNING id, user_id, token_hash, expires_at, created_at, user_agent, ip_addr
	`, userID, tokenHash, expiresAt, userAgent, ipAddr)

	var s Session
	err := row.Scan(
		&s.ID,
		&s.UserID,
		&s.TokenHash,
		&s.ExpiresAt,
		&s.CreatedAt,
		&s.UserAgent,
		&s.IPAddr,
	)
	return s, err
}

// GetSessionByTokenHash looks up an active (non-expired) session by token hash.
// Returns pgx.ErrNoRows if not found or already expired.
func GetSessionByTokenHash(ctx context.Context, pool *db.Pool, tokenHash string) (Session, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, created_at, user_agent, ip_addr
		FROM sessions
		WHERE token_hash = $1
		  AND expires_at > NOW()
	`, tokenHash)

	var s Session
	err := row.Scan(
		&s.ID,
		&s.UserID,
		&s.TokenHash,
		&s.ExpiresAt,
		&s.CreatedAt,
		&s.UserAgent,
		&s.IPAddr,
	)
	return s, err
}

// DeleteSession removes a session by token hash (used on logout).
func DeleteSession(ctx context.Context, pool *db.Pool, tokenHash string) error {
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// DeleteExpiredSessions removes all sessions whose expires_at is in the past.
// Intended for a periodic cleanup job.
func DeleteExpiredSessions(ctx context.Context, pool *db.Pool) error {
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	return err
}

// DeleteSessionsByUserID removes all sessions for a user (force-logout).
func DeleteSessionsByUserID(ctx context.Context, pool *db.Pool, userID string) error {
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}
