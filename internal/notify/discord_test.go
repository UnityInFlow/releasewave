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

func TestDiscordNotifier_ConnectionError(t *testing.T) {
	notifier := NewDiscordNotifier("http://localhost:1")
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	err := notifier.Notify(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for connection failure, got nil")
	}
}

func TestDiscordNotifier_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	notifier := NewDiscordNotifier(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := notifier.Notify(ctx, Event{ServiceName: "api", NewVersion: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDiscordNotifier_PayloadStructure(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	notifier := NewDiscordNotifier(srv.URL)
	event := Event{
		ServiceName: "my-service",
		OldVersion:  "v1.0.0",
		NewVersion:  "v2.0.0",
		ReleaseURL:  "https://github.com/org/repo/releases/tag/v2.0.0",
		Platform:    "github",
	}

	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify username.
	if received["username"] != "ReleaseWave" {
		t.Errorf("username = %v, want 'ReleaseWave'", received["username"])
	}

	// Verify embeds structure.
	embeds, ok := received["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatalf("expected exactly 1 embed, got %v", embeds)
	}

	embed, ok := embeds[0].(map[string]any)
	if !ok {
		t.Fatal("expected embed to be a map")
	}

	// Verify embed title contains service name.
	title, ok := embed["title"].(string)
	if !ok {
		t.Fatal("expected title string")
	}
	if title != "New Release: my-service" {
		t.Errorf("title = %q, want 'New Release: my-service'", title)
	}

	// Verify embed URL.
	if embed["url"] != event.ReleaseURL {
		t.Errorf("url = %v, want %q", embed["url"], event.ReleaseURL)
	}

	// Verify color is green (3066993).
	color, ok := embed["color"].(float64) // JSON numbers decode as float64
	if !ok {
		t.Fatal("expected color as number")
	}
	if int(color) != 3066993 {
		t.Errorf("color = %v, want 3066993", color)
	}

	// Verify description contains version transition.
	desc, ok := embed["description"].(string)
	if !ok {
		t.Fatal("expected description string")
	}
	if desc != "**v1.0.0** \u2192 **v2.0.0**" {
		t.Errorf("description = %q, want '**v1.0.0** \u2192 **v2.0.0**'", desc)
	}

	// Verify fields.
	fields, ok := embed["fields"].([]any)
	if !ok {
		t.Fatal("expected fields array")
	}
	if len(fields) != 4 {
		t.Errorf("expected 4 fields, got %d", len(fields))
	}

	// Check first field details (Service).
	f0, ok := fields[0].(map[string]any)
	if !ok {
		t.Fatal("expected field 0 to be a map")
	}
	if f0["name"] != "Service" {
		t.Errorf("field 0 name = %v, want 'Service'", f0["name"])
	}
	if f0["value"] != "my-service" {
		t.Errorf("field 0 value = %v, want 'my-service'", f0["value"])
	}
	if f0["inline"] != true {
		t.Errorf("field 0 inline = %v, want true", f0["inline"])
	}
}

func TestDiscordNotifier_EmptyFieldValues(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	notifier := NewDiscordNotifier(srv.URL)
	event := Event{
		ServiceName: "",
		OldVersion:  "",
		NewVersion:  "",
		ReleaseURL:  "",
		Platform:    "",
	}

	if err := notifier.Notify(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	embeds, ok := received["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatalf("expected 1 embed even with empty fields, got %v", len(embeds))
	}
}

func TestDiscordNotifier_HTTP429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	notifier := NewDiscordNotifier(srv.URL)
	err := notifier.Notify(context.Background(), Event{ServiceName: "api", NewVersion: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}
