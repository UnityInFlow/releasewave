// Package github implements the Provider interface for GitHub.
//
// Go learning notes:
//   - encoding/json is the standard library for JSON parsing
//   - fmt.Sprintf is like printf but returns a string
//   - defer resp.Body.Close() ensures the body is closed when the function returns
//   - slices are dynamic arrays — append() adds elements
//   - &variable gives you a pointer to that variable
//   - struct{} embedding (not used here yet) is Go's version of composition
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/UnityInFlow/releasewave/internal/model"
)

const baseURL = "https://api.github.com"

// Client is the GitHub provider. It implements the provider.Provider interface.
type Client struct {
	httpClient *http.Client
	token      string
}

// New creates a new GitHub client.
// The token is optional — without it, you get 60 requests/hour (with: 5000/hour).
func New(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
	}
}

// Name returns "github".
func (c *Client) Name() string {
	return "github"
}

// githubRelease is the JSON shape returned by the GitHub API.
// We keep this private (lowercase first letter) — it's an implementation detail.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

// githubTag is the JSON shape for tags from the GitHub API.
type githubTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// ListReleases fetches all releases for a GitHub repository.
func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]model.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", baseURL, owner, repo)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	var ghReleases []githubRelease
	if err := json.Unmarshal(body, &ghReleases); err != nil {
		return nil, fmt.Errorf("parse releases JSON: %w", err)
	}

	releases := make([]model.Release, 0, len(ghReleases))
	for _, gr := range ghReleases {
		publishedAt, _ := time.Parse(time.RFC3339, gr.PublishedAt)
		releases = append(releases, model.Release{
			Tag:         gr.TagName,
			Name:        gr.Name,
			Body:        gr.Body,
			Draft:       gr.Draft,
			Prerelease:  gr.Prerelease,
			PublishedAt: publishedAt,
			HTMLURL:     gr.HTMLURL,
		})
	}

	return releases, nil
}

// GetLatestRelease fetches the latest release for a GitHub repository.
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*model.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", baseURL, owner, repo)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("get latest release: %w", err)
	}

	var gr githubRelease
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("parse release JSON: %w", err)
	}

	publishedAt, _ := time.Parse(time.RFC3339, gr.PublishedAt)
	return &model.Release{
		Tag:         gr.TagName,
		Name:        gr.Name,
		Body:        gr.Body,
		Draft:       gr.Draft,
		Prerelease:  gr.Prerelease,
		PublishedAt: publishedAt,
		HTMLURL:     gr.HTMLURL,
	}, nil
}

// ListTags fetches all tags for a GitHub repository.
func (c *Client) ListTags(ctx context.Context, owner, repo string) ([]model.Tag, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=30", baseURL, owner, repo)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	var ghTags []githubTag
	if err := json.Unmarshal(body, &ghTags); err != nil {
		return nil, fmt.Errorf("parse tags JSON: %w", err)
	}

	tags := make([]model.Tag, 0, len(ghTags))
	for _, gt := range ghTags {
		tags = append(tags, model.Tag{
			Name:   gt.Name,
			Commit: gt.Commit.SHA,
		})
	}

	return tags, nil
}

// doRequest is a helper that makes an authenticated HTTP GET request.
func (c *Client) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	var buf []byte
	buf, err = readAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return buf, nil
}

// readAll reads all bytes from a reader. Separated for clarity.
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var result []byte
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
	}
	return result, nil
}
