package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackNotifier_Success(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL)
	event := Event{
		ServiceName: "api",
		OldVersion:  "v1.0.0",
		NewVersion:  "v2.0.0",
		ReleaseURL:  "https://github.com/org/api/releases/tag/v2.0.0",
		Platform:    "github",
	}

	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	blocks, ok := received["blocks"]
	if !ok {
		t.Fatal("expected 'blocks' in Slack payload")
	}
	arr, ok := blocks.([]any)
	if !ok || len(arr) == 0 {
		t.Fatal("expected non-empty blocks array")
	}
}

func TestSlackNotifier_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL)
	err := notifier.Notify(context.Background(), Event{ServiceName: "api", NewVersion: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
