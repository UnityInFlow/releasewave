package gitlab

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	rwerrors "github.com/UnityInFlow/releasewave/internal/errors"
	"github.com/UnityInFlow/releasewave/internal/model"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

const defaultBaseURL = "https://gitlab.com/api/v4"

// Client is the GitLab provider.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	limiter    *ratelimit.Limiter
}

// Option configures a Client.
type Option func(*Client)

func WithBaseURL(url string) Option               { return func(c *Client) { c.baseURL = url } }
func WithRateLimiter(l *ratelimit.Limiter) Option { return func(c *Client) { c.limiter = l } }

// New creates a new GitLab client.
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

func (c *Client) Name() string { return "gitlab" }

type gitlabRelease struct {
	TagName     string      `json:"tag_name"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	ReleasedAt  string      `json:"released_at"`
	Links       gitlabLinks `json:"_links"`
	Upcoming    bool        `json:"upcoming_release"`
}

type gitlabLinks struct {
	Self string `json:"self"`
}

type gitlabTag struct {
	Name   string       `json:"name"`
	Commit gitlabCommit `json:"commit"`
}

type gitlabCommit struct {
	ID string `json:"id"`
}

func projectPath(owner, repo string) string {
	return url.PathEscape(owner + "/" + repo)
}

func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]model.Release, error) {
	path := projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s/releases?per_page=30", c.baseURL, path)

	body, err := c.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	var glReleases []gitlabRelease
	if err := json.Unmarshal(body, &glReleases); err != nil {
		return nil, fmt.Errorf("parse releases JSON: %w", err)
	}

	releases := make([]model.Release, 0, len(glReleases))
	for _, gr := range glReleases {
		releasedAt, err := time.Parse(time.RFC3339, gr.ReleasedAt)
		if err != nil && gr.ReleasedAt != "" {
			slog.Debug("gitlab.parse_time", "value", gr.ReleasedAt, "error", err)
		}
		htmlURL := fmt.Sprintf("https://gitlab.com/%s/%s/-/releases/%s", owner, repo, gr.TagName)
		releases = append(releases, model.Release{
			Tag:         gr.TagName,
			Name:        gr.Name,
			Body:        gr.Description,
			Draft:       false,
			Prerelease:  gr.Upcoming,
			PublishedAt: releasedAt,
			HTMLURL:     htmlURL,
		})
	}

	slog.Debug("gitlab.list_releases", "owner", owner, "repo", repo, "count", len(releases))
	return releases, nil
}

func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*model.Release, error) {
	releases, err := c.ListReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for %s/%s", owner, repo)
	}
	slog.Debug("gitlab.latest_release", "owner", owner, "repo", repo, "tag", releases[0].Tag)
	return &releases[0], nil
}

func (c *Client) ListTags(ctx context.Context, owner, repo string) ([]model.Tag, error) {
	path := projectPath(owner, repo)
	apiURL := fmt.Sprintf("%s/projects/%s/repository/tags?per_page=30", c.baseURL, path)

	body, err := c.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	var glTags []gitlabTag
	if err := json.Unmarshal(body, &glTags); err != nil {
		return nil, fmt.Errorf("parse tags JSON: %w", err)
	}

	tags := make([]model.Tag, 0, len(glTags))
	for _, gt := range glTags {
		tags = append(tags, model.Tag{
			Name:   gt.Name,
			Commit: gt.Commit.ID,
		})
	}

	slog.Debug("gitlab.list_tags", "owner", owner, "repo", repo, "count", len(tags))
	return tags, nil
}

func (c *Client) GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, error) {
	project := projectPath(owner, repo)
	encodedPath := url.PathEscape(path)
	apiURL := fmt.Sprintf("%s/projects/%s/repository/files/%s?ref=HEAD", c.baseURL, project, encodedPath)

	body, err := c.doRequest(ctx, apiURL)
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
		decoded, err := base64.StdEncoding.DecodeString(fileResp.Content)
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

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	slog.Debug("gitlab.request", "url", url)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, rwerrors.NewProviderError("gitlab", resp.StatusCode, url)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return buf, nil
}
