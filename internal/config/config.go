package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/UnityInFlow/releasewave/internal/model"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
type Config struct {
	Services      []ServiceConfig    `yaml:"services"`
	Cache         CacheConfig        `yaml:"cache"`
	Server        ServerConfig       `yaml:"server"`
	Tokens        TokenConfig        `yaml:"tokens"`
	RateLimit     RateLimitConfig    `yaml:"rate_limit"`
	Log           LogConfig          `yaml:"log"`
	Notifications NotificationConfig `yaml:"notifications"`
}

// NotificationConfig controls release notifications.
type NotificationConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	Enabled    bool   `yaml:"enabled"`
}

// ServiceConfig defines a tracked microservice.
type ServiceConfig struct {
	Name     string `yaml:"name"`
	Repo     string `yaml:"repo"`
	Registry string `yaml:"registry"`
}

// CacheConfig controls caching behavior.
type CacheConfig struct {
	TTL string `yaml:"ttl"`
}

// TTLDuration parses the TTL string into a time.Duration. Defaults to 15m.
func (c CacheConfig) TTLDuration() time.Duration {
	if c.TTL == "" {
		return 15 * time.Minute
	}
	d, err := time.ParseDuration(c.TTL)
	if err != nil {
		return 15 * time.Minute
	}
	return d
}

// ServerConfig controls the MCP server.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// TokenConfig holds API tokens for various platforms.
type TokenConfig struct {
	GitHub string `yaml:"github"`
	GitLab string `yaml:"gitlab"`
}

// RateLimitConfig controls per-provider rate limits (requests per second).
type RateLimitConfig struct {
	GitHub float64 `yaml:"github"`
	GitLab float64 `yaml:"gitlab"`
}

// LogConfig controls logging.
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	seen := make(map[string]bool)
	for i, svc := range c.Services {
		if svc.Name == "" {
			return fmt.Errorf("services[%d]: name is required", i)
		}
		if svc.Repo == "" {
			return fmt.Errorf("services[%d] (%s): repo is required", i, svc.Name)
		}
		parts := strings.Split(svc.Repo, "/")
		if len(parts) < 3 {
			return fmt.Errorf("services[%d] (%s): repo must be host/owner/repo format, got %q", i, svc.Name, svc.Repo)
		}
		if seen[svc.Name] {
			return fmt.Errorf("services[%d]: duplicate service name %q", i, svc.Name)
		}
		seen[svc.Name] = true
	}

	if c.Cache.TTL != "" {
		if _, err := time.ParseDuration(c.Cache.TTL); err != nil {
			return fmt.Errorf("cache.ttl: invalid duration %q", c.Cache.TTL)
		}
	}

	if c.Server.Port < 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port: must be 0-65535, got %d", c.Server.Port)
	}

	return nil
}

// DefaultConfigPath returns ~/.config/releasewave/config.yaml
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".config", "releasewave", "config.yaml"), nil
}

// Load reads and parses a config file. If path is empty, uses the default path.
func Load(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Override tokens from environment if set
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		cfg.Tokens.GitHub = t
	}
	if t := os.Getenv("GITLAB_TOKEN"); t != "" {
		cfg.Tokens.GitLab = t
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 7891
	}
	if c.Cache.TTL == "" {
		c.Cache.TTL = "15m"
	}
	if c.RateLimit.GitHub == 0 {
		c.RateLimit.GitHub = 5
	}
	if c.RateLimit.GitLab == 0 {
		c.RateLimit.GitLab = 3
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "text"
	}
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	cfg := &Config{}
	cfg.applyDefaults()
	return cfg
}

// ParseRepo splits a repo string like "github.com/org/repo" into a Service.
func ParseRepo(repo string) (*model.Service, error) {
	parts := strings.Split(repo, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid repo format %q, expected host/owner/repo", repo)
	}

	host := parts[0]
	var platform string
	switch {
	case strings.Contains(host, "github"):
		platform = "github"
	case strings.Contains(host, "gitlab"):
		platform = "gitlab"
	default:
		platform = host
	}

	return &model.Service{
		Repo:     repo,
		Platform: platform,
		Owner:    parts[1],
		RepoName: parts[2],
	}, nil
}

// ExampleConfig is the default config file content.
const ExampleConfig = `# ReleaseWave configuration
# https://github.com/UnityInFlow/releasewave

# Microservices to track
services:
  # - name: my-api
  #   repo: github.com/my-org/my-api
  #   registry: ghcr.io/my-org/my-api
  # - name: billing
  #   repo: gitlab.com/my-org/billing

# API tokens (can also use GITHUB_TOKEN / GITLAB_TOKEN env vars)
tokens:
  github: ""
  gitlab: ""

# Cache settings
cache:
  ttl: 15m

# MCP server settings
server:
  port: 7891

# Rate limiting (requests per second)
rate_limit:
  github: 5
  gitlab: 3

# Logging
log:
  level: info    # debug, info, warn, error
  format: text   # text, json

# Notifications
notifications:
  webhook_url: ""
  enabled: false
`
