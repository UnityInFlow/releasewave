package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestProviderError_Error_WithStatusCode(t *testing.T) {
	pe := &ProviderError{Platform: "github", Status: 404, Message: "not found"}
	got := pe.Error()
	want := "github: HTTP 404: not found"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProviderError_Error_WithoutStatusCode(t *testing.T) {
	pe := &ProviderError{Platform: "gitlab", Status: 0, Message: "connection refused"}
	got := pe.Error()
	want := "gitlab: connection refused"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProviderError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	pe := &ProviderError{Platform: "github", Message: "wrapped", Err: inner}
	if pe.Unwrap() != inner {
		t.Errorf("Unwrap() did not return the wrapped error")
	}
}

func TestProviderError_Unwrap_Nil(t *testing.T) {
	pe := &ProviderError{Platform: "github", Message: "no inner"}
	if pe.Unwrap() != nil {
		t.Errorf("Unwrap() should return nil when no wrapped error")
	}
}

func TestNewProviderError_401(t *testing.T) {
	pe := NewProviderError("github", 401, "https://api.github.com/repos")
	if !errors.Is(pe, ErrAuth) {
		t.Errorf("401 should wrap ErrAuth")
	}
}

func TestNewProviderError_403(t *testing.T) {
	pe := NewProviderError("github", 403, "https://api.github.com/repos")
	if !errors.Is(pe, ErrAuth) {
		t.Errorf("403 should wrap ErrAuth")
	}
}

func TestNewProviderError_404(t *testing.T) {
	pe := NewProviderError("github", 404, "https://api.github.com/repos/org/repo")
	if !errors.Is(pe, ErrNotFound) {
		t.Errorf("404 should wrap ErrNotFound")
	}
}

func TestNewProviderError_429(t *testing.T) {
	pe := NewProviderError("github", 429, "https://api.github.com/repos")
	if !errors.Is(pe, ErrRateLimit) {
		t.Errorf("429 should wrap ErrRateLimit")
	}
}

func TestNewProviderError_500(t *testing.T) {
	pe := NewProviderError("github", 500, "https://api.github.com/repos")
	if pe.Err != nil {
		t.Errorf("500 should have nil wrapped error, got %v", pe.Err)
	}
}

func TestIsNotFound(t *testing.T) {
	pe := NewProviderError("github", 404, "/some/url")
	if !IsNotFound(pe) {
		t.Error("IsNotFound should return true for 404 ProviderError")
	}
	if IsNotFound(fmt.Errorf("random error")) {
		t.Error("IsNotFound should return false for unrelated error")
	}
}

func TestIsRateLimit(t *testing.T) {
	pe := NewProviderError("github", 429, "/some/url")
	if !IsRateLimit(pe) {
		t.Error("IsRateLimit should return true for 429 ProviderError")
	}
	if IsRateLimit(fmt.Errorf("random error")) {
		t.Error("IsRateLimit should return false for unrelated error")
	}
}

func TestIsAuth(t *testing.T) {
	pe := NewProviderError("github", 401, "/some/url")
	if !IsAuth(pe) {
		t.Error("IsAuth should return true for 401 ProviderError")
	}
	if IsAuth(fmt.Errorf("random error")) {
		t.Error("IsAuth should return false for unrelated error")
	}
}

func TestConfigError_Error(t *testing.T) {
	ce := &ConfigError{Field: "api_token", Message: "must not be empty"}
	got := ce.Error()
	want := "config error: api_token: must not be empty"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
