---
project: "kai"
maturity: "draft"
last_updated: "2025-07-14"
updated_by: "@staff-engineer"
scope: "Authentik OIDC integration for Kai: PKCE flow, session management, group-based
  authorization, API key auth, and Authentik blueprint."
owner: "@staff-engineer"
dependencies:
  - docs/tdd/kai-backend.md
  - docs/tdd/kai-deploy.md
  - rke2/authentik/blueprints/providers-openhands.yaml
  - rke2/authentik/blueprints/scope-mappings.yaml
---

# TDD: Kai Authentication & Authorization

## 1. Problem Statement

Kai uses **direct Authentik OIDC with PKCE** — no Keycloak, no broker chain.

The Keycloak-as-Authentik-broker architecture used by OpenHands on this cluster is
broken: Keycloak's `offline_access` scope is not configured on the `allhands` client,
causing `validate_offline_token` to fail, which triggers an infinite redirect loop.
Kai is explicitly designed to avoid every layer of that stack.

**The single auth rule**: No tokens, auth flags, or session identifiers are stored in
the browser (localStorage, sessionStorage, JS memory). The only browser-side state is
an HTTP-only SameSite=Strict cookie managed entirely by the backend.

---

## 2. Auth Flow: PKCE (Authorization Code + PKCE)

### 2.1 Sequence

```
Browser                    Kai Backend               Authentik
   │                            │                        │
   │  GET /api/auth/login        │                        │
   │ ─────────────────────────► │                        │
   │                            │  generate code_verifier│
   │                            │  generate state        │
   │                            │  store state→verifier  │
   │                            │  (10-min TTL)          │
   │  302 → Authentik authz URL │                        │
   │ ◄───────────────────────── │                        │
   │                            │                        │
   │  GET /application/o/kai/authorize?                  │
   │      code_challenge=...&state=...                   │
   │ ──────────────────────────────────────────────────► │
   │                            │                        │
   │  [User clicks "Authorize"] │                        │
   │                            │                        │
   │  302 → /api/auth/callback?code=...&state=...        │
   │ ◄──────────────────────────────────────────────────  │
   │                            │                        │
   │  GET /api/auth/callback    │                        │
   │ ─────────────────────────► │                        │
   │                            │  validate state        │
   │                            │  POST /token           │
   │                            │      code + verifier   │
   │                            │ ─────────────────────► │
   │                            │  {access_token,        │
   │                            │   id_token,            │
   │                            │   refresh_token}       │
   │                            │ ◄───────────────────── │
   │                            │  verify id_token JWKS  │
   │                            │  extract claims        │
   │                            │  upsert user in DB     │
   │                            │  create session row    │
   │  302 /                     │                        │
   │  Set-Cookie: kai_session=  │                        │
   │ ◄───────────────────────── │                        │
   │                            │                        │
   │  GET /api/me               │                        │
   │ ─────────────────────────► │                        │
   │  200 {id, email, is_admin} │                        │
   │ ◄───────────────────────── │                        │
```

### 2.2 Why PKCE (not client credentials or implicit)

- **Not implicit**: implicit flow sends tokens in URL fragment — logged in proxy/server access logs
- **Not client credentials**: no user identity
- **PKCE**: code_verifier never leaves the backend; code interception by a MITM yields nothing without the verifier; standard for public/backend apps calling OIDC

---

## 3. Authentik Blueprint

This file is committed at `rke2/kai/authentik/providers-kai.yaml` and applied by the
Authentik operator on startup (same pattern as `providers-openhands.yaml`).

```yaml
# rke2/kai/authentik/providers-kai.yaml
version: 1
metadata:
  name: kai Provider and Application
entries:
- attrs:
    access_code_validity: minutes=1
    access_token_validity: hours=1
    authorization_flow: !Find [authentik_flows.flow, [slug, default-provider-authorization-implicit-consent]]
    client_id: kai                         # REPLACE with generated client_id from Bitwarden
    client_type: confidential
    include_claims_in_id_token: true
    invalidation_flow: !Find [authentik_flows.flow, [slug, default-provider-invalidation-flow]]
    issuer_mode: per_provider
    logout_method: backchannel
    name: kai
    property_mappings:
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, email]]
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, profile]]
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, openid]]
    - !Find [authentik_providers_oauth2.scopemapping, [scope_name, groups]]  # existing mapping
    redirect_uris:
    - matching_mode: strict
      url: https://kai.hwcopeland.net/api/auth/callback
    refresh_token_threshold: seconds=0
    refresh_token_validity: days=30
    signing_key: !Find [authentik_crypto.certificatekeypair, [name, authentik Self-signed Certificate]]
    sub_mode: hashed_user_id              # consistent with other apps on this cluster
  conditions: []
  identifiers:
    pk: 9                                 # increment from openhands (pk: 8)
  model: authentik_providers_oauth2.oauth2provider
  permissions: []
  state: present
- attrs:
    meta_description: Kai AI Agent Platform
    meta_launch_url: https://kai.hwcopeland.net
    name: kai
    policy_engine_mode: any
    provider: !Find [authentik_providers_oauth2.oauth2provider, [name, kai]]
    slug: kai
  conditions: []
  identifiers:
    pk: kai-app-uuid-placeholder          # REPLACE with actual UUID
  model: authentik_core.application
  permissions: []
  state: present
```

**Key differences from `providers-openhands.yaml`**:
- `redirect_uris` points directly to `kai.hwcopeland.net/api/auth/callback` (no Keycloak)
- `groups` scope mapping included — Authentik already has this scope at `scope-mappings.yaml`
- `sub_mode: hashed_user_id` — matches OpenHands; ensures `users.id` is stable

---

## 4. JWKS Validation

The backend validates ID tokens locally using Authentik's JWKS endpoint. No introspection
call on every request — the JWKS is cached and refreshed automatically.

```go
// internal/auth/oidc.go

type OIDCClient struct {
    keyCache    *jwk.Cache
    oauth2Cfg   *oauth2.Config
    issuerURL   string
}

func NewOIDCClient(cfg *config.Config) (*OIDCClient, error) {
    jwksURL := cfg.AuthIssuerURL + ".well-known/jwks.json"
    // e.g. https://auth.hwcopeland.net/application/o/kai/.well-known/jwks.json

    cache := jwk.NewCache(context.Background())
    cache.Register(jwksURL, jwk.WithMinRefreshInterval(15*time.Minute))

    // Pre-fetch on startup
    if _, err := cache.Refresh(context.Background(), jwksURL); err != nil {
        return nil, fmt.Errorf("fetch JWKS: %w", err)
    }

    return &OIDCClient{
        keyCache: cache,
        issuerURL: cfg.AuthIssuerURL,
        oauth2Cfg: &oauth2.Config{
            ClientID:     cfg.AuthClientID,
            ClientSecret: cfg.AuthClientSecret,
            RedirectURL:  cfg.AuthRedirectURL,
            Endpoint: oauth2.Endpoint{
                AuthURL:  cfg.AuthIssuerURL + "authorize/",
                TokenURL: cfg.AuthIssuerURL + "token/",
            },
            Scopes: []string{"openid", "profile", "email", "groups"},
        },
    }, nil
}

func (c *OIDCClient) VerifyIDToken(ctx context.Context, rawIDToken string) (jwt.Token, error) {
    keySet, err := c.keyCache.Get(ctx, c.issuerURL+".well-known/jwks.json")
    if err != nil {
        return nil, fmt.Errorf("get keyset: %w", err)
    }

    token, err := jwt.Parse([]byte(rawIDToken),
        jwt.WithKeySet(keySet),
        jwt.WithValidate(true),
        jwt.WithIssuer(c.issuerURL),
        jwt.WithAudience(c.oauth2Cfg.ClientID),
    )
    return token, err
}
```

---

## 5. Claims Extraction

```go
// internal/auth/claims.go

type Claims struct {
    Sub     string   // Authentik hashed user ID — becomes users.id
    Email   string
    Name    string   // display_name
    Picture string   // avatar_url
    Groups  []string // Authentik groups; "kai-admins" → is_admin=true
}

func ExtractClaims(token jwt.Token) (*Claims, error) {
    sub, _ := token.Subject()
    email, _ := token.Get("email")
    name, _  := token.Get("name")
    pic, _   := token.Get("picture")

    var groups []string
    if raw, ok := token.Get("groups"); ok {
        if arr, ok := raw.([]interface{}); ok {
            for _, g := range arr {
                if s, ok := g.(string); ok {
                    groups = append(groups, s)
                }
            }
        }
    }

    return &Claims{
        Sub:     sub,
        Email:   stringOrEmpty(email),
        Name:    stringOrEmpty(name),
        Picture: stringOrEmpty(pic),
        Groups:  groups,
    }, nil
}

func containsGroup(groups []string, target string) bool {
    for _, g := range groups {
        if g == target {
            return true
        }
    }
    return false
}
```

---

## 6. Session Management

### 6.1 Session Token Generation

```go
// internal/auth/session.go

// GenerateSessionToken returns a cryptographically random 32-byte token, base64url-encoded.
// This becomes the kai_session cookie value.
func GenerateSessionToken() (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.RawURLEncoding.EncodeToString(b), nil
}

// SHA256Hex returns the hex-encoded SHA-256 hash of s.
// This is stored in sessions.token_hash — the DB never holds the raw token.
func SHA256Hex(s string) string {
    h := sha256.Sum256([]byte(s))
    return hex.EncodeToString(h[:])
}
```

### 6.2 Cookie Settings

```go
http.SetCookie(w, &http.Cookie{
    Name:     "kai_session",
    Value:    rawToken,        // 32 random bytes, base64url
    Path:     "/",
    HttpOnly: true,            // not accessible to JavaScript
    Secure:   true,            // HTTPS only
    SameSite: http.SameSiteStrictMode,  // no cross-site requests
    Expires:  time.Now().Add(cfg.SessionDuration),  // default 7 days
})
```

**Why SameSite=Strict**: Prevents CSRF — a form on attacker.com cannot trigger a
state-changing request to kai.hwcopeland.net with the cookie attached.

### 6.3 Session Validation Middleware

Every authenticated request:
1. Extract `kai_session` cookie
2. SHA-256 hash it
3. Query `sessions WHERE token_hash = $1 AND expires_at > NOW()`
4. Load `users WHERE id = $1`
5. Attach user to request context

Cost: 1 DB round-trip per request. Acceptable for Phase 1. Phase 3 option: Redis
session cache with `users.id` as cache key.

### 6.4 Logout

```go
func (h *AuthHandlers) LogoutHandler(w http.ResponseWriter, r *http.Request) {
    if cookie, err := r.Cookie("kai_session"); err == nil {
        tokenHash := auth.SHA256Hex(cookie.Value)
        h.db.DeleteSessionByTokenHash(r.Context(), tokenHash)
    }
    http.SetCookie(w, &http.Cookie{
        Name:    "kai_session",
        Value:   "",
        Path:    "/",
        Expires: time.Unix(0, 0),
        MaxAge:  -1,
    })
    http.Redirect(w, r, "/login", http.StatusFound)
}
```

---

## 7. API Key Authentication

Non-browser clients (CI pipelines, scripts) use API keys instead of cookies.

### 7.1 Key Format

`kai_<base64url(32 random bytes)>`

The `kai_` prefix makes keys identifiable in logs/secrets scanners.
The first 8 characters after the prefix are stored as `key_prefix` for display.

### 7.2 Key Validation

```go
func (a *AuthService) validateAPIKey(ctx context.Context, bearer string) (*db.User, error) {
    if !strings.HasPrefix(bearer, "kai_") {
        return nil, errors.New("not a kai api key")
    }
    keyHash := SHA256Hex(bearer)
    key, err := a.db.GetAPIKeyByHash(ctx, keyHash)
    if err != nil {
        return nil, err
    }
    if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
        return nil, errors.New("api key expired")
    }
    // Update last_used_at asynchronously (don't block request)
    go a.db.UpdateAPIKeyLastUsed(context.Background(), key.ID)
    return a.db.GetUserByID(ctx, key.UserID)
}
```

---

## 8. Internal Callback Auth

Agent pods authenticate to `/internal/callback` (port 8081) using a shared secret,
not OIDC. The secret is injected via `envFrom` into the agent pod container.

```go
// Validation in internal.go:
if r.Header.Get("X-Kai-Callback-Token") != h.cfg.CallbackToken {
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
}
```

The `SECRET_CALLBACK_TOKEN` is a random 32-byte value stored in Bitwarden and
delivered via ExternalSecret → Kubernetes Secret → `envFrom` in both the kai-api
Deployment and AgentSandbox pod spec.

---

## 9. Group-Based Authorization

Authentik group `kai-admins` → `users.is_admin = true` (synced on every login).

```go
// Admin middleware (applied to /api/admin/* routes):
func (a *AuthService) AdminMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user := UserFromContext(r.Context())
        if user == nil || !user.IsAdmin {
            writeError(w, http.StatusForbidden, "forbidden", "Admin access required")
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

Team membership (`team_members`) controls access to team runs — checked per-route,
not via middleware.

---

## 10. Security Decisions

| Concern | Decision |
|---------|----------|
| CSRF | SameSite=Strict cookie; additionally check `Origin` header on state-changing requests |
| Session fixation | Generate new session token on every login; old sessions remain valid until expiry |
| Token storage | Raw token never in DB; only SHA-256 hash stored |
| PKCE state store | In-memory with 10-min TTL (Phase 1); PostgreSQL rows in Phase 2 |
| Replay attacks | State is single-use (deleted on first use in callback) |
| Transport | HTTPS enforced by Cilium Gateway; `Secure` cookie flag enforced by backend |
| Logs | Tokens never logged; only `session_id` (UUID) and `user_id` appear in logs |

---

## 11. Open Questions

### OQ-1: Refresh Token Strategy

Should the backend use Authentik's `refresh_token` to silently renew the ID token
before session expiry? Or just let sessions expire and require re-login?

Suggested: let sessions expire (7 days). Users re-login. Simpler, fewer moving parts.

### OQ-2: Authentik Client Secret Rotation

The `AUTH_CLIENT_SECRET` is stored in Bitwarden. Rotation requires updating Bitwarden +
restarting the kai-api pod. Document this in the runbook.

### OQ-3: Multi-Session Handling

Should a user be able to have multiple active sessions (multiple browsers/devices)?
Current design: yes — `sessions` is a 1-to-many table per user. A "revoke all sessions"
admin action should be added to Phase 2.
