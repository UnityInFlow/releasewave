package config

import (
	"testing"
	"time"
)

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc1", Repo: "github.com/org/repo1"},
			{Name: "svc2", Repo: "gitlab.com/org/repo2"},
		},
		Cache:  CacheConfig{TTL: "10m"},
		Server: ServerConfig{Port: 8080},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingName(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{{Name: "", Repo: "github.com/org/repo"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestValidate_BadRepo(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{{Name: "svc", Repo: "bad-format"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for bad repo format")
	}
}

func TestValidate_DuplicateNames(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{
			{Name: "svc", Repo: "github.com/org/repo1"},
			{Name: "svc", Repo: "github.com/org/repo2"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for duplicate names")
	}
}

func TestValidate_BadTTL(t *testing.T) {
	cfg := &Config{Cache: CacheConfig{TTL: "not-a-duration"}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for bad TTL")
	}
}

func TestTTLDuration(t *testing.T) {
	tests := []struct {
		ttl  string
		want time.Duration
	}{
		{"", 15 * time.Minute},
		{"5m", 5 * time.Minute},
		{"1h", 1 * time.Hour},
		{"invalid", 15 * time.Minute},
	}

	for _, tt := range tests {
		c := CacheConfig{TTL: tt.ttl}
		if got := c.TTLDuration(); got != tt.want {
			t.Errorf("TTLDuration(%q) = %v, want %v", tt.ttl, got, tt.want)
		}
	}
}

func TestValidate_RateLimitBounds(t *testing.T) {
	cfg := &Config{RateLimit: RateLimitConfig{GitHub: 2000}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for rate limit > 1000")
	}
	cfg = &Config{RateLimit: RateLimitConfig{GitLab: -1}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative rate limit")
	}
}

func TestValidate_WebhookURL(t *testing.T) {
	cfg := &Config{Notifications: NotificationConfig{WebhookURL: "ftp://invalid"}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for non-http webhook URL")
	}
	cfg = &Config{Notifications: NotificationConfig{WebhookURL: "https://hooks.slack.com/xxx"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for valid https webhook URL, got: %v", err)
	}
}

func TestValidate_LogLevel(t *testing.T) {
	cfg := &Config{Log: LogConfig{Level: "verbose"}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid log level")
	}
	cfg = &Config{Log: LogConfig{Level: "debug"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for valid log level, got: %v", err)
	}
}

func TestValidate_LogFormat(t *testing.T) {
	cfg := &Config{Log: LogConfig{Format: "xml"}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid log format")
	}
	cfg = &Config{Log: LogConfig{Format: "json"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for valid log format, got: %v", err)
	}
}

func TestParseRepo(t *testing.T) {
	tests := []struct {
		input    string
		platform string
		owner    string
		repo     string
		wantErr  bool
	}{
		{"github.com/org/repo", "github", "org", "repo", false},
		{"gitlab.com/org/repo", "gitlab", "org", "repo", false},
		{"bad", "", "", "", true},
	}

	for _, tt := range tests {
		svc, err := ParseRepo(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseRepo(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseRepo(%q): %v", tt.input, err)
		}
		if svc.Platform != tt.platform {
			t.Errorf("platform = %q, want %q", svc.Platform, tt.platform)
		}
		if svc.Owner != tt.owner {
			t.Errorf("owner = %q, want %q", svc.Owner, tt.owner)
		}
		if svc.RepoName != tt.repo {
			t.Errorf("repo = %q, want %q", svc.RepoName, tt.repo)
		}
	}
}
