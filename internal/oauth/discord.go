package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

var discordEndpoint = oauth2.Endpoint{
	AuthURL:  "https://discord.com/api/oauth2/authorize",
	TokenURL: "https://discord.com/api/oauth2/token",
}

// DiscordProvider implements OAuth2 login against Discord API v10.
type DiscordProvider struct {
	cfg *oauth2.Config
}

// NewDiscordProvider constructs a DiscordProvider.
func NewDiscordProvider(clientID, clientSecret, redirectURL string) *DiscordProvider {
	return &DiscordProvider{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"identify", "email"},
			Endpoint:     discordEndpoint,
		},
	}
}

func (d *DiscordProvider) AuthURL(state string) string {
	return d.cfg.AuthCodeURL(state)
}

func (d *DiscordProvider) ExchangeUser(ctx context.Context, code string) (*ExternalUser, error) {
	token, err := d.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("discord token exchange: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord userinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord userinfo returned HTTP %d", resp.StatusCode)
	}

	var profile struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode discord profile: %w", err)
	}
	if profile.ID == "" || profile.Email == "" {
		return nil, fmt.Errorf("discord profile missing id or email")
	}

	return &ExternalUser{
		Provider:       "discord",
		ProviderUserID: profile.ID,
		Email:          profile.Email,
	}, nil
}
