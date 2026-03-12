package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
services:
  - name: api
    repo: github.com/org/api
server:
  port: 9090
cache:
  ttl: 30m
tokens:
  github: file-token
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Cache.TTL != "30m" {
		t.Errorf("ttl = %q, want 30m", cfg.Cache.TTL)
	}
	if len(cfg.Services) != 1 || cfg.Services[0].Name != "api" {
		t.Errorf("services = %v, want [{api ...}]", cfg.Services)
	}
	if cfg.Tokens.GitHub != "file-token" {
		t.Errorf("github token = %q, want file-token", cfg.Tokens.GitHub)
	}
	// Defaults should be applied
	if cfg.RateLimit.GitHub != 5 {
		t.Errorf("rate_limit.github = %v, want 5", cfg.RateLimit.GitHub)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("log.level = %q, want info", cfg.Log.Level)
	}
}

func TestLoad_MissingFile_ReturnsDefault(t *testing.T) {
	cfg, err := Load("/tmp/releasewave-test-nonexistent-12345.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Should get default config
	if cfg.Server.Port != 7891 {
		t.Errorf("port = %d, want default 7891", cfg.Server.Port)
	}
	if cfg.Cache.TTL != "15m" {
		t.Errorf("ttl = %q, want default 15m", cfg.Cache.TTL)
	}
	if cfg.RateLimit.GitHub != 5 {
		t.Errorf("rate_limit.github = %v, want default 5", cfg.RateLimit.GitHub)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(cfgPath, []byte(`{{{not yaml at all`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_ValidationError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
services:
  - name: svc
    repo: bad-repo
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected validation error for bad repo format")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
tokens:
  github: from-file
  gitlab: from-file
server:
  api_key: from-file
`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GITHUB_TOKEN", "env-gh-token")
	t.Setenv("GITLAB_TOKEN", "env-gl-token")
	t.Setenv("RELEASEWAVE_API_KEY", "env-api-key")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Tokens.GitHub != "env-gh-token" {
		t.Errorf("github token = %q, want env-gh-token", cfg.Tokens.GitHub)
	}
	if cfg.Tokens.GitLab != "env-gl-token" {
		t.Errorf("gitlab token = %q, want env-gl-token", cfg.Tokens.GitLab)
	}
	if cfg.Server.APIKey != "env-api-key" {
		t.Errorf("api_key = %q, want env-api-key", cfg.Server.APIKey)
	}
}

func TestLoad_EnvNotSet_KeepsFileValues(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
tokens:
  github: from-file
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Ensure env vars are not set
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("RELEASEWAVE_API_KEY", "")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Tokens.GitHub != "from-file" {
		t.Errorf("github token = %q, want from-file", cfg.Tokens.GitHub)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Port != 7891 {
		t.Errorf("port = %d, want 7891", cfg.Server.Port)
	}
	if cfg.Cache.TTL != "15m" {
		t.Errorf("ttl = %q, want 15m", cfg.Cache.TTL)
	}
	if cfg.RateLimit.GitHub != 5 {
		t.Errorf("rate_limit.github = %v, want 5", cfg.RateLimit.GitHub)
	}
	if cfg.RateLimit.GitLab != 3 {
		t.Errorf("rate_limit.gitlab = %v, want 3", cfg.RateLimit.GitLab)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("log.level = %q, want info", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("log.format = %q, want text", cfg.Log.Format)
	}
}

func TestApplyDefaults_DoesNotOverrideSetValues(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{Port: 3000},
		Cache:     CacheConfig{TTL: "5m"},
		RateLimit: RateLimitConfig{GitHub: 10, GitLab: 8},
		Log:       LogConfig{Level: "debug", Format: "json"},
	}
	cfg.applyDefaults()

	if cfg.Server.Port != 3000 {
		t.Errorf("port = %d, want 3000 (should not override)", cfg.Server.Port)
	}
	if cfg.Cache.TTL != "5m" {
		t.Errorf("ttl = %q, want 5m (should not override)", cfg.Cache.TTL)
	}
	if cfg.RateLimit.GitHub != 10 {
		t.Errorf("rate_limit.github = %v, want 10 (should not override)", cfg.RateLimit.GitHub)
	}
	if cfg.RateLimit.GitLab != 8 {
		t.Errorf("rate_limit.gitlab = %v, want 8 (should not override)", cfg.RateLimit.GitLab)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log.level = %q, want debug (should not override)", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("log.format = %q, want json (should not override)", cfg.Log.Format)
	}
}

func TestParseRepo_UnknownHost(t *testing.T) {
	svc, err := ParseRepo("bitbucket.org/org/repo")
	if err != nil {
		t.Fatalf("ParseRepo: %v", err)
	}
	// Unknown host should use the host string as platform
	if svc.Platform != "bitbucket.org" {
		t.Errorf("platform = %q, want bitbucket.org", svc.Platform)
	}
	if svc.Owner != "org" {
		t.Errorf("owner = %q, want org", svc.Owner)
	}
	if svc.RepoName != "repo" {
		t.Errorf("repo = %q, want repo", svc.RepoName)
	}
}

func TestParseRepo_TwoPartPath(t *testing.T) {
	_, err := ParseRepo("github.com/just-one")
	if err == nil {
		t.Error("expected error for two-part path")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	// Should end with the expected suffix
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("path = %q, expected to end with config.yaml", path)
	}
}

func TestValidate_MissingRepo(t *testing.T) {
	cfg := &Config{
		Services: []ServiceConfig{{Name: "svc", Repo: ""}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing repo")
	}
}

func TestValidate_PortBounds(t *testing.T) {
	cfg := &Config{Server: ServerConfig{Port: -1}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative port")
	}
	cfg = &Config{Server: ServerConfig{Port: 70000}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port > 65535")
	}
}
