// Package main provides JWT authentication middleware for the docking controller API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware validates JWT tokens against an OIDC provider's JWKS endpoint.
// Requests from internal pod/service CIDRs bypass authentication entirely.
type AuthMiddleware struct {
	jwks      keyfunc.Keyfunc
	issuerURL string
	cidrs     []*net.IPNet

	mu     sync.RWMutex
	cancel context.CancelFunc
}

// oidcDiscovery represents the subset of an OIDC discovery document we need.
type oidcDiscovery struct {
	JWKSURI string `json:"jwks_uri"`
	Issuer  string `json:"issuer"`
}

// NewAuthMiddleware creates a new AuthMiddleware by fetching the OIDC discovery
// document and initializing the JWKS keyfunc for token validation. It starts a
// background goroutine that refreshes the JWKS every hour.
func NewAuthMiddleware(issuerURL string) (*AuthMiddleware, error) {
	// Normalize issuer URL: strip trailing slash for consistent comparison.
	issuerURL = strings.TrimRight(issuerURL, "/")

	// Fetch OIDC discovery document.
	discoveryURL := issuerURL + "/.well-known/openid-configuration"
	log.Printf("[auth] Fetching OIDC discovery from %s", discoveryURL)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("fetching OIDC discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}

	var disc oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		return nil, fmt.Errorf("decoding OIDC discovery document: %w", err)
	}

	if disc.JWKSURI == "" {
		return nil, fmt.Errorf("OIDC discovery document missing jwks_uri")
	}
	log.Printf("[auth] JWKS URI: %s", disc.JWKSURI)

	// Initialize JWKS keyfunc with background refresh.
	ctx, cancel := context.WithCancel(context.Background())

	jwks, err := keyfunc.NewJWKSetJSON(mustFetchJWKS(client, disc.JWKSURI))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating JWKS keyfunc: %w", err)
	}

	// Parse internal CIDRs: pod CIDR, service CIDR, localhost.
	cidrs := parseCIDRs([]string{
		"10.42.0.0/16", // RKE2 pod CIDR
		"10.43.0.0/16", // RKE2 service CIDR
		"127.0.0.0/8",  // IPv4 loopback
		"::1/128",      // IPv6 loopback
	})

	am := &AuthMiddleware{
		jwks:      jwks,
		issuerURL: issuerURL,
		cidrs:     cidrs,
		cancel:    cancel,
	}

	// Background JWKS refresh every hour.
	go am.refreshLoop(ctx, client, disc.JWKSURI)

	log.Printf("[auth] JWT authentication middleware initialized (issuer: %s)", issuerURL)
	return am, nil
}

// mustFetchJWKS fetches the raw JWKS JSON from the given URI.
func mustFetchJWKS(client *http.Client, jwksURI string) json.RawMessage {
	resp, err := client.Get(jwksURI)
	if err != nil {
		log.Fatalf("[auth] Failed to fetch JWKS from %s: %v", jwksURI, err)
	}
	defer resp.Body.Close()

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		log.Fatalf("[auth] Failed to decode JWKS from %s: %v", jwksURI, err)
	}
	return raw
}

// refreshLoop periodically re-fetches the JWKS to pick up key rotations.
func (a *AuthMiddleware) refreshLoop(ctx context.Context, client *http.Client, jwksURI string) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("[auth] Refreshing JWKS from %s", jwksURI)
			raw := fetchJWKS(client, jwksURI)
			if raw == nil {
				log.Printf("[auth] JWKS refresh failed, keeping existing keys")
				continue
			}
			newJWKS, err := keyfunc.NewJWKSetJSON(raw)
			if err != nil {
				log.Printf("[auth] Failed to parse refreshed JWKS: %v", err)
				continue
			}
			a.mu.Lock()
			a.jwks = newJWKS
			a.mu.Unlock()
			log.Printf("[auth] JWKS refreshed successfully")
		}
	}
}

// fetchJWKS fetches JWKS JSON, returning nil on error (non-fatal for refresh).
func fetchJWKS(client *http.Client, jwksURI string) json.RawMessage {
	resp, err := client.Get(jwksURI)
	if err != nil {
		log.Printf("[auth] Failed to fetch JWKS: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[auth] JWKS endpoint returned HTTP %d", resp.StatusCode)
		return nil
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		log.Printf("[auth] Failed to decode JWKS response: %v", err)
		return nil
	}
	return raw
}

// Shutdown stops the background JWKS refresh goroutine.
func (a *AuthMiddleware) Shutdown() {
	if a.cancel != nil {
		a.cancel()
	}
}

// Wrap returns a middleware-wrapped version of the given handler.
// It checks for internal IP exemption first, then validates the JWT.
func (a *AuthMiddleware) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for internal IPs.
		if a.isInternalIP(r.RemoteAddr) {
			next(w, r)
			return
		}

		// Extract Bearer token from Authorization header.
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeAuthError(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeAuthError(w, "invalid Authorization header format, expected 'Bearer <token>'", http.StatusUnauthorized)
			return
		}
		tokenStr := parts[1]

		// Validate the JWT.
		a.mu.RLock()
		jwks := a.jwks
		a.mu.RUnlock()

		token, err := jwt.Parse(tokenStr, jwks.KeyfuncCtx(r.Context()),
			jwt.WithIssuer(a.issuerURL),
			jwt.WithExpirationRequired(),
			jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}),
		)
		if err != nil {
			writeAuthError(w, fmt.Sprintf("invalid token: %v", err), http.StatusUnauthorized)
			return
		}
		if !token.Valid {
			writeAuthError(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// isInternalIP checks whether the request originates from an internal IP address
// (pod CIDR, service CIDR, or loopback).
func (a *AuthMiddleware) isInternalIP(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// RemoteAddr may not have a port (unlikely for HTTP but handle gracefully).
		host = remoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, cidr := range a.cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// parseCIDRs parses a list of CIDR strings into net.IPNet values.
// Invalid CIDRs are logged and skipped.
func parseCIDRs(cidrs []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			log.Printf("[auth] Warning: invalid CIDR %q: %v", cidr, err)
			continue
		}
		nets = append(nets, ipNet)
	}
	return nets
}

// noopAuthMiddleware returns a passthrough wrapper when auth is disabled.
func noopAuthMiddleware() *AuthMiddleware {
	return &AuthMiddleware{}
}

// WrapNoop is used when AUTH_ENABLED is not "true". It passes through all requests.
func (a *AuthMiddleware) WrapNoop(next http.HandlerFunc) http.HandlerFunc {
	return next
}

// writeAuthError writes a JSON authentication error response.
func writeAuthError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
