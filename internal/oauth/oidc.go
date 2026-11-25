package oauth

import (
	"context"
	"fmt"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCProvider implements Generic OpenID Connect discovery and ID Token verification.
type OIDCProvider struct {
	cfg      *oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

// NewOIDCProvider discovers endpoints from issuerURL/.well-known/openid-configuration.
func NewOIDCProvider(ctx context.Context, issuerURL, clientID, clientSecret, redirectURL string) (*OIDCProvider, error) {
	if issuerURL == "" {
		return nil, fmt.Errorf("oidc provider requires issuer_url")
	}

	provider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery failed: %w", err)
	}

	return &OIDCProvider{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
		},
		verifier: provider.Verifier(&gooidc.Config{ClientID: clientID}),
	}, nil
}

func (o *OIDCProvider) AuthURL(state string) string {
	return o.cfg.AuthCodeURL(state)
}

func (o *OIDCProvider) ExchangeUser(ctx context.Context, code string) (*ExternalUser, error) {
	token, err := o.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("oidc response missing id_token")
	}

	idToken, err := o.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse id_token claims: %w", err)
	}
	if idToken.Subject == "" || claims.Email == "" {
		return nil, fmt.Errorf("oidc id_token missing sub or email")
	}

	return &ExternalUser{
		Provider:       "oidc",
		ProviderUserID: idToken.Subject,
		Email:          claims.Email,
	}, nil
}
