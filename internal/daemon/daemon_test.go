package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/model"
	"github.com/UnityInFlow/releasewave/internal/provider"
)

type mockProvider struct {
	name    string
	release *model.Release
	err     error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) ListReleases(_ context.Context, _, _ string) ([]model.Release, error) {
	return nil, nil
}
func (m *mockProvider) GetLatestRelease(_ context.Context, _, _ string) (*model.Release, error) {
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
