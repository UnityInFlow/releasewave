// Package config handles loading and parsing the ReleaseWave configuration file.
//
// Go learning notes:
//   - os.ReadFile reads an entire file into a []byte (byte slice)
//   - yaml.Unmarshal parses YAML bytes into a Go struct
//   - os.UserHomeDir() returns the user's home directory path
//   - filepath.Join safely joins path components (handles / correctly)
//   - the `yaml:"key"` tags map YAML keys to struct fields
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UnityInFlow/releasewave/internal/model"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
type Config struct {
	Services []ServiceConfig `yaml:"services"`
	Cache    CacheConfig     `yaml:"cache"`
	Server   ServerConfig    `yaml:"server"`
	Tokens   TokenConfig     `yaml:"tokens"`
}

// ServiceConfig defines a tracked microservice.
type ServiceConfig struct {
	Name     string `yaml:"name"`
	Repo     string `yaml:"repo"`     // e.g. "github.com/org/repo"
	Registry string `yaml:"registry"` // e.g. "ghcr.io/org/repo"
}

// CacheConfig controls caching behavior.
type CacheConfig struct {
	TTL string `yaml:"ttl"` // e.g. "15m"
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
			// Return default config if file doesn't exist
			return defaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 7891
	}

	return &cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{Port: 7891},
		Cache:  CacheConfig{TTL: "15m"},
	}
}

// ParseRepo splits a repo string like "github.com/org/repo" into platform, owner, repo.
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
