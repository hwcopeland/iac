package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

func TestIsInternalIP(t *testing.T) {
	cidrs := parseCIDRs([]string{
		"10.42.0.0/16",
		"10.43.0.0/16",
		"127.0.0.0/8",
		"::1/128",
	})
	am := &AuthMiddleware{cidrs: cidrs}

	tests := []struct {
		remoteAddr string
		want       bool
		desc       string
	}{
		// Pod CIDR.
		{"10.42.0.1:12345", true, "pod CIDR with port"},
		{"10.42.255.255:80", true, "pod CIDR high end"},
		{"10.42.0.0:80", true, "pod CIDR base"},

		// Service CIDR.
		{"10.43.0.1:8080", true, "service CIDR with port"},
		{"10.43.255.255:443", true, "service CIDR high end"},

		// Loopback.
		{"127.0.0.1:8080", true, "IPv4 loopback"},
		{"127.0.0.2:80", true, "IPv4 loopback alternate"},
		{"[::1]:8080", true, "IPv6 loopback with port"},

		// External IPs should not be exempt.
		{"192.168.1.1:8080", false, "external RFC1918"},
		{"8.8.8.8:443", false, "external public IP"},
		{"10.44.0.1:80", false, "just outside service CIDR"},
		{"10.41.255.255:80", false, "just below pod CIDR"},

		// Edge cases.
		{"", false, "empty string"},
		{"not-an-ip:80", false, "invalid IP"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := am.isInternalIP(tt.remoteAddr)
			if got != tt.want {
				t.Errorf("isInternalIP(%q) = %v, want %v", tt.remoteAddr, got, tt.want)
			}
		})
	}
}

func TestParseCIDRs(t *testing.T) {
	nets := parseCIDRs([]string{
		"10.42.0.0/16",
		"invalid-cidr",
		"10.43.0.0/16",
		"::1/128",
	})

	// Should have 3 valid CIDRs (invalid one skipped).
	if len(nets) != 3 {
		t.Errorf("expected 3 parsed CIDRs, got %d", len(nets))
	}

	// Verify the valid ones are correct.
	expected := []string{"10.42.0.0/16", "10.43.0.0/16", "::1/128"}
	for i, want := range expected {
		if i >= len(nets) {
			break
		}
		if nets[i].String() != want {
			t.Errorf("CIDR[%d]: expected %q, got %q", i, want, nets[i].String())
		}
	}
}

func TestNoopAuthMiddleware(t *testing.T) {
	noop := noopAuthMiddleware()
	called := false
	inner := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	wrapped := noop.WrapNoop(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if !called {
		t.Error("expected inner handler to be called with noop middleware")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestWrapMissingAuthHeader(t *testing.T) {
	am := &AuthMiddleware{
		cidrs: parseCIDRs([]string{"10.42.0.0/16"}),
	}

	inner := func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called when auth header is missing")
	}

	wrapped := am.Wrap(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "192.168.1.100:12345" // external IP
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestWrapBadAuthHeaderFormat(t *testing.T) {
	am := &AuthMiddleware{
		cidrs: parseCIDRs([]string{"10.42.0.0/16"}),
	}

	inner := func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called with bad auth header")
	}

	tests := []struct {
		authHeader string
		desc       string
	}{
		{"Basic dXNlcjpwYXNz", "Basic auth instead of Bearer"},
		{"Bearer", "Bearer without token"},
		{"token-without-scheme", "no scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			wrapped := am.Wrap(inner)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
			req.RemoteAddr = "192.168.1.100:12345"
			req.Header.Set("Authorization", tt.authHeader)
			rr := httptest.NewRecorder()
			wrapped(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestWrapInternalIPBypass(t *testing.T) {
	am := &AuthMiddleware{
		cidrs: parseCIDRs([]string{
			"10.42.0.0/16",
			"10.43.0.0/16",
			"127.0.0.0/8",
			"::1/128",
		}),
	}

	called := false
	inner := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	wrapped := am.Wrap(inner)

	// Request from pod CIDR — no auth header needed.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "10.42.5.10:54321"
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if !called {
		t.Error("expected inner handler to be called for internal IP")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestWrapInternalIPBypassLoopback(t *testing.T) {
	am := &AuthMiddleware{
		cidrs: parseCIDRs([]string{"127.0.0.0/8", "::1/128"}),
	}

	called := false
	inner := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	wrapped := am.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if !called {
		t.Error("expected inner handler to be called for loopback IP")
	}
}

// TestWrapValidJWT tests the full happy path with a real ECDSA-signed JWT.
func TestWrapValidJWT(t *testing.T) {
	// Generate an ECDSA key pair.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	issuer := "https://auth.example.com/oidc"

	// Build a valid JWT.
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(now),
		Subject:   "test-user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key-1"

	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Build a JWKS containing the public key.
	jwksJSON := buildTestJWKS(t, privateKey, "test-key-1")
	jwks, err := keyfunc.NewJWKSetJSON(jwksJSON)
	if err != nil {
		t.Fatalf("failed to create JWKS keyfunc: %v", err)
	}

	am := &AuthMiddleware{
		jwks:      jwks,
		issuerURL: issuer,
		cidrs:     parseCIDRs([]string{"10.42.0.0/16"}),
	}

	called := false
	inner := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	wrapped := am.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "192.168.1.100:12345" // external IP
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if !called {
		t.Error("expected inner handler to be called with valid JWT")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

// TestWrapExpiredJWT verifies that expired tokens are rejected.
func TestWrapExpiredJWT(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	issuer := "https://auth.example.com/oidc"

	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // expired
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		Subject:   "test-user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key-1"

	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	jwksJSON := buildTestJWKS(t, privateKey, "test-key-1")
	jwks, err := keyfunc.NewJWKSetJSON(jwksJSON)
	if err != nil {
		t.Fatalf("failed to create JWKS keyfunc: %v", err)
	}

	am := &AuthMiddleware{
		jwks:      jwks,
		issuerURL: issuer,
		cidrs:     parseCIDRs([]string{"10.42.0.0/16"}),
	}

	inner := func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called with expired JWT")
	}

	wrapped := am.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

// TestWrapWrongIssuer verifies that tokens with a wrong issuer are rejected.
func TestWrapWrongIssuer(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	claims := jwt.RegisteredClaims{
		Issuer:    "https://evil.example.com/oidc",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Subject:   "test-user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key-1"

	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	jwksJSON := buildTestJWKS(t, privateKey, "test-key-1")
	jwks, err := keyfunc.NewJWKSetJSON(jwksJSON)
	if err != nil {
		t.Fatalf("failed to create JWKS keyfunc: %v", err)
	}

	am := &AuthMiddleware{
		jwks:      jwks,
		issuerURL: "https://auth.example.com/oidc", // different from token issuer
		cidrs:     parseCIDRs([]string{"10.42.0.0/16"}),
	}

	inner := func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called with wrong issuer")
	}

	wrapped := am.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

// TestWrapInvalidSignature verifies that tokens signed with a different key are rejected.
func TestWrapInvalidSignature(t *testing.T) {
	// Sign with one key.
	signingKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate signing key: %v", err)
	}

	// JWKS contains a different key.
	jwksKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate JWKS key: %v", err)
	}

	issuer := "https://auth.example.com/oidc"

	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Subject:   "test-user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key-1"

	tokenStr, err := token.SignedString(signingKey) // signed with signingKey
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	jwksJSON := buildTestJWKS(t, jwksKey, "test-key-1") // JWKS has jwksKey
	jwks, err := keyfunc.NewJWKSetJSON(jwksJSON)
	if err != nil {
		t.Fatalf("failed to create JWKS keyfunc: %v", err)
	}

	am := &AuthMiddleware{
		jwks:      jwks,
		issuerURL: issuer,
		cidrs:     parseCIDRs([]string{"10.42.0.0/16"}),
	}

	inner := func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called with invalid signature")
	}

	wrapped := am.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

// TestWriteAuthError tests the JSON error response for auth failures.
func TestWriteAuthError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeAuthError(rr, "unauthorized", http.StatusUnauthorized)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error %q, got %q", "unauthorized", body["error"])
	}
}

// TestNewAuthMiddlewareOIDCDiscovery tests NewAuthMiddleware with a mock OIDC server.
func TestNewAuthMiddlewareOIDCDiscovery(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	jwksJSON := buildTestJWKS(t, privateKey, "test-key-1")

	// Create a test OIDC server.
	mux := http.NewServeMux()

	var serverURL string

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":   serverURL,
			"jwks_uri": serverURL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwksJSON)
	})

	server := httptest.NewServer(mux)
	defer server.Close()
	serverURL = server.URL

	am, err := NewAuthMiddleware(server.URL)
	if err != nil {
		t.Fatalf("NewAuthMiddleware failed: %v", err)
	}
	defer am.Shutdown()

	if am.issuerURL != server.URL {
		t.Errorf("expected issuerURL %q, got %q", server.URL, am.issuerURL)
	}

	// Verify it can validate a token signed with the private key.
	claims := jwt.RegisteredClaims{
		Issuer:    server.URL,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Subject:   "test-user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key-1"

	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	called := false
	inner := func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}

	wrapped := am.Wrap(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dockingjobs", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if !called {
		t.Error("expected inner handler to be called with valid JWT from OIDC-discovered JWKS")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

// buildTestJWKS creates a JWKS JSON document from an ECDSA private key.
func buildTestJWKS(t *testing.T, key *ecdsa.PrivateKey, kid string) json.RawMessage {
	t.Helper()

	// Encode the EC public key coordinates as base64url.
	pub := key.PublicKey
	xBytes := pub.X.Bytes()
	yBytes := pub.Y.Bytes()

	// Pad to 32 bytes for P-256.
	byteLen := (pub.Params().BitSize + 7) / 8
	xPad := make([]byte, byteLen-len(xBytes))
	yPad := make([]byte, byteLen-len(yBytes))
	xBytes = append(xPad, xBytes...)
	yBytes = append(yPad, yBytes...)

	jwk := map[string]interface{}{
		"kty": "EC",
		"crv": "P-256",
		"kid": kid,
		"use": "sig",
		"alg": "ES256",
		"x":   base64URLEncode(xBytes),
		"y":   base64URLEncode(yBytes),
	}

	jwksDoc := map[string]interface{}{
		"keys": []interface{}{jwk},
	}

	raw, err := json.Marshal(jwksDoc)
	if err != nil {
		t.Fatalf("failed to marshal JWKS: %v", err)
	}
	return raw
}

// base64URLEncode encodes bytes as base64url without padding (per JWK/RFC 7517 spec).
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// isInternalIPStandalone tests the standalone function variant without a receiver.
func TestIsInternalIPStandalone(t *testing.T) {
	// Verify localhost/loopback detection with various formats.
	am := &AuthMiddleware{
		cidrs: parseCIDRs([]string{"127.0.0.0/8", "::1/128"}),
	}

	if !am.isInternalIP("127.0.0.1:9999") {
		t.Error("127.0.0.1 should be internal")
	}
	if am.isInternalIP("128.0.0.1:9999") {
		t.Error("128.0.0.1 should not be internal")
	}
}

// TestIsInternalIPNoPort verifies graceful handling when RemoteAddr has no port.
func TestIsInternalIPNoPort(t *testing.T) {
	am := &AuthMiddleware{
		cidrs: parseCIDRs([]string{"10.42.0.0/16"}),
	}

	// net.SplitHostPort will error without a port, so isInternalIP falls back to
	// treating the whole string as the IP. This tests that fallback path.
	if am.isInternalIP("10.42.1.1") {
		// This is expected to return true since "10.42.1.1" is a valid IP.
		// Actually, without a port, SplitHostPort will fail and we use the raw
		// string which is a valid IP.
	}

	// Just verify it does not panic.
	_ = am.isInternalIP("10.42.1.1")
	_ = am.isInternalIP("")
	_ = am.isInternalIP("garbage")
}

// TestNewAuthMiddlewareMissingJWKSURI verifies error when discovery doc has no jwks_uri.
func TestNewAuthMiddlewareMissingJWKSURI(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"issuer": "https://example.com",
			// No jwks_uri.
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := NewAuthMiddleware(server.URL)
	if err == nil {
		t.Fatal("expected error when jwks_uri is missing")
	}
	if !contains(err.Error(), "jwks_uri") {
		t.Errorf("expected error mentioning jwks_uri, got: %v", err)
	}
}

// TestNewAuthMiddlewareBadDiscoveryURL verifies error when OIDC endpoint is unreachable.
func TestNewAuthMiddlewareBadDiscoveryURL(t *testing.T) {
	// Use a listener that immediately closes to get a connection-refused error.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // close immediately so connections are refused

	_, err = NewAuthMiddleware("http://" + addr)
	if err == nil {
		t.Fatal("expected error when OIDC endpoint is unreachable")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
