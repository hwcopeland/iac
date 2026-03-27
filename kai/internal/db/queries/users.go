package queries

import (
	"context"
	"time"

	"github.com/hwcopeland/iac/kai/internal/db"
)

// User mirrors the users table.
// ID is the Authentik sub claim (SHA-256 derived string, not a UUID).
// See docs/tdd/kai-data.md §3.1 for the rationale on TEXT primary key.
type User struct {
	ID          string
	Email       string
	DisplayName string
	AvatarURL   string
	IsAdmin     bool
	CreatedAt   time.Time
	LastLoginAt time.Time
}

// UpsertUser inserts a new user row or, on conflict with an existing id, updates
// the mutable profile fields and refreshes last_login_at.
// This is called on every successful OIDC login so the profile stays current.
func UpsertUser(ctx context.Context, pool *db.Pool, u User) (User, error) {
	row := pool.QueryRow(ctx, `
		INSERT INTO users (id, email, display_name, avatar_url, is_admin, last_login_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
		    email         = EXCLUDED.email,
		    display_name  = EXCLUDED.display_name,
		    avatar_url    = EXCLUDED.avatar_url,
		    is_admin      = EXCLUDED.is_admin,
		    last_login_at = NOW()
		RETURNING id, email, display_name, avatar_url, is_admin, created_at, last_login_at
	`, u.ID, u.Email, u.DisplayName, u.AvatarURL, u.IsAdmin)

	var out User
	err := row.Scan(
		&out.ID,
		&out.Email,
		&out.DisplayName,
		&out.AvatarURL,
		&out.IsAdmin,
		&out.CreatedAt,
		&out.LastLoginAt,
	)
	return out, err
}

// GetUserByID fetches a single user by primary key.
// Returns pgx.ErrNoRows if not found.
func GetUserByID(ctx context.Context, pool *db.Pool, id string) (User, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, email, display_name, avatar_url, is_admin, created_at, last_login_at
		FROM users
		WHERE id = $1
	`, id)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.DisplayName,
		&u.AvatarURL,
		&u.IsAdmin,
		&u.CreatedAt,
		&u.LastLoginAt,
	)
	return u, err
}
