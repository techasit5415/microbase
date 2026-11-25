package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCProvider supports both standard OIDC discovery and custom OAuth2/IAM endpoints with JSON claim mapping.
type OIDCProvider struct {
	cfg          *oauth2.Config
	verifier     *gooidc.IDTokenVerifier
	userInfoURL  string
	fieldMapping map[string]string
}

// NewOIDCProvider builds an OIDC/Custom OAuth2 provider from manual endpoints or IssuerURL discovery.
func NewOIDCProvider(ctx context.Context, pc ProviderConfig, redirectURL string) (*OIDCProvider, error) {
	scopes := strings.Fields(strings.ReplaceAll(pc.Scopes, ",", " "))
	if len(scopes) == 0 {
		scopes = []string{gooidc.ScopeOpenID, "email", "profile"}
	}

	var endpoint oauth2.Endpoint
	var verifier *gooidc.IDTokenVerifier
	userInfoURL := strings.TrimSpace(pc.UserInfoURL)

	if strings.TrimSpace(pc.AuthURL) != "" && strings.TrimSpace(pc.TokenURL) != "" {
		endpoint = oauth2.Endpoint{
			AuthURL:  strings.TrimSpace(pc.AuthURL),
			TokenURL: strings.TrimSpace(pc.TokenURL),
		}
	} else if strings.TrimSpace(pc.IssuerURL) != "" {
		provider, err := gooidc.NewProvider(ctx, strings.TrimSpace(pc.IssuerURL))
		if err != nil {
			return nil, fmt.Errorf("oidc discovery failed: %w", err)
		}
		endpoint = provider.Endpoint()
		verifier = provider.Verifier(&gooidc.Config{ClientID: pc.ClientID})
		if userInfoURL == "" {
			var disc struct {
				UserInfoEndpoint string `json:"userinfo_endpoint"`
			}
			if provider.Claims(&disc) == nil && disc.UserInfoEndpoint != "" {
				userInfoURL = disc.UserInfoEndpoint
			}
		}
	} else {
		return nil, fmt.Errorf("oidc provider requires either issuer_url or manual auth_url + token_url")
	}

	return &OIDCProvider{
		cfg: &oauth2.Config{
			ClientID:     pc.ClientID,
			ClientSecret: pc.ClientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     endpoint,
			Scopes:       scopes,
		},
		verifier:     verifier,
		userInfoURL:  userInfoURL,
		fieldMapping: pc.FieldMapping,
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

	claimsMap := make(map[string]any)

	// 1. Parse id_token claims if present and verifier is configured.
	if rawIDToken, ok := token.Extra("id_token").(string); ok && rawIDToken != "" && o.verifier != nil {
		if idToken, err := o.verifier.Verify(ctx, rawIDToken); err == nil {
			_ = idToken.Claims(&claimsMap)
			if _, hasSub := claimsMap["sub"]; !hasSub && idToken.Subject != "" {
				claimsMap["sub"] = idToken.Subject
			}
		}
	}

	// 2. Fetch UserInfo / Custom IAM JSON endpoint if configured.
	if o.userInfoURL != "" {
		client := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.userInfoURL, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+token.AccessToken)
			req.Header.Set("Accept", "application/json")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				var userInfo map[string]any
				if json.Unmarshal(body, &userInfo) == nil {
					for k, v := range userInfo {
						claimsMap[k] = v
					}
				}
			}
		}
	}

	// 3. Resolve sub (provider user ID), email, and any custom mapped fields using dot-paths.
	subPath := "sub"
	emailPath := "email"
	if p := strings.TrimSpace(o.fieldMapping["sub"]); p != "" {
		subPath = p
	}
	if p := strings.TrimSpace(o.fieldMapping["email"]); p != "" {
		emailPath = p
	}

	sub := valueToString(extractJSONPath(claimsMap, subPath))
	if sub == "" {
		sub = valueToString(extractJSONPath(claimsMap, "student_id"))
	}
	if sub == "" {
		sub = valueToString(extractJSONPath(claimsMap, "id"))
	}
	email := valueToString(extractJSONPath(claimsMap, emailPath))
	if email == "" && sub != "" {
		email = sub + "@oidc.local"
	}
	if sub == "" {
		return nil, fmt.Errorf("oidc response missing subject/id claim (%q)", subPath)
	}

	extra := make(map[string]any)
	for targetCol, srcPath := range o.fieldMapping {
		if targetCol == "sub" || targetCol == "email" || strings.TrimSpace(srcPath) == "" {
			continue
		}
		if val := extractJSONPath(claimsMap, srcPath); val != nil {
			extra[targetCol] = val
		}
	}

	return &ExternalUser{
		Provider:       "oidc",
		ProviderUserID: sub,
		Email:          email,
		ExtraFields:    extra,
		RawClaims:      claimsMap,
	}, nil
}

// extractJSONPath resolves dot-separated paths like "department.name" or "data.0.firstname_th".
func extractJSONPath(root map[string]any, path string) any {
	var cur any = root
	for _, seg := range strings.Split(path, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		switch node := cur.(type) {
		case map[string]any:
			var ok bool
			cur, ok = node[seg]
			if !ok {
				return nil
			}
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil
			}
			cur = node[idx]
		default:
			return nil
		}
	}
	return cur
}

func valueToString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}


// ErrClaimNotAllowed is returned when an OIDC/IAM user does not satisfy the provider's allowed_claims restrictions.
var ErrClaimNotAllowed = fmt.Errorf("access denied: user does not match allowed claim restrictions")

// CheckAllowedClaims verifies that the user's raw IAM/OIDC claims match every configured allowed_claims rule.
// Each key is a dot-separated JSON path (e.g. "department.name") and the value is a list of permitted values.
func CheckAllowedClaims(allowed map[string][]string, raw map[string]any) error {
	if len(allowed) == 0 {
		return nil
	}
	for path, allowedVals := range allowed {
		path = strings.TrimSpace(path)
		if path == "" || len(allowedVals) == 0 {
			continue
		}
		actual := strings.TrimSpace(valueToString(extractJSONPath(raw, path)))
		matched := false
		for _, want := range allowedVals {
			if strings.EqualFold(actual, strings.TrimSpace(want)) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w (%s=%q)", ErrClaimNotAllowed, path, actual)
		}
	}
	return nil
}
