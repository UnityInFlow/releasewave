package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNew_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected database file to exist")
	}
}

func TestRecordRelease_AndGetHistory(t *testing.T) {
	s := testStore(t)

	now := time.Now().Truncate(time.Second)
	r := Release{
		Service:      "api",
		Tag:          "v1.0.0",
		Platform:     "github",
		URL:          "https://github.com/org/api/releases/tag/v1.0.0",
		PublishedAt:  now,
		DiscoveredAt: now,
	}

	if err := s.RecordRelease(r); err != nil {
		t.Fatalf("RecordRelease: %v", err)
	}

	// Insert duplicate should not error (IGNORE).
	if err := s.RecordRelease(r); err != nil {
		t.Fatalf("RecordRelease duplicate: %v", err)
	}

	history, err := s.GetHistory("api", 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("expected 1 release, got %d", len(history))
	}
	if history[0].Tag != "v1.0.0" {
		t.Errorf("tag = %q, want %q", history[0].Tag, "v1.0.0")
	}
}

func TestGetHistory_DefaultLimit(t *testing.T) {
	s := testStore(t)

	history, err := s.GetHistory("nonexistent", 0)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 releases, got %d", len(history))
	}
}

func TestLogToolCall(t *testing.T) {
	s := testStore(t)

	tc := ToolCall{
		Tool:       "list_releases",
		Args:       `{"owner":"org","repo":"api"}`,
		Status:     "ok",
		DurationMs: 150,
		CalledAt:   time.Now(),
	}

	if err := s.LogToolCall(tc); err != nil {
		t.Fatalf("LogToolCall: %v", err)
	}
}

func TestKVStore(t *testing.T) {
	s := testStore(t)

	// Get non-existent key.
	_, found, err := s.GetKV("missing")
	if err != nil {
		t.Fatalf("GetKV: %v", err)
	}
	if found {
		t.Error("expected not found for missing key")
	}

	// Set and get.
	if err := s.SetKV("version:api", "v1.0.0"); err != nil {
		t.Fatalf("SetKV: %v", err)
	}

	val, found, err := s.GetKV("version:api")
	if err != nil {
		t.Fatalf("GetKV: %v", err)
	}
	if !found {
		t.Error("expected key to be found")
	}
	if val != "v1.0.0" {
		t.Errorf("value = %q, want %q", val, "v1.0.0")
	}

	// Update.
	if err := s.SetKV("version:api", "v2.0.0"); err != nil {
		t.Fatalf("SetKV update: %v", err)
	}

	val, _, err = s.GetKV("version:api")
	if err != nil {
		t.Fatalf("GetKV: %v", err)
	}
	if val != "v2.0.0" {
		t.Errorf("value = %q, want %q", val, "v2.0.0")
	}
}
