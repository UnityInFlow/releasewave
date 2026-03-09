package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// GO LEARNING: Test Structure Reuse
//
// Notice this test file has the SAME structure as github_test.go:
//   1. fakeServer helper
//   2. newTestClient helper
//   3. Table-driven tests
//
// This is the Go testing idiom — once you learn the pattern,
// every test file looks familiar. That's the point.

func fakeGitLabServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GO LEARNING: r.URL.RawPath vs r.URL.Path
		//   Go's HTTP server decodes percent-encoding in the path automatically.
		//   r.URL.Path = decoded ("/projects/testorg/testrepo/releases")
		//   r.URL.RawPath = original ("/projects/testorg%2Ftestrepo/releases")
		//   We use RawPath to match the GitLab-style encoded path.
		//   If RawPath is empty, fall back to Path.
		path := r.URL.RawPath
		if path == "" {
			path = r.URL.Path
		}

		switch path {

		case "/projects/testorg%2Ftestrepo/releases",
			"/projects/testorg/testrepo/releases":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
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
			w.Write([]byte(`[
				{"name": "v3.0.0", "commit": {"id": "aabbccdd11223344"}},
				{"name": "v2.0.0", "commit": {"id": "55667788aabbccdd"}}
			]`))

		case "/projects/testorg%2Fempty/releases",
			"/projects/testorg/empty/releases":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))

		case "/projects/testorg%2Fbroken/releases",
			"/projects/testorg/broken/releases":
			w.WriteHeader(http.StatusInternalServerError)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return httptest.NewServer(handler)
}

func newTestClient(serverURL string) *Client {
	return &Client{
		httpClient: http.DefaultClient,
		token:      "fake-token",
		baseURL:    serverURL,
	}
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
		{
			name:      "returns releases for valid project",
			owner:     "testorg",
			repo:      "testrepo",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "returns empty for project with no releases",
			owner:     "testorg",
			repo:      "empty",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:    "returns error for server error",
			owner:   "testorg",
			repo:    "broken",
			wantErr: true,
		},
		{
			name:    "returns error for non-existent project",
			owner:   "testorg",
			repo:    "doesnotexist",
			wantErr: true,
		},
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

	// GO LEARNING: Checking multiple fields in sequence
	// Each check is independent — if one fails, we still see the others.
	// t.Errorf continues the test; t.Fatalf would stop it.
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

// GO LEARNING: projectPath encoding test
func TestProjectPath(t *testing.T) {
	// This tests our URL encoding helper.
	// "myorg/myrepo" should become "myorg%2Fmyrepo" for GitLab API.
	result := projectPath("myorg", "myrepo")
	expected := "myorg%2Fmyrepo"
	if result != expected {
		t.Errorf("projectPath = %q, want %q", result, expected)
	}
}
