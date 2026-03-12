package githubapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testApp creates an App with a real RSA key pointing at a test server.
func testApp(t *testing.T, serverURL string) *App {
	t.Helper()
	keyPath, _ := writeTestKey(t)

	app, err := New(Config{AppID: 100, PrivateKeyPath: keyPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	app.baseURL = serverURL
	return app
}

func TestListInstallations_Success(t *testing.T) {
	installations := []Installation{
		{ID: 1},
		{ID: 2},
	}
	installations[0].Account.Login = "org-a"
	installations[0].Account.Type = "Organization"
	installations[1].Account.Login = "user-b"
	installations[1].Account.Type = "User"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(installations); err != nil {
			t.Fatal(err)
		}
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	got, err := app.ListInstallations(context.Background())
	if err != nil {
		t.Fatalf("ListInstallations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d installations, want 2", len(got))
	}
	if got[0].Account.Login != "org-a" {
		t.Errorf("got[0].Account.Login = %q, want org-a", got[0].Account.Login)
	}
	if got[1].Account.Login != "user-b" {
		t.Errorf("got[1].Account.Login = %q, want user-b", got[1].Account.Login)
	}
}

func TestListInstallations_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	_, err := app.ListInstallations(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestListInstallations_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	got, err := app.ListInstallations(context.Background())
	if err != nil {
		t.Fatalf("ListInstallations: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d installations, want 0", len(got))
	}
}

func TestGetInstallationToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations/42/access_tokens" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"ghs_test_token_123"}`))
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	token, err := app.GetInstallationToken(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetInstallationToken: %v", err)
	}
	if token != "ghs_test_token_123" {
		t.Errorf("token = %q, want ghs_test_token_123", token)
	}
}

func TestGetInstallationToken_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	_, err := app.GetInstallationToken(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestListRepos_Success(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.URL.Path {
		case "/app/installations/10/access_tokens":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"ghs_abc"}`))
		case "/installation/repositories":
			if r.Header.Get("Authorization") != "token ghs_abc" {
				t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"repositories":[{"full_name":"org/repo1"},{"full_name":"org/repo2"}]}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	repos, err := app.ListRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(repos))
	}
	if repos[0] != "org/repo1" {
		t.Errorf("repos[0] = %q, want org/repo1", repos[0])
	}
	if repos[1] != "org/repo2" {
		t.Errorf("repos[1] = %q, want org/repo2", repos[1])
	}
}

func TestListRepos_TokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	_, err := app.ListRepos(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error when token request fails")
	}
}

func TestListRepos_EmptyRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations/10/access_tokens":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"ghs_abc"}`))
		case "/installation/repositories":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"repositories":[]}`))
		}
	}))
	defer srv.Close()

	app := testApp(t, srv.URL)
	repos, err := app.ListRepos(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("got %d repos, want 0", len(repos))
	}
}
