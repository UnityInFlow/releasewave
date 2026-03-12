package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizePath_API(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/api/v1/services/my-svc/releases", "/api/v1/services/{name}/releases"},
		{"/api/v1/services/other", "/api/v1/services/{name}"},
		{"/api/v1/timeline", "/api/v1/timeline"},
		{"/api/health", "/api/health"},
	}
	for _, tt := range tests {
		got := normalizePath(tt.input)
		if got != tt.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePath_SSE(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/sse", "/sse"},
		{"/sse/some-session-id", "/sse"},
	}
	for _, tt := range tests {
		got := normalizePath(tt.input)
		if got != tt.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePath_Message(t *testing.T) {
	got := normalizePath("/message/abc123")
	if got != "/message" {
		t.Errorf("normalizePath(/message/abc123) = %q, want /message", got)
	}
}

func TestNormalizePath_Dashboard(t *testing.T) {
	got := normalizePath("/dashboard/settings")
	if got != "/dashboard" {
		t.Errorf("normalizePath(/dashboard/settings) = %q, want /dashboard", got)
	}
}

func TestNormalizePath_UnknownCapped(t *testing.T) {
	// Unknown paths with many segments get capped at 3.
	got := normalizePath("/foo/bar/baz/qux/extra")
	if got != "/foo/bar/baz" {
		t.Errorf("normalizePath(/foo/bar/baz/qux/extra) = %q, want /foo/bar/baz", got)
	}
}

func TestNormalizePath_ShortPathPassThrough(t *testing.T) {
	tests := []string{"/", "/health", "/metrics", "/foo/bar"}
	for _, p := range tests {
		got := normalizePath(p)
		if got != p {
			t.Errorf("normalizePath(%q) = %q, want passthrough", p, got)
		}
	}
}

func TestMetrics_RecordsStatus(t *testing.T) {
	handler := Metrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status 418, got %d", rec.Code)
	}
}

func TestMetrics_DefaultStatus(t *testing.T) {
	handler := Metrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected default status 200, got %d", rec.Code)
	}
}

func TestStatusRecorder_Flush(t *testing.T) {
	// httptest.ResponseRecorder implements http.Flusher.
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: 200}
	// Should not panic.
	sr.Flush()

	if !rec.Flushed {
		t.Error("expected Flush to delegate to underlying writer")
	}
}
