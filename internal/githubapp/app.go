// Package githubapp provides GitHub App integration for ReleaseWave.
package githubapp

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config holds GitHub App configuration.
type Config struct {
	AppID          int64  `yaml:"app_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
	WebhookSecret  string `yaml:"webhook_secret"`
}

// App represents a GitHub App instance.
type App struct {
	config     Config
	privateKey *rsa.PrivateKey
	httpClient *http.Client
}

// New creates a GitHub App instance.
func New(cfg Config) (*App, error) {
	if cfg.AppID == 0 || cfg.PrivateKeyPath == "" {
		return nil, fmt.Errorf("github app: app_id and private_key_path are required")
	}

	keyData, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return &App{
		config:     cfg,
		privateKey: key,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Installation represents a GitHub App installation.
type Installation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

// ListInstallations returns all installations of this GitHub App.
func (a *App) ListInstallations(ctx context.Context) ([]Installation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/app/installations", nil)
	if err != nil {
		return nil, err
	}

	jwt, err := a.generateJWT()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list installations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var installations []Installation
	if err := json.NewDecoder(resp.Body).Decode(&installations); err != nil {
		return nil, err
	}
	return installations, nil
}

// GetInstallationToken generates an access token for an installation.
func (a *App) GetInstallationToken(ctx context.Context, installationID int64) (string, error) {
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}

	jwt, err := a.generateJWT()
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Token, nil
}

// ListRepos lists repositories accessible to an installation.
func (a *App) ListRepos(ctx context.Context, installationID int64) ([]string, error) {
	token, err := a.GetInstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/installation/repositories", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Repositories []struct {
			FullName string `json:"full_name"`
		} `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	repos := make([]string, len(result.Repositories))
	for i, r := range result.Repositories {
		repos[i] = r.FullName
	}
	return repos, nil
}

func (a *App) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    strconv.FormatInt(a.config.AppID, 10),
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(a.privateKey)
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}

	slog.Debug("githubapp.jwt", "app_id", a.config.AppID)
	return signed, nil
}
