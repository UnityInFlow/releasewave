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

// SlackNotifier sends release notifications to a Slack Incoming Webhook.
type SlackNotifier struct {
	httpClient *http.Client
	webhookURL string
}

// NewSlackNotifier creates a Slack webhook notifier.
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		webhookURL: webhookURL,
	}
}

// Notify sends a Block Kit message to Slack.
func (s *SlackNotifier) Notify(ctx context.Context, event Event) error {
	slog.Debug("notify.slack", "service", event.ServiceName, "new_version", event.NewVersion)

	payload := map[string]any{
		"blocks": []map[string]any{
			{
				"type": "header",
				"text": map[string]string{
					"type": "plain_text",
					"text": fmt.Sprintf("🚀 New Release: %s", event.ServiceName),
				},
			},
			{
				"type": "section",
				"fields": []map[string]string{
					{"type": "mrkdwn", "text": fmt.Sprintf("*Service:*\n%s", event.ServiceName)},
					{"type": "mrkdwn", "text": fmt.Sprintf("*Platform:*\n%s", event.Platform)},
					{"type": "mrkdwn", "text": fmt.Sprintf("*Previous:*\n%s", event.OldVersion)},
					{"type": "mrkdwn", "text": fmt.Sprintf("*New:*\n%s", event.NewVersion)},
				},
			},
			{
				"type": "actions",
				"elements": []map[string]any{
					{
						"type": "button",
						"text": map[string]string{
							"type": "plain_text",
							"text": "View Release",
						},
						"url": event.ReleaseURL,
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned HTTP %d", resp.StatusCode)
	}

	slog.Info("notify.slack.sent", "service", event.ServiceName, "new_version", event.NewVersion)
	return nil
}
