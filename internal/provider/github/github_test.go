package github

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

func fakeGitHubServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testorg/testrepo/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{
					"tag_name": "v2.0.0",
					"name": "Version 2.0",
					"body": "Major release with breaking changes",
					"draft": false,
					"prerelease": false,
					"published_at": "2025-06-15T10:00:00Z",
					"html_url": "https://github.com/testorg/testrepo/releases/tag/v2.0.0"
				},
				{
					"tag_name": "v1.0.0",
					"name": "Version 1.0",
					"body": "Initial release",
					"draft": false,
					"prerelease": false,
					"published_at": "2025-01-01T10:00:00Z",
					"html_url": "https://github.com/testorg/testrepo/releases/tag/v1.0.0"
				}
			]`))

		case "/repos/testorg/testrepo/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"tag_name": "v2.0.0",
				"name": "Version 2.0",
				"body": "Major release with breaking changes",
				"draft": false,
				"prerelease": false,
				"published_at": "2025-06-15T10:00:00Z",
				"html_url": "https://github.com/testorg/testrepo/releases/tag/v2.0.0"
			}`))

		case "/repos/testorg/testrepo/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"name": "v2.0.0", "commit": {"sha": "abc123def456"}},
				{"name": "v1.0.0", "commit": {"sha": "789xyz000111"}}
			]`))

		case "/repos/testorg/empty/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))

		case "/repos/testorg/empty/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))

		case "/repos/testorg/broken/releases":
			w.WriteHeader(http.StatusInternalServerError)

		case "/repos/testorg/broken/releases/latest":
			w.WriteHeader(http.StatusInternalServerError)

		case "/repos/testorg/broken/tags":
			w.WriteHeader(http.StatusInternalServerError)

		// Invalid JSON responses for decode error paths
		case "/repos/testorg/badjson/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not valid json`))

		case "/repos/testorg/badjson/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not valid json`))

		case "/repos/testorg/badjson/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not valid json`))

		// Rate limit response
		case "/repos/testorg/ratelimited/releases":
			w.WriteHeader(http.StatusTooManyRequests)

		// Auth error response
		case "/repos/testorg/autherror/releases":
			w.WriteHeader(http.StatusForbidden)

		// Invalid published_at time
		case "/repos/testorg/badtime/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{
				"tag_name": "v1.0.0",
				"name": "Bad Time",
				"body": "release with bad time",
				"draft": false,
				"prerelease": true,
				"published_at": "not-a-date",
				"html_url": "https://github.com/testorg/badtime/releases/tag/v1.0.0"
			}]`))

		case "/repos/testorg/badtime/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"tag_name": "v1.0.0",
				"name": "Bad Time",
				"body": "release with bad time",
				"draft": false,
				"prerelease": true,
				"published_at": "not-a-date",
				"html_url": "https://github.com/testorg/badtime/releases/tag/v1.0.0"
			}`))

		// Empty published_at (should not log warning)
		case "/repos/testorg/emptytime/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{
				"tag_name": "v0.1.0",
				"name": "Draft",
				"body": "",
				"draft": true,
				"prerelease": false,
				"published_at": "",
				"html_url": ""
			}]`))

		// File content endpoints
		case "/repos/testorg/testrepo/contents/README.md":
			w.Header().Set("Content-Type", "application/json")
			encoded := base64.StdEncoding.EncodeToString([]byte("# Hello World"))
			_, _ = w.Write([]byte(`{"content":"` + encoded + `","encoding":"base64"}`))

		case "/repos/testorg/testrepo/contents/plain.txt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content":"plain text content","encoding":"none"}`))

		case "/repos/testorg/testrepo/contents/bad-base64.txt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content":"!!!not-valid-base64!!!","encoding":"base64"}`))

		case "/repos/testorg/testrepo/contents/bad-json.txt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not valid json`))

		// Base64 with embedded newlines (GitHub does this)
		case "/repos/testorg/testrepo/contents/multiline.txt":
			w.Header().Set("Content-Type", "application/json")
			raw := base64.StdEncoding.EncodeToString([]byte("line1\nline2\nline3"))
			// Inject newlines to simulate GitHub's chunked base64 (escaped in JSON)
			chunked := raw[:4] + `\n` + raw[4:]
			_, _ = w.Write([]byte(`{"content":"` + chunked + `","encoding":"base64"}`))

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
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	tests := []struct {
		name      string
		owner     string
		repo      string
		wantCount int
		wantErr   bool
	}{
		{"returns releases for valid repo", "testorg", "testrepo", 2, false},
		{"returns empty slice for no releases", "testorg", "empty", 0, false},
		{"returns error for server error", "testorg", "broken", 0, true},
		{"returns error for non-existent repo", "testorg", "doesnotexist", 0, true},
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
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	releases, err := client.ListReleases(context.Background(), "testorg", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first := releases[0]
	if first.Tag != "v2.0.0" {
		t.Errorf("tag = %q, want %q", first.Tag, "v2.0.0")
	}
	if first.Name != "Version 2.0" {
		t.Errorf("name = %q, want %q", first.Name, "Version 2.0")
	}
	if first.Draft {
		t.Error("expected draft=false")
	}
}

func TestGetLatestRelease(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	tests := []struct {
		name    string
		owner   string
		repo    string
		wantTag string
		wantErr bool
	}{
		{"returns latest release", "testorg", "testrepo", "v2.0.0", false},
		{"returns error for non-existent repo", "testorg", "doesnotexist", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release, err := client.GetLatestRelease(context.Background(), tt.owner, tt.repo)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if release == nil {
				t.Fatal("release is nil")
			}
			if release.Tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", release.Tag, tt.wantTag)
			}
		})
	}
}

func TestListTags(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	tags, err := client.ListTags(context.Background(), "testorg", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	if tags[0].Name != "v2.0.0" {
		t.Errorf("first tag = %q, want %q", tags[0].Name, "v2.0.0")
	}
	if tags[0].Commit != "abc123def456" {
		t.Errorf("commit = %q, want %q", tags[0].Commit, "abc123def456")
	}
}

func TestName(t *testing.T) {
	client := New("token")
	if got := client.Name(); got != "github" {
		t.Errorf("Name() = %q, want %q", got, "github")
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
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
		t.Error("expected prerelease=true")
	}
	// PublishedAt should be zero value since the time is invalid
	if !releases[0].PublishedAt.IsZero() {
		t.Errorf("expected zero PublishedAt for invalid time, got %v", releases[0].PublishedAt)
	}
}

func TestListReleases_EmptyPublishedAt(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	releases, err := client.ListReleases(context.Background(), "testorg", "emptytime")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("got %d releases, want 1", len(releases))
	}
	if !releases[0].Draft {
		t.Error("expected draft=true")
	}
	if !releases[0].PublishedAt.IsZero() {
		t.Errorf("expected zero PublishedAt for empty string, got %v", releases[0].PublishedAt)
	}
}

func TestListReleases_RateLimitError(t *testing.T) {
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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

func TestGetLatestRelease_InvalidJSON(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.GetLatestRelease(context.Background(), "testorg", "badjson")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse release JSON") {
		t.Errorf("error = %q, want it to contain 'parse release JSON'", err.Error())
	}
}

func TestGetLatestRelease_ServerError(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	_, err := client.GetLatestRelease(context.Background(), "testorg", "broken")
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
	if !strings.Contains(err.Error(), "get latest release") {
		t.Errorf("error = %q, want it to contain 'get latest release'", err.Error())
	}
}

func TestGetLatestRelease_InvalidTime(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	release, err := client.GetLatestRelease(context.Background(), "testorg", "badtime")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release.Tag != "v1.0.0" {
		t.Errorf("tag = %q, want %q", release.Tag, "v1.0.0")
	}
	if !release.PublishedAt.IsZero() {
		t.Errorf("expected zero PublishedAt for invalid time, got %v", release.PublishedAt)
	}
}

func TestGetFileContent_Base64(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	content, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "README.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "# Hello World" {
		t.Errorf("content = %q, want %q", string(content), "# Hello World")
	}
}

func TestGetFileContent_NonBase64Encoding(t *testing.T) {
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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

func TestGetFileContent_Base64WithNewlines(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	content, err := client.GetFileContent(context.Background(), "testorg", "testrepo", "multiline.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "line1\nline2\nline3" {
		t.Errorf("content = %q, want %q", string(content), "line1\nline2\nline3")
	}
}

func TestDoRequest_CancelledContext(t *testing.T) {
	server := fakeGitHubServer(t)
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
	server := fakeGitHubServer(t)
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
