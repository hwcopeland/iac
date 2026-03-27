package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hwcopeland/iac/kai/internal/db"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
)

type contextKey string

// UserContextKey is the context key under which the authenticated user is stored.
const UserContextKey contextKey = "user"

// SessionMiddleware authenticates requests via two paths, checked in order:
//
//  1. Bearer token (API key): Authorization: Bearer kai_xxxx
//     The raw token is hashed (SHA-256 hex) and looked up in api_keys.
//     If valid the associated user is loaded and injected into context.
//     An invalid or expired Bearer token results in an immediate 401 —
//     the middleware does NOT fall through to the cookie path.
//
//  2. Session cookie: kai_session=<raw_token>
//     The raw token is hashed and looked up in sessions.
//     If valid the associated user is loaded and injected into context.
func SessionMiddleware(pool *db.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ── Path 1: Bearer API key ────────────────────────────────────────
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				const prefix = "Bearer "
				if !strings.HasPrefix(authHeader, prefix) {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				rawKey := strings.TrimPrefix(authHeader, prefix)
				keyHash := HashAPIKey(rawKey)

				userID, err := queries.ValidateAPIKey(r.Context(), pool, keyHash)
				if err != nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}

				user, err := queries.GetUserByID(r.Context(), pool, userID)
				if err != nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}

				ctx := context.WithValue(r.Context(), UserContextKey, user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// ── Path 2: Session cookie ────────────────────────────────────────
			cookie, err := r.Cookie(CookieName)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			hash := HashToken(cookie.Value)

			session, err := queries.GetSessionByTokenHash(r.Context(), pool, hash)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			user, err := queries.GetUserByID(r.Context(), pool, session.UserID)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Touch session last_seen_at asynchronously — fire and forget.
			go func() {
				if err := queries.TouchSession(context.Background(), pool, hash); err != nil {
					slog.Debug("touch session", "err", err)
				}
			}()

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext retrieves the authenticated user from the request context.
// Returns false if the middleware was not applied or authentication failed.
func UserFromContext(ctx context.Context) (queries.User, bool) {
	u, ok := ctx.Value(UserContextKey).(queries.User)
	return u, ok
}
