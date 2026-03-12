package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeNotifier is a test helper that records calls and optionally returns an error.
type fakeNotifier struct {
	called bool
	event  Event
	err    error
}

func (f *fakeNotifier) Notify(_ context.Context, event Event) error {
	f.called = true
	f.event = event
	return f.err
}

func TestMultiNotifier_AllSucceed(t *testing.T) {
	n1 := &fakeNotifier{}
	n2 := &fakeNotifier{}
	n3 := &fakeNotifier{}

	multi := NewMultiNotifier(n1, n2, n3)
	event := Event{
		ServiceName: "api",
		OldVersion:  "v1.0.0",
		NewVersion:  "v2.0.0",
		ReleaseURL:  "https://example.com/release",
		Platform:    "github",
	}

	err := multi.Notify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for i, n := range []*fakeNotifier{n1, n2, n3} {
		if !n.called {
			t.Errorf("notifier %d was not called", i)
		}
		if n.event.ServiceName != "api" {
			t.Errorf("notifier %d: service = %q, want %q", i, n.event.ServiceName, "api")
		}
		if n.event.NewVersion != "v2.0.0" {
			t.Errorf("notifier %d: version = %q, want %q", i, n.event.NewVersion, "v2.0.0")
		}
	}
}

func TestMultiNotifier_OneFailsOthersContinue(t *testing.T) {
	n1 := &fakeNotifier{}
	n2 := &fakeNotifier{err: errors.New("slack is down")}
	n3 := &fakeNotifier{}

	multi := NewMultiNotifier(n1, n2, n3)
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	err := multi.Notify(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when one notifier fails, got nil")
	}

	if !strings.Contains(err.Error(), "slack is down") {
		t.Errorf("error should contain 'slack is down', got %q", err.Error())
	}

	// All notifiers should still have been called.
	if !n1.called {
		t.Error("notifier 1 should have been called")
	}
	if !n2.called {
		t.Error("notifier 2 should have been called")
	}
	if !n3.called {
		t.Error("notifier 3 should have been called")
	}
}

func TestMultiNotifier_AllFail(t *testing.T) {
	n1 := &fakeNotifier{err: errors.New("error-one")}
	n2 := &fakeNotifier{err: errors.New("error-two")}

	multi := NewMultiNotifier(n1, n2)
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	err := multi.Notify(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when all notifiers fail")
	}

	if !strings.Contains(err.Error(), "error-one") {
		t.Errorf("error should contain 'error-one', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "error-two") {
		t.Errorf("error should contain 'error-two', got %q", err.Error())
	}
}

func TestMultiNotifier_Empty(t *testing.T) {
	multi := NewMultiNotifier()
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	err := multi.Notify(context.Background(), event)
	if err != nil {
		t.Fatalf("expected nil error for empty notifier list, got %v", err)
	}
}

func TestMultiNotifier_SingleNotifier(t *testing.T) {
	n := &fakeNotifier{}
	multi := NewMultiNotifier(n)
	event := Event{ServiceName: "svc", NewVersion: "v3.0.0"}

	err := multi.Notify(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !n.called {
		t.Error("notifier should have been called")
	}
}

// --- FromConfig tests ---

func TestFromConfig_NoURLs_ReturnsNil(t *testing.T) {
	n := FromConfig("", "", "")
	if n != nil {
		t.Fatalf("expected nil for no config, got %T", n)
	}
}

func TestFromConfig_WebhookOnly(t *testing.T) {
	n := FromConfig("https://example.com/hook", "", "")
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	if _, ok := n.(*WebhookNotifier); !ok {
		t.Errorf("expected *WebhookNotifier, got %T", n)
	}
}

func TestFromConfig_SlackOnly(t *testing.T) {
	n := FromConfig("", "https://hooks.slack.com/services/xxx", "")
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	if _, ok := n.(*SlackNotifier); !ok {
		t.Errorf("expected *SlackNotifier, got %T", n)
	}
}

func TestFromConfig_DiscordOnly(t *testing.T) {
	n := FromConfig("", "", "https://discord.com/api/webhooks/xxx")
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	if _, ok := n.(*DiscordNotifier); !ok {
		t.Errorf("expected *DiscordNotifier, got %T", n)
	}
}

func TestFromConfig_TwoURLs_ReturnsMulti(t *testing.T) {
	n := FromConfig("https://example.com/hook", "https://hooks.slack.com/xxx", "")
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	if _, ok := n.(*MultiNotifier); !ok {
		t.Errorf("expected *MultiNotifier, got %T", n)
	}
}

func TestFromConfig_AllThreeURLs_ReturnsMulti(t *testing.T) {
	n := FromConfig("https://example.com/hook", "https://hooks.slack.com/xxx", "https://discord.com/api/webhooks/xxx")
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	multi, ok := n.(*MultiNotifier)
	if !ok {
		t.Fatalf("expected *MultiNotifier, got %T", n)
	}
	if len(multi.notifiers) != 3 {
		t.Errorf("expected 3 notifiers, got %d", len(multi.notifiers))
	}
}
