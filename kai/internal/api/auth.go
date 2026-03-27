package api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/hwcopeland/iac/kai/internal/auth"
	"github.com/hwcopeland/iac/kai/internal/db/queries"
)

const (
	// pkceVerifierCookie holds the PKCE code_verifier across the redirect.
	pkceVerifierCookie = "pkce_verifier"
	// oauthStateCookie holds the CSRF state nonce across the redirect.
	oauthStateCookie = "oauth_state"
	// tempCookieTTL is the lifetime of the short-lived PKCE/state cookies.
	tempCookieTTL = 10 * time.Minute
)

// setTempCookie writes a short-lived, HttpOnly, SameSite=Lax cookie used
// to carry PKCE verifier and state across the Authentik redirect.
func setTempCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(tempCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearTempCookie immediately expires a temporary cookie.
func clearTempCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
	})
}

// generateStateNonce returns a 32-byte hex-encoded random state nonce for CSRF protection.
func generateStateNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GET /api/auth/login
//
// Initiates the Authentik PKCE OIDC flow:
//  1. Generates a PKCE code_verifier and derived code_challenge.
//  2. Generates a state nonce for CSRF protection.
//  3. Stores both in short-lived HttpOnly cookies for retrieval in /callback.
//  4. Redirects the browser to Authentik's authorization endpoint.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	verifier, err := auth.GenerateCodeVerifier()
	if err != nil {
		slog.Error("auth login: generate code verifier", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	state, err := generateStateNonce()
	if err != nil {
		slog.Error("auth login: generate state nonce", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	challenge := auth.CodeChallenge(verifier)

	setTempCookie(w, pkceVerifierCookie, verifier)
	setTempCookie(w, oauthStateCookie, state)

	authURL := s.oidc.AuthCodeURL(state, challenge)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// GET /api/auth/callback
//
// Handles the Authentik redirect after user login:
//  1. Validates the state nonce against the cookie value (CSRF check).
//  2. Exchanges the authorization code for tokens using the stored PKCE verifier.
//  3. Parses the id_token to extract user claims.
//  4. Upserts the user record in the database.
//  5. Creates a new session and sets the kai_session cookie.
//  6. Clears PKCE/state temp cookies and redirects to /.
func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	// ── Validate state (CSRF) ────────────────────────────────────────────────
	stateParam := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateParam == "" || stateParam != stateCookie.Value {
		slog.Warn("auth callback: state mismatch", "param", stateParam)
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// ── Read PKCE verifier ───────────────────────────────────────────────────
	verifierCookie, err := r.Cookie(pkceVerifierCookie)
	if err != nil {
		slog.Warn("auth callback: missing pkce_verifier cookie")
		http.Error(w, "missing verifier", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		oidcErr := r.URL.Query().Get("error")
		slog.Warn("auth callback: no code", "oidc_error", oidcErr)
		http.Error(w, "authorization failed", http.StatusBadRequest)
		return
	}

	// ── Exchange code for tokens ─────────────────────────────────────────────
	token, err := s.oidc.Exchange(r.Context(), code, verifierCookie.Value)
	if err != nil {
		slog.Error("auth callback: token exchange", "err", err)
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}

	// ── Parse id_token claims ────────────────────────────────────────────────
	claims, err := s.oidc.ParseIDToken(token)
	if err != nil {
		slog.Error("auth callback: parse id_token", "err", err)
		http.Error(w, "invalid id_token", http.StatusBadGateway)
		return
	}

	if claims.Sub == "" || claims.Email == "" {
		slog.Error("auth callback: missing required claims", "sub", claims.Sub, "email", claims.Email)
		http.Error(w, "incomplete claims", http.StatusBadGateway)
		return
	}

	// Derive admin status from Authentik groups claim.
	isAdmin := false
	for _, g := range claims.Groups {
		if g == "kai-admins" || g == "platform-admins" {
			isAdmin = true
			break
		}
	}

	// ── Upsert user ──────────────────────────────────────────────────────────
	_, err = queries.UpsertUser(r.Context(), s.pool, queries.User{
		ID:          claims.Sub,
		Email:       claims.Email,
		DisplayName: claims.Name,
		AvatarURL:   claims.Picture,
		IsAdmin:     isAdmin,
	})
	if err != nil {
		slog.Error("auth callback: upsert user", "err", err, "sub", claims.Sub)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// ── Create session ───────────────────────────────────────────────────────
	rawToken, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		slog.Error("auth callback: generate session token", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	expires := time.Now().Add(auth.SessionDuration)

	_, err = queries.CreateSession(
		r.Context(),
		s.pool,
		claims.Sub,
		tokenHash,
		r.UserAgent(),
		r.RemoteAddr,
		expires,
	)
	if err != nil {
		slog.Error("auth callback: create session", "err", err, "sub", claims.Sub)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// ── Set session cookie and clean up temp cookies ─────────────────────────
	auth.SetSessionCookie(w, rawToken, expires)
	clearTempCookie(w, pkceVerifierCookie)
	clearTempCookie(w, oauthStateCookie)

	http.Redirect(w, r, "/", http.StatusFound)
}

// GET /api/auth/logout
//
// Terminates the current session:
//  1. Reads and hashes the session cookie.
//  2. Deletes the session from the database.
//  3. Clears the cookie and redirects to /.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		// No session cookie — already logged out; redirect to root.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	hash := auth.HashToken(cookie.Value)

	if err := queries.DeleteSessionByTokenHash(r.Context(), s.pool, hash); err != nil {
		// Log but don't block logout — always clear the cookie.
		slog.Warn("auth logout: delete session", "err", err)
	}

	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusFound)
}
