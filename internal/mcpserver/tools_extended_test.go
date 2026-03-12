package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"errors"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/model"
	"github.com/UnityInFlow/releasewave/internal/store"
)

// ── parseDeps ──────────────────────────────────────────────────────────

func TestParseDeps_GoMod(t *testing.T) {
	content := `module example.com/app

go 1.21

require (
	golang.org/x/net v0.10.0
	github.com/stretchr/testify v1.8.4
	// indirect dep
	golang.org/x/text v0.9.0 // indirect
)

require github.com/spf13/cobra v1.7.0
`
	deps := parseDeps("go.mod", content)

	expected := map[string]string{
		"golang.org/x/net":            "v0.10.0",
		"github.com/stretchr/testify": "v1.8.4",
		"golang.org/x/text":           "v0.9.0",
		"github.com/spf13/cobra":      "v1.7.0",
	}

	for k, v := range expected {
		if got, ok := deps[k]; !ok {
			t.Errorf("missing dep %q", k)
		} else if got != v {
			t.Errorf("dep %q = %q, want %q", k, got, v)
		}
	}
}

func TestParseDeps_GoMod_Empty(t *testing.T) {
	deps := parseDeps("go.mod", "module example.com/app\n\ngo 1.21\n")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for go.mod without requires, got %d", len(deps))
	}
}

func TestParseDeps_GoMod_CommentsSkipped(t *testing.T) {
	content := `module x

require (
	// this is a comment
	golang.org/x/net v0.10.0
)
`
	deps := parseDeps("go.mod", content)
	if len(deps) != 1 {
		t.Errorf("expected 1 dep, got %d", len(deps))
	}
	if _, ok := deps["// this is a comment"]; ok {
		t.Error("comment was parsed as a dependency")
	}
}

func TestParseDeps_PackageJSON(t *testing.T) {
	content := `{
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "4.17.21"
  },
  "devDependencies": {
    "jest": "^29.0.0"
  }
}`
	deps := parseDeps("package.json", content)

	if deps["express"] != "^4.18.0" {
		t.Errorf("express = %q, want ^4.18.0", deps["express"])
	}
	if deps["lodash"] != "4.17.21" {
		t.Errorf("lodash = %q, want 4.17.21", deps["lodash"])
	}
	if deps["jest"] != "^29.0.0" {
		t.Errorf("jest = %q, want ^29.0.0", deps["jest"])
	}
}

func TestParseDeps_PackageJSON_Empty(t *testing.T) {
	deps := parseDeps("package.json", `{"name":"app"}`)
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestParseDeps_PackageJSON_Invalid(t *testing.T) {
	deps := parseDeps("package.json", "not valid json")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for invalid JSON, got %d", len(deps))
	}
}

func TestParseDeps_RequirementsTxt(t *testing.T) {
	content := `# My requirements
Django==4.2.0
requests>=2.28.0
flask~=2.3.0
boto3!=1.26.0
numpy
`
	deps := parseDeps("requirements.txt", content)

	if deps["Django"] != "==4.2.0" {
		t.Errorf("Django = %q, want ==4.2.0", deps["Django"])
	}
	if deps["requests"] != ">=2.28.0" {
		t.Errorf("requests = %q, want >=2.28.0", deps["requests"])
	}
	if deps["flask"] != "~=2.3.0" {
		t.Errorf("flask = %q, want ~=2.3.0", deps["flask"])
	}
	if deps["boto3"] != "!=1.26.0" {
		t.Errorf("boto3 = %q, want !=1.26.0", deps["boto3"])
	}
	if deps["numpy"] != "*" {
		t.Errorf("numpy = %q, want *", deps["numpy"])
	}
}

func TestParseDeps_RequirementsTxt_CommentsAndBlanks(t *testing.T) {
	content := `# comment

requests==2.28.0

# another comment
`
	deps := parseDeps("requirements.txt", content)
	if len(deps) != 1 {
		t.Errorf("expected 1 dep, got %d: %v", len(deps), deps)
	}
}

func TestParseDeps_UnknownFile(t *testing.T) {
	deps := parseDeps("Cargo.toml", "[dependencies]\nserde = \"1.0\"")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for unknown file type, got %d", len(deps))
	}
}

// ── handleReleaseHistory ───────────────────────────────────────────────

func TestHandleReleaseHistory_NoStore(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)
	// s.store is nil

	req := newCallToolRequest(map[string]any{"service": "api"})
	result, err := s.handleReleaseHistory(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErrorResult(result) {
		t.Fatal("expected error when store is nil")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "no storage configured") {
		t.Errorf("expected 'no storage configured', got: %s", text)
	}
}

func TestHandleReleaseHistory_MissingService(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{})
	result, err := s.handleReleaseHistory(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErrorResult(result) {
		t.Fatal("expected error for missing service param")
	}
}

func TestHandleReleaseHistory_WithStore(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	// Create temp SQLite store.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()
	s.store = st

	// Record a release.
	if err := st.RecordRelease(store.Release{
		Service:      "api",
		Tag:          "v1.0.0",
		Platform:     "github",
		URL:          "https://example.com",
		PublishedAt:  sampleReleases()[2].PublishedAt,
		DiscoveredAt: sampleReleases()[2].PublishedAt,
	}); err != nil {
		t.Fatalf("record release: %v", err)
	}

	req := newCallToolRequest(map[string]any{"service": "api"})
	result, callErr := s.handleReleaseHistory(context.Background(), req)
	if callErr != nil {
		t.Fatalf("unexpected error: %v", callErr)
	}
	if isErrorResult(result) {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}

	text := resultText(t, result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	total := parsed["total"].(float64)
	if total != 1 {
		t.Errorf("expected 1 release, got %v", total)
	}
}

// ── handleReleaseDiff ──────────────────────────────────────────────────

func TestHandleReleaseDiff_MissingService(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{})
	result, err := s.handleReleaseDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErrorResult(result) {
		t.Fatal("expected error for missing service param")
	}
}

func TestHandleReleaseDiff_ServiceNotFound(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServerWithServices(mock, []config.ServiceConfig{
		{Name: "web", Repo: "github.com/org/web"},
	})

	req := newCallToolRequest(map[string]any{"service": "nonexistent"})
	result, err := s.handleReleaseDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErrorResult(result) {
		t.Fatal("expected error for unknown service")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "not found in config") {
		t.Errorf("expected 'not found in config', got: %s", text)
	}
}

func TestHandleReleaseDiff_NoK8s(t *testing.T) {
	latest := sampleReleases()[0]
	mock := &mockProvider{
		name:          "github",
		latestRelease: &latest,
		releases:      sampleReleases(),
	}
	s := testServerWithServices(mock, []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
	})

	// No K8s available — deployed version will be empty.
	req := newCallToolRequest(map[string]any{
		"service":    "api",
		"kubeconfig": filepath.Join(os.TempDir(), "nonexistent-kubeconfig"),
	})
	result, err := s.handleReleaseDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isErrorResult(result) {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}

	text := resultText(t, result)
	if !strings.Contains(text, "v2.0.0") {
		t.Error("expected latest release tag in result")
	}
	if !strings.Contains(text, "deployed version unknown") {
		t.Error("expected 'deployed version unknown' status")
	}
}

// ── handleWatchReleases version change detection ────────────────────────

func TestHandleWatchReleases_DetectsVersionChange(t *testing.T) {
	latest := sampleReleases()[0] // v2.0.0
	mock := &mockProvider{
		name:          "github",
		latestRelease: &latest,
	}

	services := []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
	}
	s := testServerWithServices(mock, services)

	ctx := context.Background()
	req := newCallToolRequest(nil)

	// First call — seeds known versions.
	result1, err := s.handleWatchReleases(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if isErrorResult(result1) {
		t.Fatalf("first call error: %s", resultText(t, result1))
	}

	// Change the latest release to v3.0.0.
	newRelease := model.Release{Tag: "v3.0.0", HTMLURL: "https://example.com"}
	mock.latestRelease = &newRelease

	// Second call — should detect version change.
	result2, err := s.handleWatchReleases(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, result2)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}

	// Without a notifier configured, new_releases stays 0 (no notification sent),
	// but the version tracking still updates.
	if !strings.Contains(text, "v3.0.0") {
		t.Error("expected v3.0.0 in result")
	}
}

func TestHandleWatchReleases_NoServices(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServerWithServices(mock, nil)

	req := newCallToolRequest(nil)
	result, err := s.handleWatchReleases(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No services configured") {
		t.Errorf("expected 'No services configured', got: %s", text)
	}
}

// ── handleDependencyMatrix ─────────────────────────────────────────────

func TestHandleDependencyMatrix_NoServices(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServerWithServices(mock, nil)

	req := newCallToolRequest(nil)
	result, err := s.handleDependencyMatrix(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No services configured") {
		t.Errorf("expected 'No services configured', got: %s", text)
	}
}

func TestHandleDependencyMatrix_NoDepFile(t *testing.T) {
	mock := &mockProvider{
		name:        "github",
		fileContent: nil, // all GetFileContent calls will return "file not found"
	}

	services := []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
	}
	s := testServerWithServices(mock, services)

	req := newCallToolRequest(nil)
	result, err := s.handleDependencyMatrix(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if isErrorResult(result) {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
}

// ── handleServiceGraph ─────────────────────────────────────────────────

func TestHandleServiceGraph_NoServices(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServerWithServices(mock, nil)

	req := newCallToolRequest(nil)
	result, err := s.handleServiceGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No services configured") {
		t.Errorf("expected 'No services configured', got: %s", text)
	}
}

func TestHandleServiceGraph_SharedDeps(t *testing.T) {
	mock := &mockProvider{
		name: "github",
		fileContent: map[string][]byte{
			"go.mod": []byte("module example.com/app\n\ngo 1.21\n\nrequire (\n\tgolang.org/x/net v0.10.0\n\tgithub.com/shared/lib v1.0.0\n)\n"),
		},
	}

	services := []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
		{Name: "web", Repo: "github.com/testorg/web"},
	}
	s := testServerWithServices(mock, services)

	req := newCallToolRequest(nil)
	result, err := s.handleServiceGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if isErrorResult(result) {
		t.Fatalf("expected success, got error: %s", resultText(t, result))
	}
	text := resultText(t, result)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	sharedDeps := parsed["shared_deps"].(float64)
	if sharedDeps < 1 {
		t.Errorf("expected at least 1 shared dep, got %v", sharedDeps)
	}
	connections := parsed["service_connections"].(float64)
	if connections < 1 {
		t.Errorf("expected at least 1 connection, got %v", connections)
	}
}

// ── handleGetRepoFile ──────────────────────────────────────────────────

func TestHandleGetRepoFile_FileNotFound(t *testing.T) {
	mock := &mockProvider{name: "github", fileContent: nil}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{
		"owner": "org", "repo": "repo", "path": "missing.txt", "platform": "github",
	})
	result, err := s.handleGetRepoFile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !isErrorResult(result) {
		t.Fatal("expected error for missing file")
	}
}

func TestHandleGetRepoFile_UnsupportedPlatform(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{
		"owner": "org", "repo": "repo", "path": "file.txt", "platform": "bitbucket",
	})
	result, err := s.handleGetRepoFile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !isErrorResult(result) {
		t.Fatal("expected error for unsupported platform")
	}
}

// ── handleUpgradePlan ──────────────────────────────────────────────────

func TestHandleUpgradePlan_NoServices(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServerWithServices(mock, nil)

	req := newCallToolRequest(map[string]any{})
	result, err := s.handleUpgradePlan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No services configured") {
		t.Errorf("expected 'No services configured', got: %s", text)
	}
}

// ── handleCompareReleaseVsDeployed ─────────────────────────────────────

func TestHandleCompareReleaseVsDeployed_NoServices(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServerWithServices(mock, nil)

	req := newCallToolRequest(map[string]any{})
	result, err := s.handleCompareReleaseVsDeployed(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No services configured") {
		t.Errorf("expected 'No services configured', got: %s", text)
	}
}

// ── Accessors ──────────────────────────────────────────────────────────

func TestServerAccessors(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	if s.Config() == nil {
		t.Error("Config() returned nil")
	}
	if s.Providers() == nil {
		t.Error("Providers() returned nil")
	}
	// MCPServer is nil in test setup (no mcp.NewMCPServer called).
	if s.MCPServer() != nil {
		t.Error("MCPServer() should be nil in test setup")
	}
	// Store is nil by default.
	if s.Store() != nil {
		t.Error("Store() should be nil by default")
	}
}

// ── handleListImageTags / handleCompareImageTags with valid image ──────
// These hit real registries; we only test validation paths above.
// Here we ensure they don't panic with well-formed but unreachable inputs.

func TestHandleListImageTags_NonexistentImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{"image": "ghcr.io/nonexistent-org-12345/nonexistent-repo"})
	result, err := s.handleListImageTags(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// Should return an MCP error result, not a Go error.
	if !isErrorResult(result) {
		// Some registries may return empty results instead of errors.
		t.Log("note: registry returned success for nonexistent image")
	}
}

// ── handleDiscoverServices ─────────────────────────────────────────────
// Requires K8s; test input validation only.

func TestHandleDiscoverServices_NoK8s(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	req := newCallToolRequest(map[string]any{
		"kubeconfig": filepath.Join(os.TempDir(), "nonexistent-kubeconfig"),
	})
	result, err := s.handleDiscoverServices(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !isErrorResult(result) {
		t.Log("note: discover succeeded without K8s (may be using in-cluster config)")
	}
}

// ── Shutdown ───────────────────────────────────────────────────────────

func TestShutdown_NoServers(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	// Should not panic when httpServer, sse, and store are all nil.
	err := s.Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestShutdown_WithStore(t *testing.T) {
	mock := &mockProvider{name: "github"}
	s := testServer(mock)

	dbPath := filepath.Join(t.TempDir(), "shutdown-test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s.store = st

	if err := s.Shutdown(context.Background()); err != nil {
		t.Errorf("shutdown with store failed: %v", err)
	}
}

// ── depFiles var ───────────────────────────────────────────────────────

func TestDepFilesContainsExpected(t *testing.T) {
	expected := []string{"go.mod", "package.json", "requirements.txt"}
	if len(depFiles) != len(expected) {
		t.Fatalf("expected %d dep files, got %d", len(expected), len(depFiles))
	}
	for i, f := range expected {
		if depFiles[i] != f {
			t.Errorf("depFiles[%d] = %q, want %q", i, depFiles[i], f)
		}
	}
}

// ── handleFindOutdated with provider error ─────────────────────────────

func TestHandleFindOutdated_ProviderError(t *testing.T) {
	mock := &mockProvider{
		name:         "github",
		getLatestErr: errors.New("provider error"),
	}
	services := []config.ServiceConfig{
		{Name: "api", Repo: "github.com/testorg/api"},
	}
	s := testServerWithServices(mock, services)

	req := newCallToolRequest(nil)
	result, err := s.handleFindOutdated(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// Individual service error should be in the result, not cause handler error.
	if isErrorResult(result) {
		t.Fatal("expected success result with per-service error")
	}
}
