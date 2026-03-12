package gitlab

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rwerrors "github.com/UnityInFlow/releasewave/internal/errors"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

func fakeGitLabServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.RawPath
		if path == "" {
			path = r.URL.Path
		}

		switch path {
		case "/projects/testorg%2Ftestrepo/releases",
			"/projects/testorg/testrepo/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{
					"tag_name": "v3.0.0",
					"name": "Version 3.0",
					"description": "Big update with new features",
					"released_at": "2025-08-01T12:00:00Z",
					"upcoming_release": false,
					"_links": {"self": "https://gitlab.com/api/v4/projects/1/releases/v3.0.0"}
				},
				{
					"tag_name": "v2.0.0",
					"name": "Version 2.0",
					"description": "Previous major release",
					"released_at": "2025-03-15T10:00:00Z",
					"upcoming_release": false,
					"_links": {"self": "https://gitlab.com/api/v4/projects/1/releases/v2.0.0"}
				}
			]`))

		case "/projects/testorg%2Ftestrepo/repository/tags",
			"/projects/testorg/testrepo/repository/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"name": "v3.0.0", "commit": {"id": "aabbccdd11223344"}},
				{"name": "v2.0.0", "commit": {"id": "55667788aabbccdd"}}
			]`))

		case "/projects/testorg%2Fempty/releases",
			"/projects/testorg/empty/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))

		case "/projects/testorg%2Fempty/repository/tags",
			"/projects/testorg/empty/repository/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))

		case "/projects/testorg%2Fbroken/releases",
			"/projects/testorg/broken/releases":
			w.WriteHeader(http.StatusInternalServerError)

		case "/projects/testorg%2Fbroken/repository/tags",
			"/projects/testorg/broken/repository/tags":
			w.WriteHeader(http.StatusInternalServerError)

		// Invalid JSON responses
		case "/projects/testorg%2Fbadjson/releases",
			"/projects/testorg/badjson/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not valid json`))

		case "/projects/testorg%2Fbadjson/repository/tags",
			"/projects/testorg/badjson/repository/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not valid json`))

		// Rate limit response
		case "/projects/testorg%2Fratelimited/releases",
			"/projects/testorg/ratelimited/releases":
			w.WriteHeader(http.StatusTooManyRequests)

		// Auth error response
		case "/projects/testorg%2Fautherror/releases",
			"/projects/testorg/autherror/releases":
			w.WriteHeader(http.StatusForbidden)

		// Invalid released_at time
		case "/projects/testorg%2Fbadtime/releases",
			"/projects/testorg/badtime/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{
				"tag_name": "v1.0.0",
				"name": "Bad Time",
				"description": "release with bad time",
				"released_at": "not-a-date",
				"upcoming_release": true,
				"_links": {"self": "https://gitlab.com/api/v4/projects/1/releases/v1.0.0"}
			}]`))

		// Empty released_at
		case "/projects/testorg%2Femptytime/releases",
			"/projects/testorg/emptytime/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{
				"tag_name": "v0.1.0",
				"name": "Draft",
				"description": "",
				"released_at": "",
				"upcoming_release": false,
				"_links": {"self": ""}
			}]`))

		// File content endpoints - base64 encoded
		case "/projects/testorg%2Ftestrepo/repository/files/README.md",
			"/projects/testorg/testrepo/repository/files/README.md":
			w.Header().Set("Content-Type", "application/json")
			encoded := base64.StdEncoding.EncodeToString([]byte("# Hello GitLab"))
			_, _ = w.Write([]byte(`{"content":"` + encoded + `","encoding":"base64"}`))

		// File content - plain encoding
		case "/projects/testorg%2Ftestrepo/repository/files/plain.txt",
			"/projects/testorg/testrepo/repository/files/plain.txt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content":"plain text content","encoding":"none"}`))

		// File content - invalid base64
		case "/projects/testorg%2Ftestrepo/repository/files/bad-base64.txt",
			"/projects/testorg/testrepo/repository/files/bad-base64.txt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content":"!!!not-valid-base64!!!","encoding":"base64"}`))

		// File content - invalid JSON
		case "/projects/testorg%2Ftestrepo/repository/files/bad-json.txt",
			"/projects/testorg/testrepo/repository/files/bad-json.txt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not valid json`))

		// File with path encoding (nested path)
		case "/projects/testorg%2Ftestrepo/repository/files/src%2Fmain.go",
			"/projects/testorg/testrepo/repository/files/src%2Fmain.go":
			w.Header().Set("Content-Type", "application/json")
			encoded := base64.StdEncoding.EncodeToString([]byte("package main"))
			_, _ = w.Write([]byte(`{"content":"` + encoded + `","encoding":"base64"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return httptest.NewServer(handler)
}

func newTestClient(serverURL string) *Client {
	return New("fake-token", WithBaseURL(serverURL))
}

func TestListReleases(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	tests := []struct {
		name      string
		owner     string
		repo      string
		wantCount int
		wantErr   bool
	}{
		{"returns releases for valid project", "testorg", "testrepo", 2, false},
		{"returns empty for no releases", "testorg", "empty", 0, false},
		{"returns error for server error", "testorg", "broken", 0, true},
		{"returns error for non-existent project", "testorg", "doesnotexist", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			releases, err := client.ListReleases(context.Background(), tt.owner, tt.repo)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(releases) != tt.wantCount {
				t.Errorf("got %d releases, want %d", len(releases), tt.wantCount)
			}
		})
	}
}

func TestListReleases_ContentCheck(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	releases, err := client.ListReleases(context.Background(), "testorg", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first := releases[0]
	if first.Tag != "v3.0.0" {
		t.Errorf("tag = %q, want %q", first.Tag, "v3.0.0")
	}
	if first.Name != "Version 3.0" {
		t.Errorf("name = %q, want %q", first.Name, "Version 3.0")
	}
	if first.Body != "Big update with new features" {
		t.Errorf("body = %q, unexpected", first.Body)
	}
}

func TestGetLatestRelease(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	t.Run("returns latest release", func(t *testing.T) {
		release, err := client.GetLatestRelease(context.Background(), "testorg", "testrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if release.Tag != "v3.0.0" {
			t.Errorf("tag = %q, want %q", release.Tag, "v3.0.0")
		}
	})

	t.Run("returns error for empty project", func(t *testing.T) {
		_, err := client.GetLatestRelease(context.Background(), "testorg", "empty")
		if err == nil {
			t.Error("expected error for empty releases, got nil")
		}
	})
}

func TestListTags(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	tags, err := client.ListTags(context.Background(), "testorg", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	if tags[0].Name != "v3.0.0" {
		t.Errorf("first tag = %q, want %q", tags[0].Name, "v3.0.0")
	}
	if tags[0].Commit != "aabbccdd11223344" {
		t.Errorf("commit = %q, want %q", tags[0].Commit, "aabbccdd11223344")
	}
}

func TestProjectPath(t *testing.T) {
	result := projectPath("myorg", "myrepo")
	expected := "myorg%2Fmyrepo"
	if result != expected {
		t.Errorf("projectPath = %q, want %q", result, expected)
	}
}

func TestName(t *testing.T) {
	client := New("token")
	if got := client.Name(); got != "gitlab" {
		t.Errorf("Name() = %q, want %q", got, "gitlab")
	}
}

func TestWithRateLimiter(t *testing.T) {
	limiter := ratelimit.New(10, 10)
	client := New("token", WithRateLimiter(limiter))
	if client.limiter != limiter {
		t.Error("WithRateLimiter did not set limiter")
	}
}

func TestListTags_HTTPError(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.ListTags(context.Background(), "testorg", "broken")
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
	if !strings.Contains(err.Error(), "list tags") {
		t.Errorf("error = %q, want it to contain 'list tags'", err.Error())
	}
}

func TestListTags_EmptyResponse(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	tags, err := client.ListTags(context.Background(), "testorg", "empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("got %d tags, want 0", len(tags))
	}
}

func TestListTags_InvalidJSON(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.ListTags(context.Background(), "testorg", "badjson")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse tags JSON") {
		t.Errorf("error = %q, want it to contain 'parse tags JSON'", err.Error())
	}
}

func TestListTags_NotFound(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.ListTags(context.Background(), "testorg", "doesnotexist")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	var provErr *rwerrors.ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if provErr.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", provErr.Status, http.StatusNotFound)
	}
}

func TestListReleases_InvalidJSON(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.ListReleases(context.Background(), "testorg", "badjson")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse releases JSON") {
		t.Errorf("error = %q, want it to contain 'parse releases JSON'", err.Error())
	}
}

func TestListReleases_InvalidTime(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	releases, err := client.ListReleases(context.Background(), "testorg", "badtime")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("got %d releases, want 1", len(releases))
	}
	if releases[0].Tag != "v1.0.0" {
		t.Errorf("tag = %q, want %q", releases[0].Tag, "v1.0.0")
	}
	if !releases[0].Prerelease {
		t.Error("expected prerelease=true for upcoming_release")
	}
	if !releases[0].PublishedAt.IsZero() {
		t.Errorf("expected zero PublishedAt for invalid time, got %v", releases[0].PublishedAt)
	}
}

func TestListReleases_EmptyReleasedAt(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	releases, err := client.ListReleases(context.Background(), "testorg", "emptytime")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("got %d releases, want 1", len(releases))
	}
	if !releases[0].PublishedAt.IsZero() {
		t.Errorf("expected zero PublishedAt for empty string, got %v", releases[0].PublishedAt)
	}
}

func TestListReleases_RateLimitError(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.ListReleases(context.Background(), "testorg", "ratelimited")
	if err == nil {
		t.Fatal("expected error for rate limit, got nil")
	}
	var provErr *rwerrors.ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if provErr.Status != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", provErr.Status, http.StatusTooManyRequests)
	}
	if !rwerrors.IsRateLimit(err) {
		t.Error("expected IsRateLimit to return true")
	}
}

func TestListReleases_AuthError(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.ListReleases(context.Background(), "testorg", "autherror")
	if err == nil {
		t.Fatal("expected error for auth failure, got nil")
	}
	var provErr *rwerrors.ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if provErr.Status != http.StatusForbidden {
		t.Errorf("status = %d, want %d", provErr.Status, http.StatusForbidden)
	}
	if !rwerrors.IsAuth(err) {
		t.Error("expected IsAuth to return true")
	}
}

func TestGetLatestRelease_ServerError(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.GetLatestRelease(context.Background(), "testorg", "broken")
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

func TestGetLatestRelease_NotFound(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.GetLatestRelease(context.Background(), "testorg", "doesnotexist")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestGetFileContent_Base64(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	content, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "README.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "# Hello GitLab" {
		t.Errorf("content = %q, want %q", string(content), "# Hello GitLab")
	}
}

func TestGetFileContent_NonBase64Encoding(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	content, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "plain.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "plain text content" {
		t.Errorf("content = %q, want %q", string(content), "plain text content")
	}
}

func TestGetFileContent_InvalidBase64(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "bad-base64.txt")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
	if !strings.Contains(err.Error(), "decode base64 content") {
		t.Errorf("error = %q, want it to contain 'decode base64 content'", err.Error())
	}
}

func TestGetFileContent_InvalidJSON(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "bad-json.txt")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse file response") {
		t.Errorf("error = %q, want it to contain 'parse file response'", err.Error())
	}
}

func TestGetFileContent_NotFound(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "get file content") {
		t.Errorf("error = %q, want it to contain 'get file content'", err.Error())
	}
	if !rwerrors.IsNotFound(err) {
		t.Error("expected IsNotFound to return true")
	}
}

func TestGetFileContent_NestedPath(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	content, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "src/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "package main" {
		t.Errorf("content = %q, want %q", string(content), "package main")
	}
}

func TestDoRequest_CancelledContext(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.ListReleases(ctx, "testorg", "testrepo")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDoRequest_NoToken(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	// Create client with empty token
	client := New("", WithBaseURL(server.URL))

	// Should still work - just no auth header
	releases, err := client.ListReleases(context.Background(), "testorg", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 2 {
		t.Errorf("got %d releases, want 2", len(releases))
	}
}

func TestListReleases_HTMLURLFormat(t *testing.T) {
	server := fakeGitLabServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	releases, err := client.ListReleases(context.Background(), "testorg", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "https://gitlab.com/testorg/testrepo/-/releases/v3.0.0"
	if releases[0].HTMLURL != expected {
		t.Errorf("HTMLURL = %q, want %q", releases[0].HTMLURL, expected)
	}
}
