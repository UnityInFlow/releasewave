package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestHandler_RendersDashboard(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "api", Repo: "github.com/org/api"},
		},
	}

	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag:     "v1.2.3",
			HTMLURL: "https://github.com/org/api/releases/tag/v1.2.3",
		},
	}

	providers := map[string]provider.Provider{"github": mock}
	handler := Handler(cfg, providers)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "ReleaseWave") {
		t.Error("expected page to contain 'ReleaseWave'")
	}
	if !strings.Contains(body, "api") {
		t.Error("expected page to contain service name 'api'")
	}
	if !strings.Contains(body, "v1.2.3") {
		t.Error("expected page to contain version 'v1.2.3'")
	}
}

func TestHandler_ProviderError(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "failing", Repo: "github.com/org/fail"},
		},
	}

	mock := &mockProvider{
		name: "github",
		err:  errors.New("API error"),
	}

	providers := map[string]provider.Provider{"github": mock}
	handler := Handler(cfg, providers)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with errors, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "failing") {
		t.Error("expected page to contain service name 'failing'")
	}
}

func TestHandler_NoServices(t *testing.T) {
	cfg := &config.Config{}
	providers := map[string]provider.Provider{}
	handler := Handler(cfg, providers)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestFetchDashboardData(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "svc-a", Repo: "github.com/org/svc-a"},
			{Name: "svc-b", Repo: "github.com/org/svc-b"},
		},
	}

	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag: "v3.0.0",
		},
	}

	providers := map[string]provider.Provider{"github": mock}
	data := fetchDashboardData(context.Background(), cfg, providers)

	if data.TotalServices != 2 {
		t.Errorf("TotalServices = %d, want 2", data.TotalServices)
	}
	if data.HealthyCount != 2 {
		t.Errorf("HealthyCount = %d, want 2", data.HealthyCount)
	}
	if data.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", data.ErrorCount)
	}
	if len(data.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(data.Services))
	}
	if data.Services[0].LatestTag != "v3.0.0" {
		t.Errorf("LatestTag = %q, want %q", data.Services[0].LatestTag, "v3.0.0")
	}
}
