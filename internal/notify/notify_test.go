package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookNotifier_Success(t *testing.T) {
	var received Event

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		if ua := r.Header.Get("User-Agent"); ua != "releasewave/1.0" {
			t.Errorf("expected releasewave/1.0 User-Agent, got %s", ua)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewWebhookNotifier(srv.URL)
	event := Event{
		ServiceName: "my-api",
		OldVersion:  "v1.0.0",
		NewVersion:  "v2.0.0",
		ReleaseURL:  "https://github.com/org/api/releases/tag/v2.0.0",
		Platform:    "github",
	}

	err := notifier.Notify(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.ServiceName != "my-api" {
		t.Errorf("service_name = %q, want %q", received.ServiceName, "my-api")
	}
	if received.NewVersion != "v2.0.0" {
		t.Errorf("new_version = %q, want %q", received.NewVersion, "v2.0.0")
	}
	if received.OldVersion != "v1.0.0" {
		t.Errorf("old_version = %q, want %q", received.OldVersion, "v1.0.0")
	}
}

func TestWebhookNotifier_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	notifier := NewWebhookNotifier(srv.URL)
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	err := notifier.Notify(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestWebhookNotifier_ConnectionError(t *testing.T) {
	notifier := NewWebhookNotifier("http://localhost:1")
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	err := notifier.Notify(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for connection failure, got nil")
	}
}

func TestWebhookNotifier_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewWebhookNotifier(srv.URL)
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := notifier.Notify(ctx, event)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestWebhookNotifier_HTTP403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	notifier := NewWebhookNotifier(srv.URL)
	err := notifier.Notify(context.Background(), Event{ServiceName: "api", NewVersion: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestWebhookNotifier_AllFieldsSent(t *testing.T) {
	var received Event

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewWebhookNotifier(srv.URL)
	event := Event{
		ServiceName: "billing-service",
		OldVersion:  "v3.2.1",
		NewVersion:  "v4.0.0",
		ReleaseURL:  "https://github.com/org/billing/releases/tag/v4.0.0",
		Platform:    "gitlab",
	}

	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.ServiceName != event.ServiceName {
		t.Errorf("service = %q, want %q", received.ServiceName, event.ServiceName)
	}
	if received.OldVersion != event.OldVersion {
		t.Errorf("old_version = %q, want %q", received.OldVersion, event.OldVersion)
	}
	if received.NewVersion != event.NewVersion {
		t.Errorf("new_version = %q, want %q", received.NewVersion, event.NewVersion)
	}
	if received.ReleaseURL != event.ReleaseURL {
		t.Errorf("release_url = %q, want %q", received.ReleaseURL, event.ReleaseURL)
	}
	if received.Platform != event.Platform {
		t.Errorf("platform = %q, want %q", received.Platform, event.Platform)
	}
}

func TestNewWebhookNotifier_SetsURL(t *testing.T) {
	url := "https://example.com/webhook"
	n := NewWebhookNotifier(url)
	if n.url != url {
		t.Errorf("url = %q, want %q", n.url, url)
	}
	if n.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}
