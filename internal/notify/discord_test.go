package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscordNotifier_Success(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	notifier := NewDiscordNotifier(srv.URL)
	event := Event{
		ServiceName: "billing",
		OldVersion:  "v3.0.0",
		NewVersion:  "v3.1.0",
		ReleaseURL:  "https://github.com/org/billing/releases/tag/v3.1.0",
		Platform:    "github",
	}

	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received["username"] != "ReleaseWave" {
		t.Errorf("username = %v, want 'ReleaseWave'", received["username"])
	}
	embeds, ok := received["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatal("expected non-empty embeds array")
	}
}

func TestDiscordNotifier_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	notifier := NewDiscordNotifier(srv.URL)
	err := notifier.Notify(context.Background(), Event{ServiceName: "api", NewVersion: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
