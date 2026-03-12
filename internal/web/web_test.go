package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func testHandler(t *testing.T, cfg *config.Config, providers map[string]provider.Provider) http.Handler {
	t.Helper()
	handler, err := Handler(cfg, providers)
	if err != nil {
		t.Fatalf("Handler() error: %v", err)
	}
	return handler
}

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

	handler := testHandler(t, cfg, map[string]provider.Provider{"github": mock})

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
	if !strings.Contains(body, "htmx") {
		t.Error("expected page to contain htmx reference")
	}
}

func TestHandler_ProviderError(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "failing", Repo: "github.com/org/fail"},
		},
	}

	mock := &mockProvider{name: "github", err: errors.New("API error")}
	handler := testHandler(t, cfg, map[string]provider.Provider{"github": mock})

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
	handler := testHandler(t, &config.Config{}, map[string]provider.Provider{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No services configured") {
		t.Error("expected empty state message")
	}
}

func TestPartialStats(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "svc", Repo: "github.com/org/svc"},
		},
	}
	mock := &mockProvider{name: "github", release: &model.Release{Tag: "v1.0.0"}}
	handler := testHandler(t, cfg, map[string]provider.Provider{"github": mock})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/partials/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Services") {
		t.Error("expected stats partial to contain 'Services'")
	}
}

func TestPartialServices(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "svc", Repo: "github.com/org/svc"},
		},
	}
	mock := &mockProvider{name: "github", release: &model.Release{Tag: "v2.0.0"}}
	handler := testHandler(t, cfg, map[string]provider.Provider{"github": mock})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/partials/services", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "v2.0.0") {
		t.Error("expected services partial to contain 'v2.0.0'")
	}
}

func TestAddService(t *testing.T) {
	cfg := &config.Config{}
	mock := &mockProvider{name: "github", release: &model.Release{Tag: "v1.0.0"}}
	handler := testHandler(t, cfg, map[string]provider.Provider{"github": mock})

	form := url.Values{}
	form.Set("name", "new-svc")
	form.Set("repo", "github.com/org/new-svc")

	req := httptest.NewRequest(http.MethodPost, "/dashboard/services", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if len(cfg.Services) != 1 || cfg.Services[0].Name != "new-svc" {
		t.Fatalf("expected service to be added, got %v", cfg.Services)
	}
}

func TestAddService_Duplicate(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{{Name: "existing", Repo: "github.com/org/existing"}},
	}
	handler := testHandler(t, cfg, map[string]provider.Provider{})

	form := url.Values{}
	form.Set("name", "existing")
	form.Set("repo", "github.com/org/existing")

	req := httptest.NewRequest(http.MethodPost, "/dashboard/services", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestAddService_BadRepo(t *testing.T) {
	handler := testHandler(t, &config.Config{}, map[string]provider.Provider{})

	form := url.Values{}
	form.Set("name", "bad")
	form.Set("repo", "not-valid")

	req := httptest.NewRequest(http.MethodPost, "/dashboard/services", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAddService_MissingFields(t *testing.T) {
	handler := testHandler(t, &config.Config{}, map[string]provider.Provider{})

	req := httptest.NewRequest(http.MethodPost, "/dashboard/services", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteService(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "svc-a", Repo: "github.com/org/a"},
			{Name: "svc-b", Repo: "github.com/org/b"},
		},
	}
	mock := &mockProvider{name: "github", release: &model.Release{Tag: "v1.0.0"}}
	handler := testHandler(t, cfg, map[string]provider.Provider{"github": mock})

	req := httptest.NewRequest(http.MethodDelete, "/dashboard/services/svc-a", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if len(cfg.Services) != 1 || cfg.Services[0].Name != "svc-b" {
		t.Fatalf("expected svc-a removed, got %v", cfg.Services)
	}
}

func TestDeleteService_NotFound(t *testing.T) {
	handler := testHandler(t, &config.Config{}, map[string]provider.Provider{})

	req := httptest.NewRequest(http.MethodDelete, "/dashboard/services/ghost", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestFetchDashboardData(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "svc-a", Repo: "github.com/org/svc-a"},
			{Name: "svc-b", Repo: "github.com/org/svc-b"},
		},
	}

	mock := &mockProvider{name: "github", release: &model.Release{Tag: "v3.0.0"}}
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
