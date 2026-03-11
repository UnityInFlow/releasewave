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
