// Package queries provides typed database access functions for the Kai platform.
// The core domain types (User, Session) and their primary CRUD functions are
// defined in users.go and sessions.go respectively. This file adds helpers that
// are required by the auth middleware and handlers but were not part of the
// initial query scaffold.
package queries

import (
	"context"

	"github.com/hwcopeland/iac/kai/internal/db"
)

// TouchSession is a no-op in Phase 1 — sessions expire on their natural TTL.
//
// Phase 2 implementation: extend expires_at by the configured session duration
// on each authenticated request to implement a sliding-window session lifetime.
func TouchSession(_ context.Context, _ *db.Pool, _ string) error {
	return nil
}

// DeleteSessionByTokenHash is a named alias for DeleteSession provided for
// consistency with the "ByTokenHash" naming convention used in auth handlers.
func DeleteSessionByTokenHash(ctx context.Context, pool *db.Pool, tokenHash string) error {
	return DeleteSession(ctx, pool, tokenHash)
}
