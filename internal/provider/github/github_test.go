package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fakeGitHubServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/testorg/testrepo/releases":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
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
			w.Write([]byte(`{
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
			w.Write([]byte(`[
				{"name": "v2.0.0", "commit": {"sha": "abc123def456"}},
				{"name": "v1.0.0", "commit": {"sha": "789xyz000111"}}
			]`))

		case "/repos/testorg/empty/releases":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))

		case "/repos/testorg/broken/releases":
			w.WriteHeader(http.StatusInternalServerError)

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
