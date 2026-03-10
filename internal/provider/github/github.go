package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	rwerrors "github.com/UnityInFlow/releasewave/internal/errors"
	"github.com/UnityInFlow/releasewave/internal/model"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

const defaultBaseURL = "https://api.github.com"

// Client is the GitHub provider.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	limiter    *ratelimit.Limiter
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithRateLimiter sets a rate limiter.
func WithRateLimiter(l *ratelimit.Limiter) Option {
	return func(c *Client) { c.limiter = l }
}

// New creates a new GitHub client.
func New(token string, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
		baseURL:    defaultBaseURL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Name() string { return "github" }

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

type githubTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]model.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", c.baseURL, owner, repo)

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

	slog.Debug("github.list_releases", "owner", owner, "repo", repo, "count", len(releases))
	return releases, nil
}

func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*model.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, owner, repo)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("get latest release: %w", err)
	}

	var gr githubRelease
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("parse release JSON: %w", err)
	}

	publishedAt, _ := time.Parse(time.RFC3339, gr.PublishedAt)
	release := &model.Release{
		Tag:         gr.TagName,
		Name:        gr.Name,
		Body:        gr.Body,
		Draft:       gr.Draft,
		Prerelease:  gr.Prerelease,
		PublishedAt: publishedAt,
		HTMLURL:     gr.HTMLURL,
	}

	slog.Debug("github.latest_release", "owner", owner, "repo", repo, "tag", release.Tag)
	return release, nil
}

func (c *Client) ListTags(ctx context.Context, owner, repo string) ([]model.Tag, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=30", c.baseURL, owner, repo)

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

	slog.Debug("github.list_tags", "owner", owner, "repo", repo, "count", len(tags))
	return tags, nil
}

func (c *Client) GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, owner, repo, path)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("get file content: %w", err)
	}

	var fileResp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return nil, fmt.Errorf("parse file response: %w", err)
	}

	if fileResp.Encoding == "base64" {
		clean := strings.ReplaceAll(fileResp.Content, "\n", "")
		decoded, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("decode base64 content: %w", err)
		}
		return decoded, nil
	}

	return []byte(fileResp.Content), nil
}

func (c *Client) doRequest(ctx context.Context, url string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	slog.Debug("github.request", "url", url)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, rwerrors.NewProviderError("github", resp.StatusCode, url)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return buf, nil
}
