package oauth

import (
	"context"
	"fmt"
)

// ProviderConfig holds persisted settings for an OAuth2/OIDC provider.
type ProviderConfig struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Enabled      bool   `json:"enabled"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	HasSecret    bool   `json:"has_secret"`
	IssuerURL    string `json:"issuer_url,omitempty"`
}

// ExternalUser represents normalized identity data returned by an external IdP.
type ExternalUser struct {
	Provider       string
	ProviderUserID string
	Email          string
}

// Provider defines the contract implemented by Google, Discord, and Generic OIDC.
type Provider interface {
	AuthURL(state string) string
	ExchangeUser(ctx context.Context, code string) (*ExternalUser, error)
}

// BuildProvider instantiates a concrete OAuth2/OIDC provider from stored config.
func BuildProvider(ctx context.Context, cfg ProviderConfig, redirectURL string) (Provider, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("provider %q is disabled", cfg.Name)
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("provider %q is missing client_id or client_secret", cfg.Name)
	}

	switch cfg.Name {
	case "google":
		return NewGoogleProvider(cfg.ClientID, cfg.ClientSecret, redirectURL), nil
	case "discord":
		return NewDiscordProvider(cfg.ClientID, cfg.ClientSecret, redirectURL), nil
	case "oidc":
		return NewOIDCProvider(ctx, cfg.IssuerURL, cfg.ClientID, cfg.ClientSecret, redirectURL)
	default:
		return nil, fmt.Errorf("unsupported oauth provider %q", cfg.Name)
	}
}
