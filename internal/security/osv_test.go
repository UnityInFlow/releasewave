package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fakeOSVServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// Read request body to determine response
		// For simplicity, we distinguish test cases by using a custom header
		testCase := r.Header.Get("X-Test-Case")
		if testCase == "" {
			// Default: return vulnerabilities based on content-type presence
			// In practice we parse the body. For tests, we use query params or
			// different server instances. Let's use a single server with
			// deterministic behavior by parsing the request body.
		}

		// We'll use the server for all test cases; the test will set baseURL.
		// Return a response with vulnerabilities.
		_, _ = w.Write([]byte(`{
			"vulns": [
				{
					"id": "GHSA-xxxx-yyyy-zzzz",
					"summary": "Critical vulnerability in example package",
					"aliases": ["CVE-2024-1234"],
					"severity": [
						{"type": "CVSS_V3", "score": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}
					],
					"affected": [
						{
							"ranges": [
								{
									"type": "SEMVER",
									"events": [
										{"introduced": "0"},
										{"fixed": "1.2.3"}
									]
								}
							]
						}
					],
					"references": [
						{"type": "ADVISORY", "url": "https://github.com/advisories/GHSA-xxxx-yyyy-zzzz"},
						{"type": "WEB", "url": "https://example.com/vuln"}
					]
				},
				{
					"id": "GO-2024-5678",
					"summary": "Minor issue in example package",
					"aliases": [],
					"severity": [],
					"affected": [
						{
							"ranges": [
								{
									"type": "SEMVER",
									"events": [
										{"introduced": "1.0.0"}
									]
								}
							]
						}
					],
					"references": [
						{"type": "PACKAGE", "url": "https://pkg.go.dev/vuln/GO-2024-5678"}
					]
				}
			]
		}`))
	})

	return httptest.NewServer(handler)
}

func fakeOSVServerNoVulns(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vulns": []}`))
	}))
}

func fakeOSVServerError(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

func newTestClient(serverURL string) *Client {
	c := New()
	c.baseURL = serverURL
	return c
}

func TestQueryByPackage(t *testing.T) {
	tests := []struct {
		name      string
		setupSrv  func(t *testing.T) *httptest.Server
		ecosystem string
		pkg       string
		version   string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "returns vulnerabilities for affected package",
			setupSrv:  fakeOSVServer,
			ecosystem: "Go",
			pkg:       "golang.org/x/net",
			version:   "0.1.0",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "returns empty for no vulnerabilities",
			setupSrv:  fakeOSVServerNoVulns,
			ecosystem: "Go",
			pkg:       "golang.org/x/text",
			version:   "0.14.0",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "returns error for server error",
			setupSrv:  fakeOSVServerError,
			ecosystem: "Go",
			pkg:       "example.com/broken",
			version:   "1.0.0",
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupSrv(t)
			defer server.Close()

			client := newTestClient(server.URL)
			vulns, err := client.QueryByPackage(context.Background(), tt.ecosystem, tt.pkg, tt.version)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(vulns) != tt.wantCount {
				t.Errorf("got %d vulnerabilities, want %d", len(vulns), tt.wantCount)
			}
		})
	}
}

func TestQueryByPackage_VulnerabilityParsing(t *testing.T) {
	server := fakeOSVServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	vulns, err := client.QueryByPackage(context.Background(), "Go", "golang.org/x/net", "0.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vulns) < 2 {
		t.Fatalf("expected at least 2 vulnerabilities, got %d", len(vulns))
	}

	// Test first vulnerability (with full details)
	first := vulns[0]

	t.Run("parses ID", func(t *testing.T) {
		if first.ID != "GHSA-xxxx-yyyy-zzzz" {
			t.Errorf("ID = %q, want %q", first.ID, "GHSA-xxxx-yyyy-zzzz")
		}
	})

	t.Run("parses summary", func(t *testing.T) {
		if first.Summary != "Critical vulnerability in example package" {
			t.Errorf("Summary = %q, want %q", first.Summary, "Critical vulnerability in example package")
		}
	})

	t.Run("parses severity", func(t *testing.T) {
		want := "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
		if first.Severity != want {
			t.Errorf("Severity = %q, want %q", first.Severity, want)
		}
	})

	t.Run("parses aliases", func(t *testing.T) {
		if len(first.Aliases) != 1 {
			t.Fatalf("got %d aliases, want 1", len(first.Aliases))
		}
		if first.Aliases[0] != "CVE-2024-1234" {
			t.Errorf("alias = %q, want %q", first.Aliases[0], "CVE-2024-1234")
		}
	})

	t.Run("parses fixed version", func(t *testing.T) {
		if first.Fixed != "1.2.3" {
			t.Errorf("Fixed = %q, want %q", first.Fixed, "1.2.3")
		}
	})

	t.Run("parses advisory URL", func(t *testing.T) {
		want := "https://github.com/advisories/GHSA-xxxx-yyyy-zzzz"
		if first.URL != want {
			t.Errorf("URL = %q, want %q", first.URL, want)
		}
	})

	// Test second vulnerability (with missing fields)
	second := vulns[1]

	t.Run("handles empty severity", func(t *testing.T) {
		if second.Severity != "" {
			t.Errorf("Severity = %q, want empty", second.Severity)
		}
	})

	t.Run("handles empty aliases", func(t *testing.T) {
		if len(second.Aliases) != 0 {
			t.Errorf("got %d aliases, want 0", len(second.Aliases))
		}
	})

	t.Run("handles missing fixed version", func(t *testing.T) {
		if second.Fixed != "" {
			t.Errorf("Fixed = %q, want empty", second.Fixed)
		}
	})

	t.Run("falls back to osv.dev URL when no advisory ref", func(t *testing.T) {
		want := "https://osv.dev/vulnerability/GO-2024-5678"
		if second.URL != want {
			t.Errorf("URL = %q, want %q", second.URL, want)
		}
	})
}

func TestQueryByGitCommit(t *testing.T) {
	tests := []struct {
		name       string
		setupSrv   func(t *testing.T) *httptest.Server
		repoURL    string
		commitHash string
		wantCount  int
		wantErr    bool
	}{
		{
			name:       "returns vulnerabilities for commit",
			setupSrv:   fakeOSVServer,
			repoURL:    "https://github.com/example/repo",
			commitHash: "abc123def456",
			wantCount:  2,
			wantErr:    false,
		},
		{
			name:       "returns empty for clean commit",
			setupSrv:   fakeOSVServerNoVulns,
			repoURL:    "https://github.com/example/repo",
			commitHash: "cleancommit123",
			wantCount:  0,
			wantErr:    false,
		},
		{
			name:       "returns error for server error",
			setupSrv:   fakeOSVServerError,
			repoURL:    "https://github.com/example/repo",
			commitHash: "errorcommit",
			wantCount:  0,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupSrv(t)
			defer server.Close()

			client := newTestClient(server.URL)
			vulns, err := client.QueryByGitCommit(context.Background(), tt.repoURL, tt.commitHash)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(vulns) != tt.wantCount {
				t.Errorf("got %d vulnerabilities, want %d", len(vulns), tt.wantCount)
			}
		})
	}
}

func TestQueryByGitCommit_ResponseParsing(t *testing.T) {
	server := fakeOSVServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	vulns, err := client.QueryByGitCommit(context.Background(), "https://github.com/example/repo", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vulns) == 0 {
		t.Fatal("expected at least 1 vulnerability")
	}

	first := vulns[0]

	if first.ID != "GHSA-xxxx-yyyy-zzzz" {
		t.Errorf("ID = %q, want %q", first.ID, "GHSA-xxxx-yyyy-zzzz")
	}
	if first.Summary != "Critical vulnerability in example package" {
		t.Errorf("Summary = %q, want %q", first.Summary, "Critical vulnerability in example package")
	}
	// QueryByGitCommit always uses osv.dev fallback URL
	wantURL := "https://osv.dev/vulnerability/GHSA-xxxx-yyyy-zzzz"
	if first.URL != wantURL {
		t.Errorf("URL = %q, want %q", first.URL, wantURL)
	}
}

func TestQueryByPackage_InvalidServerURL(t *testing.T) {
	client := newTestClient("http://127.0.0.1:1") // connection refused
	_, err := client.QueryByPackage(context.Background(), "Go", "example.com/pkg", "1.0.0")
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}

func TestQueryByGitCommit_InvalidServerURL(t *testing.T) {
	client := newTestClient("http://127.0.0.1:1") // connection refused
	_, err := client.QueryByGitCommit(context.Background(), "https://github.com/example/repo", "abc123")
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}

func TestQueryByPackage_CancelledContext(t *testing.T) {
	server := fakeOSVServer(t)
	defer server.Close()
	client := newTestClient(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.QueryByPackage(ctx, "Go", "example.com/pkg", "1.0.0")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.baseURL != osvAPIURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, osvAPIURL)
	}
	if c.httpClient == nil {
		t.Fatal("expected non-nil httpClient")
	}
}
