package daemon

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/model"
	"github.com/UnityInFlow/releasewave/internal/notify"
	"github.com/UnityInFlow/releasewave/internal/provider"
	"github.com/UnityInFlow/releasewave/internal/store"
)

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

type mockProvider struct {
	name    string
	release *model.Release
	err     error
	calls   atomic.Int64
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) ListReleases(_ context.Context, _, _ string) ([]model.Release, error) {
	return nil, nil
}
func (m *mockProvider) GetLatestRelease(_ context.Context, _, _ string) (*model.Release, error) {
	m.calls.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return m.release, nil
}
func (m *mockProvider) ListTags(_ context.Context, _, _ string) ([]model.Tag, error) {
	return nil, nil
}
func (m *mockProvider) GetFileContent(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

var _ provider.Provider = (*mockProvider)(nil)

// mockNotifier records calls to Notify.
type mockNotifier struct {
	mu     sync.Mutex
	events []notify.Event
	err    error
}

func (m *mockNotifier) Notify(_ context.Context, event notify.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return m.err
}

func (m *mockNotifier) getEvents() []notify.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]notify.Event, len(m.events))
	copy(cp, m.events)
	return cp
}

var _ notify.Notifier = (*mockNotifier)(nil)

// newTestStore creates a temporary SQLite store for testing. The caller should
// call cleanup() when done.
func newTestStore(t *testing.T) (*store.Store, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "daemon_test_*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := tmpFile.Name()
	tmpFile.Close()

	st, err := store.New(path)
	if err != nil {
		os.Remove(path)
		t.Fatalf("create store: %v", err)
	}
	return st, func() {
		st.Close()
		os.Remove(path)
	}
}

// ---------------------------------------------------------------------------
// Existing tests (preserved)
// ---------------------------------------------------------------------------

func TestDaemon_RunOnce(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag:     "v1.0.0",
			HTMLURL: "https://github.com/org/api/releases/tag/v1.0.0",
		},
	}

	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}

	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, 5*time.Minute)

	d.RunOnce(context.Background())

	if d.known["api"] != "v1.0.0" {
		t.Errorf("known version = %q, want %q", d.known["api"], "v1.0.0")
	}
}

func TestDaemon_StartStop(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag: "v1.0.0",
		},
	}

	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}

	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		d.Start(ctx)
		close(done)
	}()

	// Let it run at least one cycle.
	time.Sleep(50 * time.Millisecond)
	d.Stop()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop within timeout")
	}
}

// ---------------------------------------------------------------------------
// New tests for untested paths
// ---------------------------------------------------------------------------

// TestDaemon_StartContextCancel verifies that cancelling the context stops the
// daemon loop (the ctx.Done() branch).
func TestDaemon_StartContextCancel(t *testing.T) {
	mock := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, time.Hour) // long interval so ticker won't fire

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Start(ctx)
		close(done)
	}()

	// Give Start time to enter the for loop.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK - daemon exited via ctx.Done()
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop after context cancellation")
	}
}

// TestDaemon_StopMultipleCalls verifies Stop() is safe to call more than once
// (sync.Once behaviour).
func TestDaemon_StopMultipleCalls(t *testing.T) {
	mock := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Call Stop twice — should not panic or deadlock.
	d.Stop()
	d.Stop()
}

// TestCheckService_InvalidRepo covers the ParseRepo error path.
func TestCheckService_InvalidRepo(t *testing.T) {
	mock := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "bad", Repo: "invalid-repo"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, time.Hour)

	// Should not panic; the invalid repo is logged and skipped.
	d.RunOnce(context.Background())

	if _, ok := d.known["bad"]; ok {
		t.Error("expected 'bad' service NOT to be recorded in known map")
	}

	// Provider should not have been called.
	if got := mock.calls.Load(); got != 0 {
		t.Errorf("expected 0 provider calls, got %d", got)
	}
}

// TestCheckService_ProviderNotFound covers the unknown-platform branch.
func TestCheckService_ProviderNotFound(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "svc", Repo: "bitbucket.org/org/repo"},
		},
	}
	// No provider registered for "bitbucket.org".
	providers := map[string]provider.Provider{}
	d := New(cfg, providers, nil, nil, time.Hour)

	d.RunOnce(context.Background())

	if _, ok := d.known["svc"]; ok {
		t.Error("expected service NOT to be in known map when provider is missing")
	}
}

// TestCheckService_ProviderError covers the GetLatestRelease error branch.
func TestCheckService_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		err:  errors.New("API rate limit"),
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, time.Hour)

	d.RunOnce(context.Background())

	if _, ok := d.known["api"]; ok {
		t.Error("expected service NOT to be in known map when provider returns error")
	}
}

// TestCheckService_NewReleaseDetected covers the version-change notification
// path (seen && old != release.Tag).
func TestCheckService_NewReleaseDetected(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag:     "v2.0.0",
			HTMLURL: "https://github.com/org/api/releases/tag/v2.0.0",
		},
	}
	mn := &mockNotifier{}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, mn, nil, time.Hour)

	// Pre-seed known version so that the change is detected.
	d.known["api"] = "v1.0.0"

	d.poll(context.Background())

	if d.known["api"] != "v2.0.0" {
		t.Errorf("known version = %q, want %q", d.known["api"], "v2.0.0")
	}

	events := mn.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 notification event, got %d", len(events))
	}
	ev := events[0]
	if ev.OldVersion != "v1.0.0" {
		t.Errorf("event OldVersion = %q, want %q", ev.OldVersion, "v1.0.0")
	}
	if ev.NewVersion != "v2.0.0" {
		t.Errorf("event NewVersion = %q, want %q", ev.NewVersion, "v2.0.0")
	}
	if ev.ServiceName != "api" {
		t.Errorf("event ServiceName = %q, want %q", ev.ServiceName, "api")
	}
	if ev.Platform != "github" {
		t.Errorf("event Platform = %q, want %q", ev.Platform, "github")
	}
	if ev.ReleaseURL != "https://github.com/org/api/releases/tag/v2.0.0" {
		t.Errorf("event ReleaseURL = %q, want full URL", ev.ReleaseURL)
	}
}

// TestCheckService_SameVersionNoNotification verifies that no notification is
// sent when the version has not changed.
func TestCheckService_SameVersionNoNotification(t *testing.T) {
	mock := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	mn := &mockNotifier{}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, mn, nil, time.Hour)

	// Same version pre-seeded.
	d.known["api"] = "v1.0.0"

	d.poll(context.Background())

	events := mn.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 notification events for same version, got %d", len(events))
	}
}

// TestCheckService_FirstSeen verifies that the first time a service is seen
// (not previously in known map) no notification is sent.
func TestCheckService_FirstSeen(t *testing.T) {
	mock := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	mn := &mockNotifier{}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, mn, nil, time.Hour)

	// known map is empty — first discovery.
	d.poll(context.Background())

	events := mn.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 notification events for first-seen version, got %d", len(events))
	}
	if d.known["api"] != "v1.0.0" {
		t.Errorf("known version = %q, want %q", d.known["api"], "v1.0.0")
	}
}

// TestCheckService_NotifierError covers the notifier returning an error (the
// error is logged but does not crash).
func TestCheckService_NotifierError(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag:     "v2.0.0",
			HTMLURL: "https://github.com/org/api/releases/tag/v2.0.0",
		},
	}
	mn := &mockNotifier{err: errors.New("slack webhook failed")}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, mn, nil, time.Hour)

	d.known["api"] = "v1.0.0"

	// Should not panic even though notifier returns an error.
	d.poll(context.Background())

	if d.known["api"] != "v2.0.0" {
		t.Errorf("known version = %q, want %q", d.known["api"], "v2.0.0")
	}
}

// TestCheckService_NilNotifier covers the new-release path when notifier is nil.
func TestCheckService_NilNotifier(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag:     "v2.0.0",
			HTMLURL: "https://github.com/org/api/releases/tag/v2.0.0",
		},
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, time.Hour) // notifier is nil

	d.known["api"] = "v1.0.0"

	// Should not panic when notifier is nil and a new release is detected.
	d.poll(context.Background())

	if d.known["api"] != "v2.0.0" {
		t.Errorf("known version = %q, want %q", d.known["api"], "v2.0.0")
	}
}

// TestCheckService_WithStore verifies that SetKV and RecordRelease are called
// when a store is provided.
func TestCheckService_WithStore(t *testing.T) {
	st, cleanup := newTestStore(t)
	defer cleanup()

	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag:         "v1.0.0",
			HTMLURL:     "https://github.com/org/api/releases/tag/v1.0.0",
			PublishedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, st, time.Hour)

	d.RunOnce(context.Background())

	// Verify the version was persisted.
	val, found, err := st.GetKV("version:api")
	if err != nil {
		t.Fatalf("GetKV error: %v", err)
	}
	if !found {
		t.Fatal("expected version:api to be found in store")
	}
	if val != "v1.0.0" {
		t.Errorf("stored version = %q, want %q", val, "v1.0.0")
	}

	// Verify the release was recorded.
	releases, err := st.GetHistory("api", 10)
	if err != nil {
		t.Fatalf("GetHistory error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release record, got %d", len(releases))
	}
	if releases[0].Tag != "v1.0.0" {
		t.Errorf("recorded release tag = %q, want %q", releases[0].Tag, "v1.0.0")
	}
	if releases[0].Platform != "github" {
		t.Errorf("recorded release platform = %q, want %q", releases[0].Platform, "github")
	}
}

// TestLoadKnownVersions_WithStore verifies that loadKnownVersions populates
// the known map from the store.
func TestLoadKnownVersions_WithStore(t *testing.T) {
	st, cleanup := newTestStore(t)
	defer cleanup()

	// Pre-populate the store.
	if err := st.SetKV("version:api", "v3.0.0"); err != nil {
		t.Fatalf("SetKV: %v", err)
	}
	if err := st.SetKV("version:web", "v2.1.0"); err != nil {
		t.Fatalf("SetKV: %v", err)
	}

	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
			{Name: "web", Repo: "github.com/org/web"},
			{Name: "billing", Repo: "github.com/org/billing"}, // not in store
		},
	}
	d := New(cfg, nil, nil, st, time.Hour)

	d.loadKnownVersions()

	if d.known["api"] != "v3.0.0" {
		t.Errorf("known[api] = %q, want %q", d.known["api"], "v3.0.0")
	}
	if d.known["web"] != "v2.1.0" {
		t.Errorf("known[web] = %q, want %q", d.known["web"], "v2.1.0")
	}
	if _, ok := d.known["billing"]; ok {
		t.Error("expected billing NOT to be in known map")
	}
}

// TestLoadKnownVersions_NilStore verifies loadKnownVersions is a no-op when
// store is nil.
func TestLoadKnownVersions_NilStore(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	d := New(cfg, nil, nil, nil, time.Hour)

	// Should not panic.
	d.loadKnownVersions()

	if len(d.known) != 0 {
		t.Errorf("expected empty known map, got %v", d.known)
	}
}

// TestPoll_MultipleServices verifies that poll processes all services
// concurrently.
func TestPoll_MultipleServices(t *testing.T) {
	mockGH := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	mockGL := &mockProvider{
		name:    "gitlab",
		release: &model.Release{Tag: "v5.0.0"},
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
			{Name: "billing", Repo: "gitlab.com/org/billing"},
			{Name: "web", Repo: "github.com/org/web"},
		},
	}
	providers := map[string]provider.Provider{
		"github": mockGH,
		"gitlab": mockGL,
	}
	d := New(cfg, providers, nil, nil, time.Hour)

	d.poll(context.Background())

	if d.known["api"] != "v1.0.0" {
		t.Errorf("known[api] = %q, want v1.0.0", d.known["api"])
	}
	if d.known["billing"] != "v5.0.0" {
		t.Errorf("known[billing] = %q, want v5.0.0", d.known["billing"])
	}
	if d.known["web"] != "v1.0.0" {
		t.Errorf("known[web] = %q, want v1.0.0", d.known["web"])
	}
	// GitHub provider should have been called for api + web = 2 calls.
	if got := mockGH.calls.Load(); got != 2 {
		t.Errorf("expected 2 github provider calls, got %d", got)
	}
	if got := mockGL.calls.Load(); got != 1 {
		t.Errorf("expected 1 gitlab provider calls, got %d", got)
	}
}

// TestPoll_NoServices verifies poll with an empty service list.
func TestPoll_NoServices(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{},
	}
	d := New(cfg, nil, nil, nil, time.Hour)

	// Should not panic.
	d.poll(context.Background())
}

// TestCheckService_StoreVersionChangeTriggersNotification verifies the full
// flow: load known version from store, detect change, notify.
func TestCheckService_StoreVersionChangeTriggersNotification(t *testing.T) {
	st, cleanup := newTestStore(t)
	defer cleanup()

	// Pre-populate store with old version.
	if err := st.SetKV("version:api", "v1.0.0"); err != nil {
		t.Fatalf("SetKV: %v", err)
	}

	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag:     "v2.0.0",
			HTMLURL: "https://github.com/org/api/releases/tag/v2.0.0",
		},
	}
	mn := &mockNotifier{}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, mn, st, time.Hour)

	d.RunOnce(context.Background())

	// Version should be updated.
	if d.known["api"] != "v2.0.0" {
		t.Errorf("known[api] = %q, want v2.0.0", d.known["api"])
	}

	// Notification should have been sent.
	events := mn.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(events))
	}
	if events[0].OldVersion != "v1.0.0" || events[0].NewVersion != "v2.0.0" {
		t.Errorf("unexpected event: %+v", events[0])
	}

	// Store should have the new version.
	val, found, err := st.GetKV("version:api")
	if err != nil {
		t.Fatalf("GetKV: %v", err)
	}
	if !found || val != "v2.0.0" {
		t.Errorf("store version = %q (found=%v), want v2.0.0", val, found)
	}
}

// TestNew_FieldsInitialized verifies that New() properly initializes all fields.
func TestNew_FieldsInitialized(t *testing.T) {
	cfg := &config.Config{}
	mn := &mockNotifier{}
	providers := map[string]provider.Provider{}
	d := New(cfg, providers, mn, nil, 5*time.Minute)

	if d.cfg != cfg {
		t.Error("cfg not set")
	}
	if d.notifier == nil {
		t.Error("notifier not set")
	}
	if d.interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m", d.interval)
	}
	if d.known == nil {
		t.Error("known map not initialized")
	}
	if d.stopCh == nil {
		t.Error("stopCh not initialized")
	}
	if d.stopped == nil {
		t.Error("stopped not initialized")
	}
}

// TestDaemon_StartTickerFires verifies that the ticker fires and polls again
// after the initial poll.
func TestDaemon_StartTickerFires(t *testing.T) {
	mock := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}
	d := New(cfg, providers, nil, nil, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		d.Start(ctx)
		close(done)
	}()

	// Wait long enough for at least one ticker cycle beyond the initial poll.
	time.Sleep(150 * time.Millisecond)
	d.Stop()
	<-done

	// The provider should have been called at least twice (initial + ticker).
	if got := mock.calls.Load(); got < 2 {
		t.Errorf("expected at least 2 provider calls (initial + ticker), got %d", got)
	}
}

// TestCheckService_PartialFailureDoesNotBlockOthers verifies that one service
// failing does not prevent other services from being checked.
func TestCheckService_PartialFailureDoesNotBlockOthers(t *testing.T) {
	mockOK := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v1.0.0"},
	}
	mockFail := &mockProvider{
		name: "gitlab",
		err:  errors.New("network error"),
	}
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
			{Name: "billing", Repo: "gitlab.com/org/billing"},
		},
	}
	providers := map[string]provider.Provider{
		"github": mockOK,
		"gitlab": mockFail,
	}
	d := New(cfg, providers, nil, nil, time.Hour)

	d.poll(context.Background())

	if d.known["api"] != "v1.0.0" {
		t.Errorf("known[api] = %q, want v1.0.0", d.known["api"])
	}
	if _, ok := d.known["billing"]; ok {
		t.Error("expected billing NOT in known map after provider failure")
	}
}
