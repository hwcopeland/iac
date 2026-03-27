package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/hwcopeland/iac/kai/internal/db"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
)

type contextKey string

// UserContextKey is the context key under which the authenticated user is stored.
const UserContextKey contextKey = "user"

// SessionMiddleware validates the kai_session cookie on every request.
// On success it injects the authenticated user into the request context.
// On failure it returns 401 Unauthorized.
func SessionMiddleware(pool *db.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
