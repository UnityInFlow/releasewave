package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/model"
	"github.com/UnityInFlow/releasewave/internal/provider"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	name            string
	releases        []model.Release
	latestRelease   *model.Release
	tags            []model.Tag
	listReleasesErr error
	getLatestErr    error
	listTagsErr     error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) ListReleases(_ context.Context, _, _ string) ([]model.Release, error) {
	if m.listReleasesErr != nil {
		return nil, m.listReleasesErr
	}
	return m.releases, nil
}

func (m *mockProvider) GetLatestRelease(_ context.Context, _, _ string) (*model.Release, error) {
	if m.getLatestErr != nil {
		return nil, m.getLatestErr
	}
	return m.latestRelease, nil
}

func (m *mockProvider) ListTags(_ context.Context, _, _ string) ([]model.Tag, error) {
	if m.listTagsErr != nil {
		return nil, m.listTagsErr
	}
	return m.tags, nil
}

func (m *mockProvider) GetFileContent(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, errors.New("not implemented in mock")
}

// Compile-time check that mockProvider implements provider.Provider.
var _ provider.Provider = (*mockProvider)(nil)

func newCallToolRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func testServer(mock *mockProvider) *Server {
	cfg := &config.Config{
		Cache:     config.CacheConfig{TTL: "1m"},
		Server:    config.ServerConfig{Port: 7891},
		RateLimit: config.RateLimitConfig{GitHub: 100, GitLab: 100},
	}

	providers := map[string]provider.Provider{
		"github": mock,
	}

	return &Server{
		config:    cfg,
		providers: providers,
	}
}

func testServerWithServices(mock *mockProvider, services []config.ServiceConfig) *Server {
	s := testServer(mock)
	s.config.Services = services
	return s
}

// sampleReleases returns test releases (newest first, matching convention).
func sampleReleases() []model.Release {
	return []model.Release{
		{
			Tag:         "v2.0.0",
			Name:        "Version 2.0",
			Body:        "Major release with breaking changes",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
			HTMLURL:     "https://github.com/testorg/testrepo/releases/tag/v2.0.0",
		},
		{
			Tag:         "v1.1.0",
			Name:        "Version 1.1",
			Body:        "Minor release",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC),
			HTMLURL:     "https://github.com/testorg/testrepo/releases/tag/v1.1.0",
		},
		{
			Tag:         "v1.0.0",
			Name:        "Version 1.0",
			Body:        "Initial release",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			HTMLURL:     "https://github.com/testorg/testrepo/releases/tag/v1.0.0",
		},
	}
}

func sampleTags() []model.Tag {
	return []model.Tag{
		{Name: "v2.0.0", Commit: "abc123def456"},
		{Name: "v1.1.0", Commit: "789xyz000111"},
		{Name: "v1.0.0", Commit: "aabbccdd1122"},
	}
}

// isErrorResult checks whether the MCP result is an error result.
func isErrorResult(result *mcp.CallToolResult) bool {
	return result.IsError
}

// resultText extracts the text content from a successful CallToolResult.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// ── handleListReleases ─────────────────────────────────────────────────

func TestHandleListReleases(t *testing.T) {
	mock := &mockProvider{
		name:     "github",
		releases: sampleReleases(),
	}
	s := testServer(mock)

	tests := []struct {
		name      string
		args      map[string]any
		wantErr   bool
		wantCount int
	}{
		{
			name:      "returns releases for valid repo",
			args:      map[string]any{"owner": "testorg", "repo": "testrepo", "platform": "github"},
			wantCount: 3,
		},
		{
			name:    "returns error when owner is missing",
			args:    map[string]any{"repo": "testrepo", "platform": "github"},
			wantErr: true,
		},
		{
			name:    "returns error when repo is missing",
			args:    map[string]any{"owner": "testorg", "platform": "github"},
			wantErr: true,
		},
		{
			name:    "returns error for unsupported platform",
			args:    map[string]any{"owner": "testorg", "repo": "testrepo", "platform": "bitbucket"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newCallToolRequest(tt.args)
			result, err := s.handleListReleases(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Error("expected error result, got success")
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("expected success, got error: %s", resultText(t, result))
			}

			text := resultText(t, result)
			var releases []model.Release
			if err := json.Unmarshal([]byte(text), &releases); err != nil {
				t.Fatalf("failed to parse result JSON: %v", err)
			}
			if len(releases) != tt.wantCount {
				t.Errorf("got %d releases, want %d", len(releases), tt.wantCount)
			}
		})
	}
}

func TestHandleListReleases_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name:            "github",
		listReleasesErr: errors.New("API rate limit exceeded"),
	}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{"owner": "testorg", "repo": "testrepo", "platform": "github"})
	result, err := s.handleListReleases(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !isErrorResult(result) {
		t.Error("expected error result for provider error")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "rate limit") {
		t.Errorf("error message %q should mention rate limit", text)
	}
}

func TestHandleListReleases_DefaultPlatform(t *testing.T) {
	mock := &mockProvider{
		name:     "github",
		releases: sampleReleases(),
	}
	s := testServer(mock)

	// No platform specified - should default to github
	req := newCallToolRequest(map[string]any{"owner": "testorg", "repo": "testrepo"})
	result, err := s.handleListReleases(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if isErrorResult(result) {
		t.Fatalf("expected success with default platform, got error: %s", resultText(t, result))
	}
}

// ── handleGetLatestRelease ─────────────────────────────────────────────

func TestHandleGetLatestRelease(t *testing.T) {
	latest := sampleReleases()[0]
	mock := &mockProvider{
		name:          "github",
		latestRelease: &latest,
	}
	s := testServer(mock)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		wantTag string
	}{
		{
			name:    "returns latest release",
			args:    map[string]any{"owner": "testorg", "repo": "testrepo", "platform": "github"},
			wantTag: "v2.0.0",
		},
		{
			name:    "returns error when owner is missing",
			args:    map[string]any{"repo": "testrepo"},
			wantErr: true,
		},
		{
			name:    "returns error when repo is missing",
			args:    map[string]any{"owner": "testorg"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newCallToolRequest(tt.args)
			result, err := s.handleGetLatestRelease(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Error("expected error result, got success")
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("expected success, got error: %s", resultText(t, result))
			}

			text := resultText(t, result)
			var release model.Release
			if err := json.Unmarshal([]byte(text), &release); err != nil {
				t.Fatalf("failed to parse result JSON: %v", err)
			}
			if release.Tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", release.Tag, tt.wantTag)
			}
		})
	}
}

func TestHandleGetLatestRelease_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name:         "github",
		getLatestErr: errors.New("not found"),
	}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{"owner": "testorg", "repo": "testrepo", "platform": "github"})
	result, err := s.handleGetLatestRelease(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !isErrorResult(result) {
		t.Error("expected error result for provider error")
	}
}

// ── handleListTags ─────────────────────────────────────────────────────

func TestHandleListTags(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		tags: sampleTags(),
	}
	s := testServer(mock)

	tests := []struct {
		name      string
		args      map[string]any
		wantErr   bool
		wantCount int
	}{
		{
			name:      "returns tags for valid repo",
			args:      map[string]any{"owner": "testorg", "repo": "testrepo", "platform": "github"},
			wantCount: 3,
		},
		{
			name:    "returns error when owner is missing",
			args:    map[string]any{"repo": "testrepo"},
			wantErr: true,
		},
		{
			name:    "returns error when repo is missing",
			args:    map[string]any{"owner": "testorg"},
			wantErr: true,
		},
		{
			name:    "returns error for unsupported platform",
			args:    map[string]any{"owner": "o", "repo": "r", "platform": "bitbucket"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newCallToolRequest(tt.args)
			result, err := s.handleListTags(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Error("expected error result, got success")
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("expected success, got error: %s", resultText(t, result))
			}

			text := resultText(t, result)
			var tags []model.Tag
			if err := json.Unmarshal([]byte(text), &tags); err != nil {
				t.Fatalf("failed to parse result JSON: %v", err)
			}
			if len(tags) != tt.wantCount {
				t.Errorf("got %d tags, want %d", len(tags), tt.wantCount)
			}
		})
	}
}

func TestHandleListTags_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name:        "github",
		listTagsErr: errors.New("connection timeout"),
	}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{"owner": "testorg", "repo": "testrepo", "platform": "github"})
	result, err := s.handleListTags(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !isErrorResult(result) {
		t.Error("expected error result for provider error")
	}
}

// ── handleCheckServices ────────────────────────────────────────────────

func TestHandleCheckServices(t *testing.T) {
	latest := sampleReleases()[0]
	mock := &mockProvider{
		name:          "github",
		latestRelease: &latest,
	}

	t.Run("returns results for configured services", func(t *testing.T) {
		services := []config.ServiceConfig{
			{Name: "api", Repo: "github.com/testorg/api"},
			{Name: "web", Repo: "github.com/testorg/web"},
		}
		s := testServerWithServices(mock, services)

		req := newCallToolRequest(nil)
		result, err := s.handleCheckServices(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if isErrorResult(result) {
			t.Fatalf("expected success, got error: %s", resultText(t, result))
		}

		text := resultText(t, result)
		var results []json.RawMessage
		if err := json.Unmarshal([]byte(text), &results); err != nil {
			t.Fatalf("failed to parse result JSON: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d results, want 2", len(results))
		}

		// Verify first result contains expected fields
		if !strings.Contains(text, "v2.0.0") {
			t.Error("expected result to contain latest release tag v2.0.0")
		}
		if !strings.Contains(text, "api") {
			t.Error("expected result to contain service name 'api'")
		}
	})

	t.Run("returns message when no services configured", func(t *testing.T) {
		s := testServerWithServices(mock, nil)

		req := newCallToolRequest(nil)
		result, err := s.handleCheckServices(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if isErrorResult(result) {
			t.Fatalf("unexpected error result: %s", resultText(t, result))
		}

		text := resultText(t, result)
		if !strings.Contains(text, "No services configured") {
			t.Errorf("expected 'No services configured' message, got: %s", text)
		}
	})
}

func TestHandleCheckServices_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name:         "github",
		getLatestErr: errors.New("forbidden"),
	}

	services := []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
	}
	s := testServerWithServices(mock, services)

	req := newCallToolRequest(nil)
	result, err := s.handleCheckServices(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// Should still succeed at the handler level, but individual service has error
	if isErrorResult(result) {
		t.Fatalf("expected success result (with per-service error), got error result")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "forbidden") {
		t.Errorf("expected error details in result, got: %s", text)
	}
}

// ── handleFindOutdated ─────────────────────────────────────────────────

func TestHandleFindOutdated(t *testing.T) {
	latest := sampleReleases()[0]
	mock := &mockProvider{
		name:          "github",
		latestRelease: &latest,
	}

	t.Run("returns results for configured services", func(t *testing.T) {
		services := []config.ServiceConfig{
			{Name: "api", Repo: "github.com/testorg/api"},
		}
		s := testServerWithServices(mock, services)

		req := newCallToolRequest(nil)
		result, err := s.handleFindOutdated(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if isErrorResult(result) {
			t.Fatalf("expected success, got error: %s", resultText(t, result))
		}

		text := resultText(t, result)
		if !strings.Contains(text, "v2.0.0") {
			t.Error("expected result to contain latest release tag")
		}
	})

	t.Run("returns message when no services configured", func(t *testing.T) {
		s := testServerWithServices(mock, nil)

		req := newCallToolRequest(nil)
		result, err := s.handleFindOutdated(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}

		text := resultText(t, result)
		if !strings.Contains(text, "No services configured") {
			t.Errorf("expected 'No services configured' message, got: %s", text)
		}
	})
}

// ── handleChangelogBetweenVersions ─────────────────────────────────────

func TestHandleChangelogBetweenVersions(t *testing.T) {
	mock := &mockProvider{
		name:     "github",
		releases: sampleReleases(),
	}
	s := testServer(mock)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "returns changelog between versions",
			args: map[string]any{
				"owner":    "testorg",
				"repo":     "testrepo",
				"from":     "v1.0.0",
				"to":       "v2.0.0",
				"platform": "github",
			},
			check: func(t *testing.T, text string) {
				if !strings.Contains(text, "v2.0.0") {
					t.Error("expected changelog to contain v2.0.0")
				}
				if !strings.Contains(text, "v1.0.0") {
					t.Error("expected changelog to contain v1.0.0")
				}
				// Should contain the changelog entries
				var result map[string]any
				if err := json.Unmarshal([]byte(text), &result); err != nil {
					t.Fatalf("failed to parse: %v", err)
				}
				releases, ok := result["releases"].(float64)
				if !ok {
					t.Fatal("expected 'releases' field")
				}
				if releases < 2 {
					t.Errorf("expected at least 2 releases in changelog, got %v", releases)
				}
			},
		},
		{
			name: "returns empty when version range not found",
			args: map[string]any{
				"owner":    "testorg",
				"repo":     "testrepo",
				"from":     "v0.1.0",
				"to":       "v0.2.0",
				"platform": "github",
			},
			check: func(t *testing.T, text string) {
				if !strings.Contains(text, "No releases found") {
					t.Errorf("expected 'No releases found' message, got: %s", text)
				}
			},
		},
		{
			name:    "returns error when owner is missing",
			args:    map[string]any{"repo": "testrepo", "from": "v1.0.0", "to": "v2.0.0"},
			wantErr: true,
		},
		{
			name:    "returns error when from is missing",
			args:    map[string]any{"owner": "testorg", "repo": "testrepo", "to": "v2.0.0"},
			wantErr: true,
		},
		{
			name:    "returns error when to is missing",
			args:    map[string]any{"owner": "testorg", "repo": "testrepo", "from": "v1.0.0"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newCallToolRequest(tt.args)
			result, err := s.handleChangelogBetweenVersions(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}

			if tt.wantErr {
				if !isErrorResult(result) {
					t.Error("expected error result, got success")
				}
				return
			}

			if isErrorResult(result) {
				t.Fatalf("expected success, got error: %s", resultText(t, result))
			}

			if tt.check != nil {
				tt.check(t, resultText(t, result))
			}
		})
	}
}

func TestHandleChangelogBetweenVersions_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name:            "github",
		listReleasesErr: errors.New("network error"),
	}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{
		"owner": "testorg", "repo": "testrepo",
		"from": "v1.0.0", "to": "v2.0.0", "platform": "github",
	})
	result, err := s.handleChangelogBetweenVersions(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !isErrorResult(result) {
		t.Error("expected error result for provider error")
	}
}

// ── handleSecurityAdvisories ───────────────────────────────────────────
// Note: This handler creates its own security.Client internally and calls
// the real OSV API. We test input validation and error structure here.

func TestHandleSecurityAdvisories_MissingParams(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing ecosystem", map[string]any{"package": "express", "version": "4.17.1"}},
		{"missing package", map[string]any{"ecosystem": "npm", "version": "4.17.1"}},
		{"missing version", map[string]any{"ecosystem": "npm", "package": "express"}},
		{"all empty", map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newCallToolRequest(tt.args)
			result, err := s.handleSecurityAdvisories(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !isErrorResult(result) {
				t.Error("expected error result for missing params")
			}
			text := resultText(t, result)
			if !strings.Contains(text, "required") {
				t.Errorf("expected error about required params, got: %s", text)
			}
		})
	}
}

// ── handleReleaseTimeline ──────────────────────────────────────────────

func TestHandleReleaseTimeline(t *testing.T) {
	mock := &mockProvider{
		name:     "github",
		releases: sampleReleases(),
	}

	t.Run("returns timeline for configured services", func(t *testing.T) {
		services := []config.ServiceConfig{
			{Name: "api", Repo: "github.com/testorg/api"},
		}
		s := testServerWithServices(mock, services)

		// Use a large number of days to include all sample releases
		req := newCallToolRequest(map[string]any{"days": float64(365 * 5)})
		result, err := s.handleReleaseTimeline(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if isErrorResult(result) {
			t.Fatalf("expected success, got error: %s", resultText(t, result))
		}

		text := resultText(t, result)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			t.Fatalf("failed to parse result: %v", err)
		}

		// Verify structure
		if _, ok := parsed["period"]; !ok {
			t.Error("expected 'period' field in result")
		}
		if _, ok := parsed["total"]; !ok {
			t.Error("expected 'total' field in result")
		}
		if _, ok := parsed["timeline"]; !ok {
			t.Error("expected 'timeline' field in result")
		}
	})

	t.Run("returns message when no services configured", func(t *testing.T) {
		s := testServerWithServices(mock, nil)

		req := newCallToolRequest(map[string]any{"days": float64(30)})
		result, err := s.handleReleaseTimeline(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		text := resultText(t, result)
		if !strings.Contains(text, "No services configured") {
			t.Errorf("expected 'No services configured', got: %s", text)
		}
	})

	t.Run("filters by time window", func(t *testing.T) {
		services := []config.ServiceConfig{
			{Name: "api", Repo: "github.com/testorg/api"},
		}
		s := testServerWithServices(mock, services)

		// Use 0 days - should filter out all historical releases
		req := newCallToolRequest(map[string]any{"days": float64(0)})
		result, err := s.handleReleaseTimeline(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if isErrorResult(result) {
			t.Fatalf("expected success, got error: %s", resultText(t, result))
		}

		text := resultText(t, result)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			t.Fatalf("failed to parse result: %v", err)
		}
		total := parsed["total"].(float64)
		if total != 0 {
			t.Errorf("expected 0 releases in 0-day window, got %v", total)
		}
	})
}

func TestHandleReleaseTimeline_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name:            "github",
		listReleasesErr: errors.New("timeout"),
	}

	services := []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
	}
	s := testServerWithServices(mock, services)

	req := newCallToolRequest(map[string]any{"days": float64(30)})
	result, err := s.handleReleaseTimeline(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// handleReleaseTimeline silently ignores per-service errors, returns empty timeline
	if isErrorResult(result) {
		t.Fatalf("expected success (with empty timeline), got error result")
	}
	text := resultText(t, result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	total := parsed["total"].(float64)
	if total != 0 {
		t.Errorf("expected 0 entries when provider errors, got %v", total)
	}
}

// ── getProvider ────────────────────────────────────────────────────────

func TestGetProvider(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{"returns github provider", "github", false},
		{"defaults to github for empty", "", false},
		{"returns error for unknown platform", "bitbucket", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := s.getProvider(tt.platform)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p == nil {
				t.Fatal("expected non-nil provider")
			}
		})
	}
}

// ── handleListImageTags / handleCompareImageTags ───────────────────────
// These handlers create registry.New() internally, so we test input validation.

func TestHandleListImageTags_MissingImage(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{})
	result, err := s.handleListImageTags(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !isErrorResult(result) {
		t.Error("expected error result for missing image param")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "image is required") {
		t.Errorf("expected 'image is required' error, got: %s", text)
	}
}

func TestHandleCompareImageTags_MissingParams(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing image", map[string]any{"tag1": "v1", "tag2": "v2"}},
		{"missing tag1", map[string]any{"image": "ghcr.io/org/app", "tag2": "v2"}},
		{"missing tag2", map[string]any{"image": "ghcr.io/org/app", "tag1": "v1"}},
		{"all empty", map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newCallToolRequest(tt.args)
			result, err := s.handleCompareImageTags(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !isErrorResult(result) {
				t.Error("expected error result for missing params")
			}
		})
	}
}

// ── Info ───────────────────────────────────────────────────────────────

func TestServerInfo(t *testing.T) {
	mock := &mockProvider{name: "github"}
	services := []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
		{Name: "web", Repo: "github.com/testorg/web"},
	}
	s := testServerWithServices(mock, services)

	info := s.Info()
	if !strings.Contains(info, "ReleaseWave MCP Server") {
		t.Error("expected Info to contain server name")
	}
	if !strings.Contains(info, "2") {
		t.Error("expected Info to contain service count")
	}
	if !strings.Contains(info, "list_releases") {
		t.Error("expected Info to list tools")
	}
}
