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

func TestSlackNotifier_ConnectionError(t *testing.T) {
	notifier := NewSlackNotifier("http://localhost:1")
	event := Event{ServiceName: "api", NewVersion: "v1.0.0"}

	err := notifier.Notify(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for connection failure, got nil")
	}
}

func TestSlackNotifier_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := notifier.Notify(ctx, Event{ServiceName: "api", NewVersion: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestSlackNotifier_PayloadStructure(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL)
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

	blocks, ok := received["blocks"].([]any)
	if !ok {
		t.Fatal("expected blocks array")
	}
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks (header, section, actions), got %d", len(blocks))
	}

	// Verify header block.
	header, ok := blocks[0].(map[string]any)
	if !ok {
		t.Fatal("expected header block to be a map")
	}
	if header["type"] != "header" {
		t.Errorf("first block type = %v, want 'header'", header["type"])
	}

	// Verify section block with fields.
	section, ok := blocks[1].(map[string]any)
	if !ok {
		t.Fatal("expected section block to be a map")
	}
	if section["type"] != "section" {
		t.Errorf("second block type = %v, want 'section'", section["type"])
	}
	fields, ok := section["fields"].([]any)
	if !ok {
		t.Fatal("expected section fields array")
	}
	if len(fields) != 4 {
		t.Errorf("expected 4 fields, got %d", len(fields))
	}

	// Verify actions block with button.
	actions, ok := blocks[2].(map[string]any)
	if !ok {
		t.Fatal("expected actions block to be a map")
	}
	if actions["type"] != "actions" {
		t.Errorf("third block type = %v, want 'actions'", actions["type"])
	}
	elements, ok := actions["elements"].([]any)
	if !ok || len(elements) == 0 {
		t.Fatal("expected non-empty elements array in actions block")
	}
	btn, ok := elements[0].(map[string]any)
	if !ok {
		t.Fatal("expected button element to be a map")
	}
	if btn["url"] != event.ReleaseURL {
		t.Errorf("button url = %v, want %q", btn["url"], event.ReleaseURL)
	}
}

func TestSlackNotifier_EmptyFieldValues(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL)
	// Send event with minimal/empty fields to exercise formatter with edge-case input.
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

	// Should still produce valid blocks structure.
	blocks, ok := received["blocks"].([]any)
	if !ok || len(blocks) != 3 {
		t.Fatalf("expected 3 blocks even with empty fields, got %v", len(blocks))
	}
}

func TestSlackNotifier_HTTP403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL)
	err := notifier.Notify(context.Background(), Event{ServiceName: "api", NewVersion: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}
