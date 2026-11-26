package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var urlTemplateRe = regexp.MustCompile(`\{([a-zA-Z0-9_.-]+)\}`)

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
			Scopes:       scopes,
			Endpoint:     endpoint,
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

	// 1. Parse id_token claims (verified if OIDC discovery verifier is configured,
	//    or unverified JWT payload fallback for manual OAuth2/IAM setups).
	if rawIDToken, ok := token.Extra("id_token").(string); ok && rawIDToken != "" {
		if o.verifier != nil {
			if idToken, err := o.verifier.Verify(ctx, rawIDToken); err == nil {
				_ = idToken.Claims(&claimsMap)
				if _, hasSub := claimsMap["sub"]; !hasSub && idToken.Subject != "" {
					claimsMap["sub"] = idToken.Subject
				}
			}
		}
		if len(claimsMap) == 0 {
			for k, v := range decodeJWTPayload(rawIDToken) {
				claimsMap[k] = v
			}
		}
	}

	// 1b. Also inspect access_token JWT payload and token response extras for user_id / sub
	//     (needed to interpolate placeholders like /users/{user_id}).
	for k, v := range decodeJWTPayload(token.AccessToken) {
		if _, exists := claimsMap[k]; !exists {
			claimsMap[k] = v
		}
	}
	for _, key := range []string{"user_id", "sub", "student_id", "id", "email"} {
		if _, exists := claimsMap[key]; !exists {
			if v := token.Extra(key); v != nil && valueToString(v) != "" {
				claimsMap[key] = v
			}
		}
	}

	// 2. Fetch UserInfo / Custom IAM JSON endpoint if configured (with {user_id}/{sub}/{student_id} interpolation).
	if o.userInfoURL != "" {
		resolvedURL := resolveUserInfoURL(o.userInfoURL, claimsMap)
		client := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolvedURL, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+token.AccessToken)
			req.Header.Set("Accept", "application/json")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				var userInfo map[string]any
				if json.Unmarshal(body, &userInfo) == nil {
					mergeUserInfoClaims(claimsMap, userInfo)
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

	sub := firstNonEmptyClaim(claimsMap, subPath, "id", "user_id", "student_id", "data.id", "sub")
	email := firstNonEmptyClaim(claimsMap, emailPath, "email", "data.email", "emailSecond", "data.emailSecond")
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

// decodeJWTPayload decodes the middle JSON payload of a JWT without signature verification
// (used only to extract subject/user_id hints for manual OAuth2 UserInfo URL templates).
func decodeJWTPayload(raw string) map[string]any {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 3 {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

// resolveUserInfoURL replaces placeholders like {user_id}, {sub}, {student_id}, or {data.id} in a UserInfo URL.
func resolveUserInfoURL(tmpl string, claims map[string]any) string {
	return urlTemplateRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{"), "}")
		var val string
		switch key {
		case "user_id":
			val = firstNonEmptyClaim(claims, "user_id", "sub", "id", "student_id")
		case "student_id":
			val = firstNonEmptyClaim(claims, "student_id", "sub", "user_id", "id")
		case "sub":
			val = firstNonEmptyClaim(claims, "sub", "user_id", "id", "student_id")
		case "email":
			val = firstNonEmptyClaim(claims, "email", "preferred_username", "data.email")
		default:
			val = valueToString(extractJSONPath(claims, key))
		}
		if val == "" {
			return match
		}
		return url.PathEscape(val)
	})
}

// mergeUserInfoClaims merges the UserInfo JSON object into claimsMap and also flattens
// a nested "data" object (e.g. KMITL IAM v2 /users/{user_id} responses) while preserving "data.*".
func mergeUserInfoClaims(claimsMap map[string]any, userInfo map[string]any) {
	for k, v := range userInfo {
		claimsMap[k] = v
	}
	if dataMap, ok := userInfo["data"].(map[string]any); ok {
		for k, v := range dataMap {
			claimsMap[k] = v
		}
	}
}

func firstNonEmptyClaim(claims map[string]any, paths ...string) string {
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if v := valueToString(extractJSONPath(claims, p)); v != "" {
			return v
		}
	}
	return ""
}

// extractJSONPath resolves dot-separated paths like "department.name" or "data.student_profile.0.department".
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
