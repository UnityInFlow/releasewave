package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/model"
	"github.com/UnityInFlow/releasewave/internal/provider"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	name     string
	release  *model.Release
	releases []model.Release
	tags     []model.Tag
	err      error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) ListReleases(_ context.Context, _, _ string) ([]model.Release, error) {
	return m.releases, m.err
}

func (m *mockProvider) GetLatestRelease(_ context.Context, _, _ string) (*model.Release, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.release, nil
}

func (m *mockProvider) ListTags(_ context.Context, _, _ string) ([]model.Tag, error) {
	return m.tags, m.err
}

func (m *mockProvider) GetFileContent(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, m.err
}

// Compile-time check that mockProvider satisfies the Provider interface.
var _ provider.Provider = (*mockProvider)(nil)

// newTestHandler creates an apiHandler wired up for testing.
func newTestHandler(cfg *config.Config, providers map[string]provider.Provider) http.Handler {
	return Handler(cfg, providers, nil)
}

func TestHandlerReturnsValidMux(t *testing.T) {
	cfg := &config.Config{}
	h := Handler(cfg, nil, nil)
	if h == nil {
		t.Fatal("Handler() returned nil")
	}
}

func TestListServices_Empty(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	var total float64
	if err := json.Unmarshal(body["total"], &total); err != nil {
		t.Fatalf("failed to parse total: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected total 0, got %v", total)
	}
}

func TestListServices_WithProvider(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		release: &model.Release{
			Tag: "v1.2.3",
		},
	}

	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "my-svc", Repo: "github.com/org/repo"},
		},
	}
	providers := map[string]provider.Provider{
		"github": mock,
	}

	h := newTestHandler(cfg, providers)
	req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		Total    int `json:"total"`
		Services []struct {
			Name   string `json:"name"`
			Repo   string `json:"repo"`
			Latest string `json:"latest_release"`
			Error  string `json:"error"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body.Total != 1 {
		t.Fatalf("expected total 1, got %d", body.Total)
	}
	if body.Services[0].Name != "my-svc" {
		t.Fatalf("expected service name my-svc, got %q", body.Services[0].Name)
	}
	if body.Services[0].Latest != "v1.2.3" {
		t.Fatalf("expected latest release v1.2.3, got %q", body.Services[0].Latest)
	}
}

func TestListServices_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		err:  fmt.Errorf("rate limited"),
	}

	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "failing", Repo: "github.com/org/repo"},
		},
	}
	providers := map[string]provider.Provider{"github": mock}

	h := newTestHandler(cfg, providers)
	req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with provider error, got %d", rec.Code)
	}

	var body struct {
		Services []struct {
			Error string `json:"error"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Services[0].Error == "" {
		t.Fatal("expected error field to be populated")
	}
}

func TestListServices_UnsupportedPlatform(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "unknown", Repo: "bitbucket.org/org/repo"},
		},
	}
	// No provider registered for "bitbucket.org"
	providers := map[string]provider.Provider{}

	h := newTestHandler(cfg, providers)
	req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Services []struct {
			Error string `json:"error"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Services[0].Error != "unsupported platform" {
		t.Fatalf("expected 'unsupported platform' error, got %q", body.Services[0].Error)
	}
}

func TestAddService_Success(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	payload := `{"name":"new-svc","repo":"github.com/org/repo","registry":"ghcr.io/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "created" {
		t.Fatalf("expected status 'created', got %q", body["status"])
	}
	if body["name"] != "new-svc" {
		t.Fatalf("expected name 'new-svc', got %q", body["name"])
	}

	// Verify the service was added to the config.
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service in config, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "new-svc" {
		t.Fatalf("expected service name 'new-svc', got %q", cfg.Services[0].Name)
	}
	if cfg.Services[0].Registry != "ghcr.io/org/repo" {
		t.Fatalf("expected registry 'ghcr.io/org/repo', got %q", cfg.Services[0].Registry)
	}
}

func TestAddService_MissingName(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	payload := `{"repo":"github.com/org/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "name and repo are required") {
		t.Fatalf("expected 'name and repo are required' error, got %q", body["error"])
	}
}

func TestAddService_MissingRepo(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	payload := `{"name":"svc"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAddService_InvalidRepoFormat(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	payload := `{"name":"svc","repo":"just-a-name"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "host/owner/repo") {
		t.Fatalf("expected repo format error, got %q", body["error"])
	}
}

func TestAddService_RepoTwoParts(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	payload := `{"name":"svc","repo":"github.com/only-owner"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for two-part repo, got %d", rec.Code)
	}
}

func TestAddService_Duplicate(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "existing", Repo: "github.com/org/repo"},
		},
	}
	h := newTestHandler(cfg, nil)

	payload := `{"name":"existing","repo":"github.com/other/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "already exists") {
		t.Fatalf("expected 'already exists' error, got %q", body["error"])
	}
}

func TestAddService_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader("{invalid"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "invalid JSON") {
		t.Fatalf("expected 'invalid JSON' error, got %q", body["error"])
	}
}

func TestDeleteService_Success(t *testing.T) {
	cfg := &config.Config{
		Services: []config.ServiceConfig{
			{Name: "to-delete", Repo: "github.com/org/repo"},
			{Name: "keep-me", Repo: "github.com/org/other"},
		},
	}
	h := newTestHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/services/to-delete", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "deleted" {
		t.Fatalf("expected status 'deleted', got %q", body["status"])
	}

	// Verify the service was removed.
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service remaining, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "keep-me" {
		t.Fatalf("expected remaining service 'keep-me', got %q", cfg.Services[0].Name)
	}
}

func TestDeleteService_NotFound(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/services/nonexistent", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "not found") {
		t.Fatalf("expected 'not found' error, got %q", body["error"])
	}
}

func TestGetServiceReleases_StoreNil(t *testing.T) {
	cfg := &config.Config{}
	// store is nil via newTestHandler
	h := newTestHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/services/my-svc/releases", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "no storage configured") {
		t.Fatalf("expected 'no storage configured' error, got %q", body["error"])
	}
}

func TestGetTimeline_StoreNil(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/timeline", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "no storage configured") {
		t.Fatalf("expected 'no storage configured' error, got %q", body["error"])
	}
}

func TestAddService_BodyTooLarge(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	// Create a body larger than 1 MB (the MaxBytesReader limit).
	bigBody := bytes.Repeat([]byte("x"), (1<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/services", bytes.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !strings.Contains(body["error"], "invalid JSON") {
		t.Fatalf("expected 'invalid JSON' error for oversized body, got %q", body["error"])
	}
}

func TestConcurrentAddAndDelete(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	const numWorkers = 50
	var wg sync.WaitGroup

	// Concurrently add services.
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := fmt.Sprintf(`{"name":"svc-%d","repo":"github.com/org/repo-%d"}`, idx, idx)
			req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusCreated {
				t.Errorf("add svc-%d: expected 201, got %d", idx, rec.Code)
			}
		}(i)
	}
	wg.Wait()

	if len(cfg.Services) != numWorkers {
		t.Fatalf("expected %d services after concurrent adds, got %d", numWorkers, len(cfg.Services))
	}

	// Concurrently delete all services.
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			url := fmt.Sprintf("/v1/services/svc-%d", idx)
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("delete svc-%d: expected 200, got %d", idx, rec.Code)
			}
		}(i)
	}
	wg.Wait()

	if len(cfg.Services) != 0 {
		t.Fatalf("expected 0 services after concurrent deletes, got %d", len(cfg.Services))
	}
}

func TestConcurrentAddAndList(t *testing.T) {
	mock := &mockProvider{
		name:    "github",
		release: &model.Release{Tag: "v0.1.0"},
	}
	cfg := &config.Config{}
	providers := map[string]provider.Provider{"github": mock}
	h := Handler(cfg, providers, nil)

	var wg sync.WaitGroup

	// Add services and list concurrently.
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			payload := fmt.Sprintf(`{"name":"csvc-%d","repo":"github.com/org/repo-%d"}`, idx, idx)
			req := httptest.NewRequest(http.MethodPost, "/v1/services", strings.NewReader(payload))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
		}(i)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("list during concurrent adds: expected 200, got %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}

func TestMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	// PUT is not registered on /v1/services.
	req := httptest.NewRequest(http.MethodPut, "/v1/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Go 1.22+ mux returns 405 for unregistered methods on a known path.
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for PUT /v1/services, got %d", rec.Code)
	}
}

func TestUnknownRoute(t *testing.T) {
	cfg := &config.Config{}
	h := newTestHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route, got %d", rec.Code)
	}
}
