package store

import (
	"fmt"
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

// ---------------------------------------------------------------------------
// Additional tests to improve coverage
// ---------------------------------------------------------------------------

func TestNew_InvalidPath(t *testing.T) {
	// Attempt to create a database at a path that cannot exist.
	_, err := New("/nonexistent_dir_xyz/sub/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "close_test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Operations after close should return an error.
	if err := s.SetKV("k", "v"); err == nil {
		t.Error("expected error on SetKV after Close")
	}
}

func TestGetKV_EmptyValue(t *testing.T) {
	s := testStore(t)

	// Store an empty string value.
	if err := s.SetKV("empty_key", ""); err != nil {
		t.Fatalf("SetKV empty: %v", err)
	}

	val, found, err := s.GetKV("empty_key")
	if err != nil {
		t.Fatalf("GetKV: %v", err)
	}
	if !found {
		t.Error("expected key to be found")
	}
	if val != "" {
		t.Errorf("value = %q, want empty string", val)
	}
}

func TestGetKV_AfterClose(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "kv_close.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Close()

	_, _, err = s.GetKV("anything")
	if err == nil {
		t.Error("expected error on GetKV after Close")
	}
}

func TestSetKV_MultipleKeys(t *testing.T) {
	s := testStore(t)

	keys := map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
	}
	for k, v := range keys {
		if err := s.SetKV(k, v); err != nil {
			t.Fatalf("SetKV(%q, %q): %v", k, v, err)
		}
	}

	for k, want := range keys {
		got, found, err := s.GetKV(k)
		if err != nil {
			t.Fatalf("GetKV(%q): %v", k, err)
		}
		if !found {
			t.Errorf("key %q not found", k)
		}
		if got != want {
			t.Errorf("GetKV(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestSetKV_UpdatePreservesOtherKeys(t *testing.T) {
	s := testStore(t)

	if err := s.SetKV("k1", "original"); err != nil {
		t.Fatalf("SetKV: %v", err)
	}
	if err := s.SetKV("k2", "untouched"); err != nil {
		t.Fatalf("SetKV: %v", err)
	}

	// Update k1, k2 should remain.
	if err := s.SetKV("k1", "updated"); err != nil {
		t.Fatalf("SetKV update: %v", err)
	}

	val, found, err := s.GetKV("k2")
	if err != nil {
		t.Fatalf("GetKV: %v", err)
	}
	if !found || val != "untouched" {
		t.Errorf("k2 = %q (found=%v), want %q", val, found, "untouched")
	}
}

func TestRecordRelease_DuplicateIgnored(t *testing.T) {
	s := testStore(t)

	now := time.Now().Truncate(time.Second)
	r := Release{
		Service:      "svc",
		Tag:          "v1.0.0",
		Platform:     "github",
		URL:          "https://example.com/v1",
		PublishedAt:  now,
		DiscoveredAt: now,
	}

	if err := s.RecordRelease(r); err != nil {
		t.Fatalf("RecordRelease: %v", err)
	}

	// Insert duplicate with different platform/URL -- should be ignored due to UNIQUE(service, tag).
	r2 := r
	r2.Platform = "gitlab"
	r2.URL = "https://gitlab.com/v1"
	if err := s.RecordRelease(r2); err != nil {
		t.Fatalf("RecordRelease duplicate: %v", err)
	}

	history, err := s.GetHistory("svc", 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 release after duplicate, got %d", len(history))
	}
	// Original record should be preserved (INSERT OR IGNORE keeps the first).
	if history[0].Platform != "github" {
		t.Errorf("platform = %q, want %q", history[0].Platform, "github")
	}
}

func TestRecordRelease_MultipleServices(t *testing.T) {
	s := testStore(t)

	now := time.Now().Truncate(time.Second)
	services := []string{"api", "web", "worker"}
	for _, svc := range services {
		r := Release{
			Service:      svc,
			Tag:          "v1.0.0",
			Platform:     "github",
			URL:          "https://example.com/" + svc,
			PublishedAt:  now,
			DiscoveredAt: now,
		}
		if err := s.RecordRelease(r); err != nil {
			t.Fatalf("RecordRelease(%s): %v", svc, err)
		}
	}

	// Each service should have exactly 1 release.
	for _, svc := range services {
		h, err := s.GetHistory(svc, 10)
		if err != nil {
			t.Fatalf("GetHistory(%s): %v", svc, err)
		}
		if len(h) != 1 {
			t.Errorf("GetHistory(%s): got %d releases, want 1", svc, len(h))
		}
	}
}

func TestRecordRelease_AllFieldsPersisted(t *testing.T) {
	s := testStore(t)

	now := time.Now().Truncate(time.Second)
	r := Release{
		Service:      "myservice",
		Tag:          "v2.3.4",
		Platform:     "npm",
		URL:          "https://npmjs.com/package/myservice/v/2.3.4",
		PublishedAt:  now.Add(-1 * time.Hour),
		DiscoveredAt: now,
	}

	if err := s.RecordRelease(r); err != nil {
		t.Fatalf("RecordRelease: %v", err)
	}

	history, err := s.GetHistory("myservice", 1)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 release, got %d", len(history))
	}

	got := history[0]
	if got.Service != r.Service {
		t.Errorf("Service = %q, want %q", got.Service, r.Service)
	}
	if got.Tag != r.Tag {
		t.Errorf("Tag = %q, want %q", got.Tag, r.Tag)
	}
	if got.Platform != r.Platform {
		t.Errorf("Platform = %q, want %q", got.Platform, r.Platform)
	}
	if got.URL != r.URL {
		t.Errorf("URL = %q, want %q", got.URL, r.URL)
	}
}

func TestRecordRelease_AfterClose(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "release_close.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Close()

	err = s.RecordRelease(Release{Service: "x", Tag: "v1"})
	if err == nil {
		t.Error("expected error on RecordRelease after Close")
	}
}

func TestGetHistory_NegativeLimit(t *testing.T) {
	s := testStore(t)

	now := time.Now().Truncate(time.Second)
	if err := s.RecordRelease(Release{
		Service: "svc", Tag: "v1.0.0", DiscoveredAt: now,
	}); err != nil {
		t.Fatalf("RecordRelease: %v", err)
	}

	// Negative limit should default to 50, and still return the 1 record.
	history, err := s.GetHistory("svc", -1)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 release, got %d", len(history))
	}
}

func TestGetHistory_Ordering(t *testing.T) {
	s := testStore(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	tags := []string{"v1.0.0", "v2.0.0", "v3.0.0"}
	for i, tag := range tags {
		r := Release{
			Service:      "api",
			Tag:          tag,
			DiscoveredAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := s.RecordRelease(r); err != nil {
			t.Fatalf("RecordRelease(%s): %v", tag, err)
		}
	}

	history, err := s.GetHistory("api", 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 releases, got %d", len(history))
	}

	// Newest first (discovered_at DESC).
	if history[0].Tag != "v3.0.0" {
		t.Errorf("first = %q, want v3.0.0", history[0].Tag)
	}
	if history[1].Tag != "v2.0.0" {
		t.Errorf("second = %q, want v2.0.0", history[1].Tag)
	}
	if history[2].Tag != "v1.0.0" {
		t.Errorf("third = %q, want v1.0.0", history[2].Tag)
	}
}

func TestGetHistory_LimitEnforced(t *testing.T) {
	s := testStore(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		r := Release{
			Service:      "api",
			Tag:          fmt.Sprintf("v1.0.%d", i),
			DiscoveredAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := s.RecordRelease(r); err != nil {
			t.Fatalf("RecordRelease: %v", err)
		}
	}

	history, err := s.GetHistory("api", 2)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("expected 2 releases with limit=2, got %d", len(history))
	}
}

func TestGetHistory_NoResultsForUnknownService(t *testing.T) {
	s := testStore(t)

	history, err := s.GetHistory("unknown_service", 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 releases for unknown service, got %d", len(history))
	}
}

func TestGetHistory_AfterClose(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "hist_close.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Close()

	_, err = s.GetHistory("api", 10)
	if err == nil {
		t.Error("expected error on GetHistory after Close")
	}
}

func TestLogToolCall_ErrorStatus(t *testing.T) {
	s := testStore(t)

	tc := ToolCall{
		Tool:       "fetch_releases",
		Args:       `{"owner":"org","repo":"broken"}`,
		Status:     "error",
		DurationMs: 5000,
		CalledAt:   time.Now(),
	}

	if err := s.LogToolCall(tc); err != nil {
		t.Fatalf("LogToolCall: %v", err)
	}
}

func TestLogToolCall_EmptyArgs(t *testing.T) {
	s := testStore(t)

	tc := ToolCall{
		Tool:       "ping",
		Args:       "",
		Status:     "ok",
		DurationMs: 0,
		CalledAt:   time.Now(),
	}

	if err := s.LogToolCall(tc); err != nil {
		t.Fatalf("LogToolCall: %v", err)
	}
}

func TestLogToolCall_ZeroDuration(t *testing.T) {
	s := testStore(t)

	tc := ToolCall{
		Tool:       "noop",
		Args:       "{}",
		Status:     "ok",
		DurationMs: 0,
		CalledAt:   time.Now(),
	}

	if err := s.LogToolCall(tc); err != nil {
		t.Fatalf("LogToolCall: %v", err)
	}
}

func TestLogToolCall_MultipleCalls(t *testing.T) {
	s := testStore(t)

	statuses := []string{"ok", "error", "timeout", "ok"}
	for i, status := range statuses {
		tc := ToolCall{
			Tool:       "tool_" + status,
			Args:       "{}",
			Status:     status,
			DurationMs: int64(i * 100),
			CalledAt:   time.Now(),
		}
		if err := s.LogToolCall(tc); err != nil {
			t.Fatalf("LogToolCall(%d): %v", i, err)
		}
	}
}

func TestLogToolCall_AfterClose(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "tc_close.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Close()

	err = s.LogToolCall(ToolCall{Tool: "x", CalledAt: time.Now()})
	if err == nil {
		t.Error("expected error on LogToolCall after Close")
	}
}

func TestNew_ReopenExistingDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reopen.db")

	// Create and populate.
	s1, err := New(path)
	if err != nil {
		t.Fatalf("New (first): %v", err)
	}
	if err := s1.SetKV("persist", "yes"); err != nil {
		t.Fatalf("SetKV: %v", err)
	}
	s1.Close()

	// Reopen and verify data persists.
	s2, err := New(path)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	defer s2.Close()

	val, found, err := s2.GetKV("persist")
	if err != nil {
		t.Fatalf("GetKV: %v", err)
	}
	if !found {
		t.Error("expected key to persist across reopen")
	}
	if val != "yes" {
		t.Errorf("value = %q, want %q", val, "yes")
	}
}

func TestRecordRelease_MinimalFields(t *testing.T) {
	s := testStore(t)

	// Only required fields (service + tag); platform, url are empty defaults.
	r := Release{
		Service:      "minimal",
		Tag:          "v0.0.1",
		DiscoveredAt: time.Now().Truncate(time.Second),
	}

	if err := s.RecordRelease(r); err != nil {
		t.Fatalf("RecordRelease: %v", err)
	}

	history, err := s.GetHistory("minimal", 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 release, got %d", len(history))
	}
	if history[0].Platform != "" {
		t.Errorf("platform = %q, want empty", history[0].Platform)
	}
	if history[0].URL != "" {
		t.Errorf("url = %q, want empty", history[0].URL)
	}
}

func TestSetKV_AfterClose(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "setkv_close.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Close()

	err = s.SetKV("key", "value")
	if err == nil {
		t.Error("expected error on SetKV after Close")
	}
}
