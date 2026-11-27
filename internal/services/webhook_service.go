package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Webhook represents an outbound notification hook (generic HTTP, Discord, or Firebase FCM).
type Webhook struct {
	ID         int64             `json:"id"`
	Provider   string            `json:"provider"`   // "generic", "discord", or "fcm"
	Collection string            `json:"collection"` // Collection name or "*"
	Event      string            `json:"event"`      // "create", "update", "delete", or "*"
	URL        string            `json:"url"`
	HasToken   bool              `json:"has_token"`
	HasSecret  bool              `json:"has_secret"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxRetries int               `json:"max_retries"`
	Enabled    bool              `json:"enabled"`
	LastRunAt  *time.Time        `json:"last_run_at,omitempty"`
	LastStatus string            `json:"last_status,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// CreateWebhookInput holds parameters for creating a new webhook.
type CreateWebhookInput struct {
	Provider   string            `json:"provider"`
	Collection string            `json:"collection"`
	Event      string            `json:"event"`
	URL        string            `json:"url"`
	AuthToken  string            `json:"auth_token"`
	Secret     string            `json:"secret"`
	Headers    map[string]string `json:"headers"`
	MaxRetries int               `json:"max_retries"`
}

// WebhookService manages outbound webhooks with HMAC-SHA256 signing, custom headers, retries, and test delivery.
type WebhookService struct {
	pool   *pgxpool.Pool
	client *http.Client
}

func NewWebhookService(pool *pgxpool.Pool) *WebhookService {
	return &WebhookService{
		pool:   pool,
		client: NewSafeHTTPClient(8 * time.Second),
	}
}

func (s *WebhookService) List(ctx context.Context) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, provider, collection, event, url, auth_token, secret, headers, max_retries, enabled, last_run_at, last_status, created_at
		FROM _webhooks ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]Webhook, 0)
	for rows.Next() {
		var w Webhook
		var tok, sec string
		var rawHeaders []byte
		if err := rows.Scan(&w.ID, &w.Provider, &w.Collection, &w.Event, &w.URL, &tok, &sec, &rawHeaders, &w.MaxRetries, &w.Enabled, &w.LastRunAt, &w.LastStatus, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.HasToken = tok != ""
		w.HasSecret = sec != ""
		_ = json.Unmarshal(rawHeaders, &w.Headers)
		list = append(list, w)
	}
	return list, rows.Err()
}

func (s *WebhookService) Create(ctx context.Context, in CreateWebhookInput) (*Webhook, error) {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider == "" || provider == "webhook" {
		provider = "generic"
	}
	if provider != "generic" && provider != "discord" && provider != "fcm" {
		return nil, fmt.Errorf("%w: provider must be generic, discord, or fcm", ErrInvalidInput)
	}
	col := strings.TrimSpace(in.Collection)
	event := strings.TrimSpace(in.Event)
	targetURL := strings.TrimSpace(in.URL)
	authToken := strings.TrimSpace(in.AuthToken)
	secret := strings.TrimSpace(in.Secret)
	maxRetries := in.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if maxRetries > 5 {
		maxRetries = 5
	}
	if in.Headers == nil {
		in.Headers = map[string]string{}
	}
	headersJSON, _ := json.Marshal(in.Headers)

	if col == "" || event == "" || (!strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://")) {
		return nil, fmt.Errorf("%w: collection, event, and valid http(s) url required", ErrInvalidInput)
	}

	var w Webhook
	var tok, sec string
	var rawHeaders []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO _webhooks (provider, collection, event, url, auth_token, secret, headers, max_retries)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, provider, collection, event, url, auth_token, secret, headers, max_retries, enabled, last_run_at, last_status, created_at`,
		provider, col, event, targetURL, authToken, secret, headersJSON, maxRetries,
	).Scan(&w.ID, &w.Provider, &w.Collection, &w.Event, &w.URL, &tok, &sec, &rawHeaders, &w.MaxRetries, &w.Enabled, &w.LastRunAt, &w.LastStatus, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	w.HasToken = tok != ""
	w.HasSecret = sec != ""
	_ = json.Unmarshal(rawHeaders, &w.Headers)
	return &w, nil
}

func (s *WebhookService) Toggle(ctx context.Context, id int64, enabled bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE _webhooks SET enabled = $2 WHERE id = $1`, id, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (s *WebhookService) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _webhooks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// Test sends an immediate test payload to the webhook and persists last_run_at & last_status.
func (s *WebhookService) Test(ctx context.Context, id int64) (string, error) {
	var provider, col, event, targetURL, authToken, secret string
	var rawHeaders []byte
	var maxRetries int
	err := s.pool.QueryRow(ctx, `
		SELECT provider, collection, event, url, auth_token, secret, headers, max_retries
		FROM _webhooks WHERE id = $1`, id,
	).Scan(&provider, &col, &event, &targetURL, &authToken, &secret, &rawHeaders, &maxRetries)
	if err != nil {
		return "", ErrRecordNotFound
	}
	var customHeaders map[string]string
	_ = json.Unmarshal(rawHeaders, &customHeaders)

	sampleRec := map[string]any{
		"id":      "test_webhook_ping",
		"message": "Hello from Microbase Webhook Test!",
		"test":    true,
	}
	return s.deliverWithRetry(ctx, id, provider, col, "test", targetURL, authToken, secret, customHeaders, 1, sampleRec)
}

// ComputeHMACSignature returns "sha256=<hex>" for verifying X-Microbase-Signature.
func ComputeHMACSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// DispatchAsync fires matching enabled webhooks in background goroutines with automatic retries.
func (s *WebhookService) DispatchAsync(collection, event string, record any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		rows, err := s.pool.Query(ctx, `
			SELECT id, provider, url, auth_token, secret, headers, max_retries FROM _webhooks
			WHERE enabled = TRUE
			  AND (collection = $1 OR collection = '*')
			  AND (event = $2 OR event = '*')`,
			collection, event,
		)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var id int64
			var provider, targetURL, authToken, secret string
			var rawHeaders []byte
			var maxRetries int
			if rows.Scan(&id, &provider, &targetURL, &authToken, &secret, &rawHeaders, &maxRetries) != nil {
				continue
			}
			var customHeaders map[string]string
			_ = json.Unmarshal(rawHeaders, &customHeaders)
			if maxRetries <= 0 {
				maxRetries = 3
			}
			go func(wID int64, prov, u, tok, sec string, hdrs map[string]string, retries int) {
				_, _ = s.deliverWithRetry(context.Background(), wID, prov, collection, event, u, tok, sec, hdrs, retries, record)
			}(id, provider, targetURL, authToken, secret, customHeaders, maxRetries)
		}
	}()
}

func (s *WebhookService) deliverWithRetry(
	ctx context.Context,
	id int64,
	provider, collection, event, targetURL, authToken, secret string,
	customHeaders map[string]string,
	maxAttempts int,
	record any,
) (string, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	body := buildWebhookPayload(provider, collection, event, record)
	backoffs := []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond, 3 * time.Second, 5 * time.Second}

	var lastStatus string
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL, bytes.NewReader(body))
		if err != nil {
			cancel()
			lastErr = err
			lastStatus = "error: " + err.Error()
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Microbase-Webhook/1.0")
		req.Header.Set("X-Microbase-Event", event)
		req.Header.Set("X-Microbase-Collection", collection)
		if secret != "" {
			req.Header.Set("X-Microbase-Signature", ComputeHMACSignature(body, secret))
		}
		if authToken != "" {
			if strings.HasPrefix(strings.ToLower(authToken), "bearer ") || strings.HasPrefix(strings.ToLower(authToken), "key=") {
				req.Header.Set("Authorization", authToken)
			} else {
				req.Header.Set("Authorization", "Bearer "+authToken)
			}
		}
		for k, v := range customHeaders {
			if strings.TrimSpace(k) != "" {
				req.Header.Set(strings.TrimSpace(k), v)
			}
		}

		resp, err := s.client.Do(req)
		cancel()
		if err != nil {
			lastErr = err
			lastStatus = fmt.Sprintf("error (attempt %d/%d): %v", attempt, maxAttempts, err)
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				lastErr = nil
				lastStatus = fmt.Sprintf("ok (HTTP %d)", resp.StatusCode)
				break
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			lastStatus = fmt.Sprintf("error (HTTP %d, attempt %d/%d)", resp.StatusCode, attempt, maxAttempts)
			// Do not retry 4xx client errors
			if resp.StatusCode < 500 {
				break
			}
		}

		if attempt < maxAttempts {
			delay := backoffs[min(attempt-1, len(backoffs)-1)]
			time.Sleep(delay)
		}
	}

	if lastErr != nil {
		slog.Warn("webhook delivery failed", "id", id, "provider", provider, "url", targetURL, "status", lastStatus)
	}
	if len(lastStatus) > 250 {
		lastStatus = lastStatus[:250]
	}
	_, _ = s.pool.Exec(context.Background(),
		`UPDATE _webhooks SET last_run_at = NOW(), last_status = $2 WHERE id = $1`,
		id, lastStatus,
	)
	return lastStatus, lastErr
}

func buildWebhookPayload(provider, collection, event string, record any) []byte {
	prettyRecord, _ := json.MarshalIndent(record, "", "  ")
	recStr := string(prettyRecord)
	if len(recStr) > 1500 {
		recStr = recStr[:1500] + "..."
	}

	switch provider {
	case "discord":
		color := 5763719 // green for create/test
		if event == "update" {
			color = 16776960 // yellow
		} else if event == "delete" {
			color = 15548997 // red
		}
		b, _ := json.Marshal(map[string]any{
			"username": "Microbase",
			"embeds": []map[string]any{
				{
					"title":       fmt.Sprintf("[%s] %s", strings.ToUpper(event), collection),
					"description": fmt.Sprintf("```json\n%s\n```", recStr),
					"color":       color,
					"timestamp":   time.Now().UTC().Format(time.RFC3339),
				},
			},
		})
		return b

	case "fcm":
		b, _ := json.Marshal(map[string]any{
			"message": map[string]any{
				"topic": collection,
				"notification": map[string]any{
					"title": fmt.Sprintf("Microbase: %s on %s", event, collection),
					"body":  recStr,
				},
				"data": map[string]any{
					"event":      event,
					"collection": collection,
				},
			},
		})
		return b

	default:
		b, _ := json.Marshal(map[string]any{
			"event":      event,
			"collection": collection,
			"record":     record,
			"timestamp":  time.Now().UTC(),
		})
		return b
	}
}
