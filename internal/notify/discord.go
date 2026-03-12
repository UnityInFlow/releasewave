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

// DiscordNotifier sends release notifications to a Discord webhook.
type DiscordNotifier struct {
	httpClient *http.Client
	webhookURL string
}

// NewDiscordNotifier creates a Discord webhook notifier.
func NewDiscordNotifier(webhookURL string) *DiscordNotifier {
	return &DiscordNotifier{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		webhookURL: webhookURL,
	}
}

// Notify sends an embed message to Discord.
func (d *DiscordNotifier) Notify(ctx context.Context, event Event) error {
	slog.Debug("notify.discord", "service", event.ServiceName, "new_version", event.NewVersion)

	payload := map[string]any{
		"username": "ReleaseWave",
		"embeds": []map[string]any{
			{
				"title":       fmt.Sprintf("New Release: %s", event.ServiceName),
				"url":         event.ReleaseURL,
				"color":       3066993, // Green
				"description": fmt.Sprintf("**%s** → **%s**", event.OldVersion, event.NewVersion),
				"fields": []map[string]any{
					{"name": "Service", "value": event.ServiceName, "inline": true},
					{"name": "Platform", "value": event.Platform, "inline": true},
					{"name": "Previous", "value": event.OldVersion, "inline": true},
					{"name": "New Version", "value": event.NewVersion, "inline": true},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord webhook request: %w", err)
	}
	defer resp.Body.Close()

	// Discord returns 204 on success.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned HTTP %d", resp.StatusCode)
	}

	slog.Info("notify.discord.sent", "service", event.ServiceName, "new_version", event.NewVersion)
	return nil
}
