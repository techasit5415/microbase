package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/oauth"
)

const (
	oauthStateTTL = 5 * time.Minute
	oauthCodeTTL  = 60 * time.Second
)

var (
	ErrProviderNotFound = errors.New("oauth provider not found")
	ErrInvalidState     = errors.New("invalid or expired oauth state")
	ErrInvalidCode      = errors.New("invalid or expired exchange code")
)

// UpdateProviderRequest is the payload accepted by PUT /api/oauth/providers/:name.
type UpdateProviderRequest struct {
	Enabled      bool   `json:"enabled"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	IssuerURL    string `json:"issuer_url"`
}

// AuthMethodProvider describes an enabled OAuth2/OIDC provider for client discovery.
type AuthMethodProvider struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	AuthURL     string `json:"auth_url"`
}

// AuthMethodsResponse mirrors PocketBase's auth methods discovery payload.
type AuthMethodsResponse struct {
	Password bool `json:"password"`
	OAuth2   struct {
		Enabled   bool                 `json:"enabled"`
		Providers []AuthMethodProvider `json:"providers"`
	} `json:"oauth2"`
}

type oauthStatePayload struct {
	Provider   string `json:"provider"`
	RedirectTo string `json:"redirect_to,omitempty"`
}

// OAuthCallbackResult contains either a direct AuthResponse or a redirect URL with one-time code.
type OAuthCallbackResult struct {
	Auth        *models.AuthResponse
	RedirectURL string
}

// OAuthService manages OAuth2/OIDC provider settings and external account sign-in.
type OAuthService struct {
	pool  *pgxpool.Pool
	cache cache.CacheStore
	auth  *AuthService
}

// NewOAuthService constructs an OAuthService.
func NewOAuthService(pool *pgxpool.Pool, c cache.CacheStore, authSvc *AuthService) *OAuthService {
	return &OAuthService{pool: pool, cache: c, auth: authSvc}
}

// ListProviders returns all OAuth2/OIDC provider configurations with secrets masked.
func (s *OAuthService) ListProviders(ctx context.Context) ([]oauth.ProviderConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, display_name, enabled, client_id, client_secret, issuer_url
		FROM _oauth_providers
		ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list oauth providers: %w", err)
	}
	defer rows.Close()

	list := make([]oauth.ProviderConfig, 0)
	for rows.Next() {
		var p oauth.ProviderConfig
		var secret string
		if err := rows.Scan(&p.Name, &p.DisplayName, &p.Enabled, &p.ClientID, &secret, &p.IssuerURL); err != nil {
			return nil, err
		}
		p.HasSecret = secret != ""
		list = append(list, p)
	}
	return list, rows.Err()
}

// UpdateProvider persists provider configuration without exposing or wiping existing secrets.
func (s *OAuthService) UpdateProvider(ctx context.Context, name string, req UpdateProviderRequest) (*oauth.ProviderConfig, error) {
	var p oauth.ProviderConfig
	var secret string

	err := s.pool.QueryRow(ctx, `
		UPDATE _oauth_providers
		SET enabled = $2,
			client_id = $3,
			client_secret = CASE WHEN $4 <> '' THEN $4 ELSE client_secret END,
			issuer_url = $5,
			updated_at = NOW()
		WHERE name = $1
		RETURNING name, display_name, enabled, client_id, client_secret, issuer_url`,
		name, req.Enabled, strings.TrimSpace(req.ClientID), strings.TrimSpace(req.ClientSecret), strings.TrimSpace(req.IssuerURL),
	).Scan(&p.Name, &p.DisplayName, &p.Enabled, &p.ClientID, &secret, &p.IssuerURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProviderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update oauth provider: %w", err)
	}
	p.HasSecret = secret != ""
	return &p, nil
}

// AuthMethods returns enabled authentication methods (password + enabled OAuth2/OIDC providers).
func (s *OAuthService) AuthMethods(ctx context.Context) (*AuthMethodsResponse, error) {
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	res := &AuthMethodsResponse{Password: true}
	res.OAuth2.Providers = make([]AuthMethodProvider, 0)
	for _, p := range providers {
		if p.Enabled && p.ClientID != "" && p.HasSecret {
			res.OAuth2.Providers = append(res.OAuth2.Providers, AuthMethodProvider{
				Name:        p.Name,
				DisplayName: p.DisplayName,
				AuthURL:     "/api/auth/oauth/" + p.Name,
			})
		}
	}
	res.OAuth2.Enabled = len(res.OAuth2.Providers) > 0
	return res, nil
}

// StartAuth stores CSRF state (and optional external app redirect_to) in CacheStore.
func (s *OAuthService) StartAuth(ctx context.Context, providerName, callbackURL, redirectTo string) (string, error) {
	if redirectTo != "" {
		parsed, err := url.Parse(redirectTo)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return "", fmt.Errorf("%w: invalid redirect_to URL", ErrInvalidInput)
		}
	}

	cfg, err := s.getProviderConfig(ctx, providerName)
	if err != nil {
		return "", err
	}

	prov, err := oauth.BuildProvider(ctx, *cfg, callbackURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	state := generateRandomHex(16)
	payload, _ := json.Marshal(oauthStatePayload{
		Provider:   providerName,
		RedirectTo: redirectTo,
	})
	if err := s.cache.Set(ctx, oauthStateKey(state), string(payload), oauthStateTTL); err != nil {
		return "", fmt.Errorf("store oauth state: %w", err)
	}
	return prov.AuthURL(state), nil
}

// HandleCallback verifies CSRF state, signs in the user, and either returns JWT or issues a one-time code.
func (s *OAuthService) HandleCallback(ctx context.Context, providerName, code, state, callbackURL string) (*OAuthCallbackResult, error) {
	if code == "" || state == "" {
		return nil, ErrInvalidState
	}

	rawState, err := s.cache.Get(ctx, oauthStateKey(state))
	if err != nil {
		return nil, ErrInvalidState
	}
	_ = s.cache.Delete(ctx, oauthStateKey(state))

	var stateData oauthStatePayload
	if err := json.Unmarshal([]byte(rawState), &stateData); err != nil || stateData.Provider != providerName {
		return nil, ErrInvalidState
	}

	cfg, err := s.getProviderConfig(ctx, providerName)
	if err != nil {
		return nil, err
	}

	prov, err := oauth.BuildProvider(ctx, *cfg, callbackURL)
	if err != nil {
		return nil, err
	}

	extUser, err := prov.ExchangeUser(ctx, code)
	if err != nil {
		return nil, err
	}

	user, err := s.findOrCreateUser(ctx, extUser)
	if err != nil {
		return nil, err
	}

	token, err := s.auth.GenerateToken(*user)
	if err != nil {
		return nil, err
	}

	authRes := &models.AuthResponse{Token: token, User: *user}

	// If an external app supplied redirect_to, issue a short-lived one-time code instead of exposing JWT in URL.
	if stateData.RedirectTo != "" {
		oneTimeCode := generateRandomHex(24)
		encoded, _ := json.Marshal(authRes)
		if err := s.cache.Set(ctx, oauthCodeKey(oneTimeCode), string(encoded), oauthCodeTTL); err != nil {
			return nil, fmt.Errorf("store exchange code: %w", err)
		}

		u, _ := url.Parse(stateData.RedirectTo)
		q := u.Query()
		q.Set("code", oneTimeCode)
		u.RawQuery = q.Encode()
		return &OAuthCallbackResult{RedirectURL: u.String()}, nil
	}

	return &OAuthCallbackResult{Auth: authRes}, nil
}

// ExchangeCode swaps a one-time code from redirect_to for the signed JWT and user profile.
func (s *OAuthService) ExchangeCode(ctx context.Context, code string) (*models.AuthResponse, error) {
	if strings.TrimSpace(code) == "" {
		return nil, ErrInvalidCode
	}

	raw, err := s.cache.Get(ctx, oauthCodeKey(code))
	if err != nil {
		return nil, ErrInvalidCode
	}
	_ = s.cache.Delete(ctx, oauthCodeKey(code))

	var authRes models.AuthResponse
	if err := json.Unmarshal([]byte(raw), &authRes); err != nil {
		return nil, ErrInvalidCode
	}
	return &authRes, nil
}

func (s *OAuthService) getProviderConfig(ctx context.Context, name string) (*oauth.ProviderConfig, error) {
	var p oauth.ProviderConfig
	err := s.pool.QueryRow(ctx, `
		SELECT name, display_name, enabled, client_id, client_secret, issuer_url
		FROM _oauth_providers
		WHERE name = $1`, name,
	).Scan(&p.Name, &p.DisplayName, &p.Enabled, &p.ClientID, &p.ClientSecret, &p.IssuerURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProviderNotFound
	}
	if err != nil {
		return nil, err
	}
	p.HasSecret = p.ClientSecret != ""
	return &p, nil
}

func (s *OAuthService) findOrCreateUser(ctx context.Context, ext *oauth.ExternalUser) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(ext.Email))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var user models.User
	err = tx.QueryRow(ctx, `
		SELECT u.id, u.email, COALESCE(u.password_hash, ''), u.role, u.created_at, u.updated_at
		FROM _oauth_accounts oa
		JOIN _users u ON u.id = oa.user_id
		WHERE oa.provider = $1 AND oa.provider_user_id = $2`,
		ext.Provider, ext.ProviderUserID,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err == nil {
		return &user, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO _users (email)
		VALUES ($1)
		ON CONFLICT (email) DO UPDATE SET updated_at = NOW()
		RETURNING id, email, COALESCE(password_hash, ''), role, created_at, updated_at`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert oauth user: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO _oauth_accounts (user_id, provider, provider_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider, provider_user_id) DO NOTHING`,
		user.ID, ext.Provider, ext.ProviderUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("link oauth account: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &user, nil
}

func oauthStateKey(state string) string { return "oauth_state:" + state }
func oauthCodeKey(code string) string   { return "oauth_code:" + code }

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
