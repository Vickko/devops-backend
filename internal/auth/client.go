package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"devops-backend/internal/conf"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCClient wraps OIDC provider and OAuth2 configuration
type OIDCClient struct {
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	oauth2Config oauth2.Config
}

// NewOIDCClient creates a new OIDC client
func NewOIDCClient(ctx context.Context, cfg *conf.Auth, redirectURL string) (*OIDCClient, error) {
	// Initialize OIDC provider (discovers .well-known/openid-configuration)
	provider, err := oidc.NewProvider(ctx, cfg.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// Configure OAuth2
	oauth2Config := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	// Configure JWT verifier
	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	return &OIDCClient{
		provider:     provider,
		verifier:     verifier,
		oauth2Config: oauth2Config,
	}, nil
}

// GetAuthURL returns the OIDC authorization URL with state parameter
func (c *OIDCClient) GetAuthURL(state string) string {
	return c.oauth2Config.AuthCodeURL(state)
}

// ExchangeCode exchanges authorization code for tokens
func (c *OIDCClient) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	return c.oauth2Config.Exchange(ctx, code)
}

// VerifyIDToken verifies and parses the ID token
func (c *OIDCClient) VerifyIDToken(ctx context.Context, rawIDToken string) (*oidc.IDToken, error) {
	return c.verifier.Verify(ctx, rawIDToken)
}

// RefreshToken refreshes an expired access token
func (c *OIDCClient) RefreshToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	tokenSource := c.oauth2Config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
	})
	return tokenSource.Token()
}

// === PKCE Support ===

// GenerateCodeVerifier generates a random code verifier for PKCE
// Returns a base64-url-encoded random string (43-128 characters)
func GenerateCodeVerifier() (string, error) {
	// Generate 32 random bytes (will be 43 chars after base64url encoding)
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("failed to generate code verifier: %w", err)
	}
	// Base64-URL encode without padding
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// GenerateCodeChallenge generates a code challenge from the verifier
// Uses SHA256 and base64-url encoding as per RFC 7636
func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// GetAuthURLWithPKCE returns the OIDC authorization URL with PKCE parameters
func (c *OIDCClient) GetAuthURLWithPKCE(state string, codeChallenge string) string {
	return c.oauth2Config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// ExchangeCodeWithPKCE exchanges authorization code for tokens using PKCE
func (c *OIDCClient) ExchangeCodeWithPKCE(ctx context.Context, code string, codeVerifier string) (*oauth2.Token, error) {
	return c.oauth2Config.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
}
