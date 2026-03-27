package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

// OIDCClient wraps an oauth2.Config configured from Authentik OIDC discovery.
type OIDCClient struct {
	cfg     *oauth2.Config
	issuer  string
	jwksURL string
}

// Claims holds the JWT payload fields emitted by Authentik.
type Claims struct {
	Sub     string   `json:"sub"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Picture string   `json:"picture"`
	Groups  []string `json:"groups"`
}

// oidcDiscovery is the subset of the OpenID Connect Discovery document we need.
type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// NewOIDCClient performs OIDC discovery against issuerURL and returns a configured client.
// issuerURL is expected to be the Authentik application issuer, e.g.
//
//	https://auth.hwcopeland.net/application/o/kai/
func NewOIDCClient(issuerURL, clientID, clientSecret, redirectURL string) (*OIDCClient, error) {
	issuerURL = strings.TrimRight(issuerURL, "/")
	discoveryURL := issuerURL + "/.well-known/openid-configuration"

	//nolint:noctx // startup path; no request context available
	resp, err := http.Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery fetch %s: %w", discoveryURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery %s: unexpected status %d", discoveryURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery read body: %w", err)
	}

	var disc oidcDiscovery
	if err := json.Unmarshal(body, &disc); err != nil {
		return nil, fmt.Errorf("oidc discovery parse: %w", err)
	}

	if disc.AuthorizationEndpoint == "" || disc.TokenEndpoint == "" {
		return nil, fmt.Errorf("oidc discovery: missing required endpoints in %s", discoveryURL)
	}

	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  disc.AuthorizationEndpoint,
			TokenURL: disc.TokenEndpoint,
		},
		Scopes: []string{"openid", "email", "profile"},
	}

	return &OIDCClient{
		cfg:     cfg,
		issuer:  disc.Issuer,
		jwksURL: disc.JWKSURI,
	}, nil
}

// AuthCodeURL builds the Authentik authorization redirect URL with PKCE parameters.
func (c *OIDCClient) AuthCodeURL(state, codeChallenge string) string {
	return c.cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("scope", "openid email profile groups"),
	)
}

// Exchange trades an authorization code for an OAuth2 token using PKCE.
func (c *OIDCClient) Exchange(ctx context.Context, code, codeVerifier string) (*oauth2.Token, error) {
	return c.cfg.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
}

// ParseIDToken extracts claims from the id_token embedded in the token response.
//
// Phase 1: JWT payload is base64url-decoded without signature verification.
// Phase 2: replace with JWKS-backed RS256 verification via lestrrat-go/jwx.
func (c *OIDCClient) ParseIDToken(token *oauth2.Token) (*Claims, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	parts := strings.SplitN(rawIDToken, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}
	return &claims, nil
}
