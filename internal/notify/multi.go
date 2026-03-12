package notify

import (
	"context"
	"errors"
	"log/slog"
)

// MultiNotifier sends notifications to multiple channels concurrently.
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier creates a notifier that fans out to all provided notifiers.
func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

// Notify sends the event to all notifiers. Returns joined errors.
func (m *MultiNotifier) Notify(ctx context.Context, event Event) error {
	var errs []error
	for _, n := range m.notifiers {
		if err := n.Notify(ctx, event); err != nil {
			slog.Error("notify.multi.error", "error", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// FromConfig constructs a Notifier from the notification configuration.
// Returns nil if no notification channels are configured.
func FromConfig(webhookURL, slackURL, discordURL string) Notifier {
	var notifiers []Notifier

	if webhookURL != "" {
		notifiers = append(notifiers, NewWebhookNotifier(webhookURL))
	}
	if slackURL != "" {
		notifiers = append(notifiers, NewSlackNotifier(slackURL))
	}
	if discordURL != "" {
		notifiers = append(notifiers, NewDiscordNotifier(discordURL))
	}

	switch len(notifiers) {
	case 0:
		return nil
	case 1:
		return notifiers[0]
	default:
		return NewMultiNotifier(notifiers...)
	}
}
