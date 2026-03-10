// Package notify provides notification support for release events.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Event represents a release notification event.
type Event struct {
	ServiceName string `json:"service_name"`
	OldVersion  string `json:"old_version"`
	NewVersion  string `json:"new_version"`
	ReleaseURL  string `json:"release_url"`
	Platform    string `json:"platform"`
}

// Notifier sends notifications about release events.
type Notifier interface {
	Notify(ctx context.Context, event Event) error
}

// WebhookNotifier sends notifications by POSTing JSON to a URL.
type WebhookNotifier struct {
	httpClient *http.Client
	url        string
}

// NewWebhookNotifier creates a webhook notifier that POSTs to the given URL.
func NewWebhookNotifier(webhookURL string) *WebhookNotifier {
	return &WebhookNotifier{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		url:        webhookURL,
	}
}

// Notify sends a release event to the configured webhook URL.
func (w *WebhookNotifier) Notify(ctx context.Context, event Event) error {
	slog.Debug("notify.webhook", "service", event.ServiceName, "new_version", event.NewVersion, "url", w.url)

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "releasewave/1.0")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}

	slog.Info("notify.webhook.sent", "service", event.ServiceName, "new_version", event.NewVersion)
	return nil
}
