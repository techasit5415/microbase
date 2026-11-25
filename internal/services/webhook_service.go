package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Webhook represents an outbound notification hook (generic HTTP, Discord, or Firebase FCM).
type Webhook struct {
	ID         int64     `json:"id"`
	Provider   string    `json:"provider"`   // "generic", "discord", or "fcm"
	Collection string    `json:"collection"` // Collection name or "*"
	Event      string    `json:"event"`      // "create", "update", "delete", or "*"
	URL        string    `json:"url"`
	HasToken   bool      `json:"has_token"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// WebhookService manages outbound webhooks and dispatches event payloads asynchronously.
type WebhookService struct {
	pool   *pgxpool.Pool
	client *http.Client
}

func NewWebhookService(pool *pgxpool.Pool) *WebhookService {
	return &WebhookService{
		pool:   pool,
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

func (s *WebhookService) List(ctx context.Context) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, provider, collection, event, url, auth_token, enabled, created_at
		FROM _webhooks ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]Webhook, 0)
	for rows.Next() {
		var w Webhook
		var tok string
		if err := rows.Scan(&w.ID, &w.Provider, &w.Collection, &w.Event, &w.URL, &tok, &w.Enabled, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.HasToken = tok != ""
		list = append(list, w)
	}
	return list, rows.Err()
}

func (s *WebhookService) Create(ctx context.Context, provider, col, event, targetURL, authToken string) (*Webhook, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "generic"
	}
	if provider != "generic" && provider != "discord" && provider != "fcm" {
		return nil, fmt.Errorf("%w: provider must be generic, discord, or fcm", ErrInvalidInput)
	}
	col = strings.TrimSpace(col)
	event = strings.TrimSpace(event)
	targetURL = strings.TrimSpace(targetURL)
	authToken = strings.TrimSpace(authToken)

	if col == "" || event == "" || (!strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://")) {
		return nil, fmt.Errorf("%w: collection, event, and valid http(s) url required", ErrInvalidInput)
	}

	var w Webhook
	var tok string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO _webhooks (provider, collection, event, url, auth_token)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, provider, collection, event, url, auth_token, enabled, created_at`,
		provider, col, event, targetURL, authToken,
	).Scan(&w.ID, &w.Provider, &w.Collection, &w.Event, &w.URL, &tok, &w.Enabled, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	w.HasToken = tok != ""
	return &w, nil
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

// DispatchAsync fires matching webhooks (Generic, Discord Embed, or Firebase FCM) in background goroutines.
func (s *WebhookService) DispatchAsync(collection, event string, record any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		rows, err := s.pool.Query(ctx, `
			SELECT provider, url, auth_token FROM _webhooks
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
			var provider, targetURL, authToken string
			if rows.Scan(&provider, &targetURL, &authToken) != nil {
				continue
			}
			body := buildWebhookPayload(provider, collection, event, record)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			if authToken != "" {
				if strings.HasPrefix(strings.ToLower(authToken), "bearer ") || strings.HasPrefix(strings.ToLower(authToken), "key=") {
					req.Header.Set("Authorization", authToken)
				} else {
					req.Header.Set("Authorization", "Bearer "+authToken)
				}
			}
			resp, err := s.client.Do(req)
			if err != nil {
				slog.Warn("webhook delivery failed", "provider", provider, "url", targetURL, "error", err)
				continue
			}
			_ = resp.Body.Close()
		}
	}()
}

func buildWebhookPayload(provider, collection, event string, record any) []byte {
	prettyRecord, _ := json.MarshalIndent(record, "", "  ")
	recStr := string(prettyRecord)
	if len(recStr) > 1500 {
		recStr = recStr[:1500] + "..."
	}

	switch provider {
	case "discord":
		color := 5763719 // green for create
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
		// Compatible with Firebase Cloud Messaging HTTP v1 API (POST https://fcm.googleapis.com/v1/projects/<id>/messages:send)
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
