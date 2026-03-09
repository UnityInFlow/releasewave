// Package github — TESTS
//
// ============================================================================
// GO LEARNING: Testing in Go
// ============================================================================
//
// Key concepts:
//   1. Test files end with _test.go — Go's test runner finds them automatically
//   2. Test functions start with Test... and take *testing.T as argument
//   3. Run tests with: go test ./... (all packages) or go test ./internal/provider/github/
//   4. There's no assert library built in — you use if/t.Errorf or t.Fatal
//   5. Table-driven tests are the Go idiom — define test cases as a slice of structs
//   6. httptest.NewServer creates a fake HTTP server for testing without hitting real APIs
//   7. t.Run("name", func(t *testing.T){...}) creates sub-tests — they show up separately in output
//   8. t.Helper() marks a function as a test helper — errors show the caller's line, not the helper's
//
// Run these tests:
//   go test ./internal/provider/github/ -v        (-v = verbose, shows each test name)
//   go test ./internal/provider/github/ -run TestListReleases  (run only one test)
//   go test ./... -count=1                        (skip cache, always re-run)
//
// ============================================================================

package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ----------------------------------------------------------------------------
// GO LEARNING: httptest.NewServer
// ----------------------------------------------------------------------------
// httptest.NewServer starts a real HTTP server on localhost with a random port.
// We use it to mock the GitHub API — our Client talks to this server instead of github.com.
//
// The pattern:
//   1. Create a handler that returns fake JSON responses
//   2. Start the test server
//   3. Point our Client at it (by replacing baseURL)
//   4. Run the test
//   5. defer server.Close() ensures cleanup even if the test fails
// ----------------------------------------------------------------------------

// fakeGitHubServer creates a test server that mimics the GitHub API.
// It returns predefined JSON for different endpoints.
//
// GO LEARNING: http.HandlerFunc
//   - http.HandlerFunc is a type that turns any function with signature
//     func(http.ResponseWriter, *http.Request) into an http.Handler
//   - This is Go's "adapter pattern" — very common
func fakeGitHubServer(t *testing.T) *httptest.Server {
	// GO LEARNING: t.Helper()
	// Marks this function as a helper. If a test fails inside here,
	// Go will report the error at the caller's line, not inside this function.
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GO LEARNING: switch on r.URL.Path
		// r is the incoming request — we check the path to decide what to return
		switch r.URL.Path {

		case "/repos/testorg/testrepo/releases":
			// Return a JSON array of 2 releases
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
			// Return empty array — tests the "no releases" case
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))

		case "/repos/testorg/broken/releases":
			// Return 500 error — tests error handling
			w.WriteHeader(http.StatusInternalServerError)

		default:
			// Return 404 for unknown paths
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return httptest.NewServer(handler)
}

// newTestClient creates a GitHub Client that talks to our fake server
// instead of the real GitHub API.
func newTestClient(serverURL string) *Client {
	return &Client{
		httpClient: http.DefaultClient,
		token:      "fake-token",
		baseURL:    serverURL, // Point to our fake server
	}
}

// ============================================================================
// GO LEARNING: Table-Driven Tests
// ============================================================================
//
// This is THE standard pattern for Go tests. Instead of writing many separate
// test functions, you define a slice (array) of test cases, each with:
//   - a name (shows up in test output)
//   - input parameters
//   - expected results
//
// Then you loop over them with t.Run(). Benefits:
//   - Easy to add new cases (just add a struct to the slice)
//   - Each case runs as a sub-test (can run individually, shows up in output)
//   - DRY — the actual test logic is written once
//
// ============================================================================

func TestListReleases(t *testing.T) {
	server := fakeGitHubServer(t)
	defer server.Close() // GO LEARNING: defer = runs when function returns, even on panic

	client := newTestClient(server.URL)

	// GO LEARNING: Table-driven test — define cases as a slice of anonymous structs
	tests := []struct {
		name      string // Human-readable test name
		owner     string
		repo      string
		wantCount int  // Expected number of releases
		wantErr   bool // Do we expect an error?
	}{
		{
			name:      "returns releases for valid repo",
			owner:     "testorg",
			repo:      "testrepo",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "returns empty slice for repo with no releases",
			owner:     "testorg",
			repo:      "empty",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "returns error for server error",
			owner:     "testorg",
			repo:      "broken",
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:      "returns error for non-existent repo",
			owner:     "testorg",
			repo:      "doesnotexist",
			wantCount: 0,
			wantErr:   true,
		},
	}

	// GO LEARNING: range iterates over slices. It returns (index, value).
	// We use _ to discard the index since we don't need it.
	for _, tt := range tests {
		// GO LEARNING: t.Run creates a sub-test. Output looks like:
		//   --- PASS: TestListReleases/returns_releases_for_valid_repo
		//   --- PASS: TestListReleases/returns_empty_slice_for_repo_with_no_releases
		t.Run(tt.name, func(t *testing.T) {
			releases, err := client.ListReleases(context.Background(), tt.owner, tt.repo)

			// GO LEARNING: Go has no assert library by default.
			// You check conditions with if and call t.Errorf (continues) or t.Fatal (stops).
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return // No point checking further if we expected an error
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

	// Check first release details
	first := releases[0]
	if first.Tag != "v2.0.0" {
		t.Errorf("tag = %q, want %q", first.Tag, "v2.0.0")
	}
	if first.Name != "Version 2.0" {
		t.Errorf("name = %q, want %q", first.Name, "Version 2.0")
	}
	if first.HTMLURL != "https://github.com/testorg/testrepo/releases/tag/v2.0.0" {
		t.Errorf("url = %q, unexpected", first.HTMLURL)
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
		{
			name:    "returns latest release",
			owner:   "testorg",
			repo:    "testrepo",
			wantTag: "v2.0.0",
			wantErr: false,
		},
		{
			name:    "returns error for non-existent repo",
			owner:   "testorg",
			repo:    "doesnotexist",
			wantTag: "",
			wantErr: true,
		},
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

			// GO LEARNING: Pointer check — GetLatestRelease returns *model.Release
			// We need to make sure it's not nil before accessing fields
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

	// GO LEARNING: We set commit SHA to "abc123def456" in the mock.
	// This tests that JSON unmarshalling works for nested structs.
	if tags[0].Commit != "abc123def456" {
		t.Errorf("first tag commit = %q, want %q", tags[0].Commit, "abc123def456")
	}
}

// GO LEARNING: TestMain (optional)
// If you need setup/teardown for ALL tests in a package, you can define:
//   func TestMain(m *testing.M) {
//       // setup
//       code := m.Run()  // runs all tests
//       // teardown
//       os.Exit(code)
//   }
// We don't need it here, but it's useful for database connections, temp dirs, etc.
