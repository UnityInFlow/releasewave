// Package security provides vulnerability checking via the OSV.dev API.
package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const osvAPIURL = "https://api.osv.dev/v1"

// Vulnerability represents a security advisory from OSV.dev.
type Vulnerability struct {
	ID       string   `json:"id"`
	Summary  string   `json:"summary"`
	Severity string   `json:"severity,omitempty"`
	Aliases  []string `json:"aliases,omitempty"` // CVE IDs
	URL      string   `json:"url"`
	Fixed    string   `json:"fixed,omitempty"` // version that fixes it
}

// Client queries the OSV.dev vulnerability database.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// New creates an OSV.dev client.
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    osvAPIURL,
	}
}

// osvQueryRequest is the JSON body for OSV API queries.
type osvQueryRequest struct {
	Package *osvPackage `json:"package,omitempty"`
	Version string     `json:"version,omitempty"`
}

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// osvQueryResponse is the API response.
type osvQueryResponse struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvVuln struct {
	ID       string        `json:"id"`
	Summary  string        `json:"summary"`
	Aliases  []string      `json:"aliases"`
	Severity []osvSeverity `json:"severity"`
	Affected []osvAffected `json:"affected"`
	References []osvRef    `json:"references"`
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type osvAffected struct {
	Ranges []osvRange `json:"ranges"`
}

type osvRange struct {
	Type   string      `json:"type"`
	Events []osvEvent  `json:"events"`
}

type osvEvent struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

type osvRef struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// QueryByPackage checks for vulnerabilities affecting a package version.
// ecosystem: "Go", "npm", "PyPI", "Maven", "crates.io", etc.
func (c *Client) QueryByPackage(ctx context.Context, ecosystem, pkg, version string) ([]Vulnerability, error) {
	slog.Debug("osv.query", "ecosystem", ecosystem, "package", pkg, "version", version)

	reqBody := osvQueryRequest{
		Package: &osvPackage{Name: pkg, Ecosystem: ecosystem},
		Version: version,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/query", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("osv query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv query failed: HTTP %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var osvResp osvQueryResponse
	if err := json.Unmarshal(respBody, &osvResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	vulns := make([]Vulnerability, 0, len(osvResp.Vulns))
	for _, v := range osvResp.Vulns {
		severity := ""
		if len(v.Severity) > 0 {
			severity = v.Severity[0].Score
		}

		url := ""
		for _, ref := range v.References {
			if ref.Type == "ADVISORY" || ref.Type == "WEB" {
				url = ref.URL
				break
			}
		}
		if url == "" {
			url = fmt.Sprintf("https://osv.dev/vulnerability/%s", v.ID)
		}

		fixed := ""
		for _, aff := range v.Affected {
			for _, r := range aff.Ranges {
				for _, ev := range r.Events {
					if ev.Fixed != "" {
						fixed = ev.Fixed
					}
				}
			}
		}

		vulns = append(vulns, Vulnerability{
			ID:       v.ID,
			Summary:  v.Summary,
			Severity: severity,
			Aliases:  v.Aliases,
			URL:      url,
			Fixed:    fixed,
		})
	}

	slog.Debug("osv.query.done", "package", pkg, "vulns", len(vulns))
	return vulns, nil
}

// QueryByGitCommit checks for vulnerabilities by git commit hash.
func (c *Client) QueryByGitCommit(ctx context.Context, repoURL, commitHash string) ([]Vulnerability, error) {
	type commitQuery struct {
		Commit string `json:"commit"`
	}

	body, err := json.Marshal(commitQuery{Commit: commitHash})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/query", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv commit query: HTTP %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var osvResp osvQueryResponse
	if err := json.Unmarshal(respBody, &osvResp); err != nil {
		return nil, err
	}

	vulns := make([]Vulnerability, 0, len(osvResp.Vulns))
	for _, v := range osvResp.Vulns {
		vulns = append(vulns, Vulnerability{
			ID:      v.ID,
			Summary: v.Summary,
			Aliases: v.Aliases,
			URL:     fmt.Sprintf("https://osv.dev/vulnerability/%s", v.ID),
		})
	}

	return vulns, nil
}
