// Package gitlab implements the Provider interface for GitLab.
//
// ============================================================================
// GO LEARNING: Implementing an Interface (Second Time)
// ============================================================================
//
// This is the SAME Provider interface we implemented for GitHub.
// In Go, there's no "implements" keyword. A type satisfies an interface
// simply by having all the methods the interface requires.
//
// Compare this file to github.go — you'll see the same pattern:
//   1. A Client struct with httpClient, token, baseURL
//   2. A constructor function New(...)
//   3. Methods that match the Provider interface
//   4. Private types for API JSON shapes
//   5. A helper doRequest for HTTP calls
//
// This is how Go achieves polymorphism:
//   var p provider.Provider
//   p = github.New("token")   // works!
//   p = gitlab.New("token")   // also works!
//   p.ListReleases(...)       // calls the right implementation
//
// ============================================================================
//
// GO LEARNING: GitLab API Differences from GitHub
//   - GitLab uses numeric project IDs or URL-encoded paths: "owner%2Frepo"
//   - Pagination via ?per_page=N (same concept, slightly different params)
//   - Auth header: "PRIVATE-TOKEN: xxx" instead of "Authorization: Bearer xxx"
//   - Base URL: gitlab.com/api/v4 (not api.gitlab.com)
//
// ============================================================================

package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/UnityInFlow/releasewave/internal/model"
)

const defaultBaseURL = "https://gitlab.com/api/v4"

// Client is the GitLab provider. It implements provider.Provider.
//
// GO LEARNING: Struct Embedding & Composition
//   Notice this struct looks almost identical to the GitHub one.
//   In a larger project, you might extract shared fields into a base struct
//   and embed it. But for now, duplication is fine — "a little copying is
//   better than a little dependency" is a Go proverb.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
}

// New creates a new GitLab client.
//
// GO LEARNING: Constructor Pattern
//   Go doesn't have constructors. By convention, we use a function called New()
//   that returns a pointer to the struct. This is where you set defaults.
//   The caller can override fields after creation if needed.
func New(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
		baseURL:    defaultBaseURL,
	}
}

// Name returns "gitlab".
func (c *Client) Name() string {
	return "gitlab"
}

// GO LEARNING: Private Types for JSON Mapping
//
// These types map to GitLab's API JSON response shape.
// They're lowercase (private) because they're implementation details.
// We convert them to our shared model.Release/model.Tag types before returning.
//
// Why not use model.Release directly?
//   - API field names differ (GitLab: "tag_name", our model: "tag")
//   - API may have extra fields we don't need
//   - Decouples our model from any specific API shape
//   - Each provider maps its own API → shared model

type gitlabRelease struct {
	TagName     string          `json:"tag_name"`
	Name        string          `json:"name"`
	Description string          `json:"description"` // GitLab uses "description" not "body"
	ReleasedAt  string          `json:"released_at"`
	Links       gitlabLinks     `json:"_links"`
	Upcoming    bool            `json:"upcoming_release"`
}

type gitlabLinks struct {
	Self string `json:"self"`
}

type gitlabTag struct {
	Name   string       `json:"name"`
	Commit gitlabCommit `json:"commit"`
}

type gitlabCommit struct {
	ID string `json:"id"` // GitLab uses "id" for SHA, GitHub uses "sha"
}

// projectPath URL-encodes the owner/repo path for GitLab API.
//
// GO LEARNING: url.PathEscape
//   GitLab API needs "owner/repo" to be URL-encoded as "owner%2Frepo"
//   in the URL path. url.PathEscape does this safely.
//   Example: "myorg/myrepo" → "myorg%2Fmyrepo"
func projectPath(owner, repo string) string {
	return url.PathEscape(owner + "/" + repo)
}

// ListReleases fetches all releases for a GitLab project.
//
// GO LEARNING: Method Receivers
//   (c *Client) means this method "belongs to" *Client.
//   The 'c' is like 'this' or 'self' in other languages.
//   We use a pointer receiver (*Client) because:
//   1. It avoids copying the entire struct on each call
//   2. It lets us modify the struct if needed (we don't here, but convention)
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

	// GO LEARNING: make([]T, 0, cap)
	//   make creates a slice with length 0 and capacity cap.
	//   Capacity is a hint — Go won't need to re-allocate memory
	//   as we append, because we know the final size upfront.
	releases := make([]model.Release, 0, len(glReleases))
	for _, gr := range glReleases {
		releasedAt, _ := time.Parse(time.RFC3339, gr.ReleasedAt)

		// Build the web URL from the API self link or construct it
		htmlURL := fmt.Sprintf("https://gitlab.com/%s/%s/-/releases/%s", owner, repo, gr.TagName)

		releases = append(releases, model.Release{
			Tag:         gr.TagName,
			Name:        gr.Name,
			Body:        gr.Description,
			Draft:       false, // GitLab doesn't have draft releases
			Prerelease:  gr.Upcoming,
			PublishedAt: releasedAt,
			HTMLURL:     htmlURL,
		})
	}

	return releases, nil
}

// GetLatestRelease returns the most recent release.
//
// GO LEARNING: Returning Pointers
//   We return *model.Release (pointer) not model.Release (value).
//   This is a convention when the result might be "nothing" (nil).
//   The caller should check: if release == nil { ... }
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*model.Release, error) {
	// GitLab doesn't have a dedicated "latest" endpoint for releases,
	// so we fetch the first page (sorted by date) and take the first one.
	//
	// GO LEARNING: Reusing Your Own Methods
	//   We call c.ListReleases instead of duplicating the HTTP logic.
	//   This is simpler and means bug fixes in ListReleases apply here too.
	releases, err := c.ListReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for %s/%s", owner, repo)
	}

	// GO LEARNING: &releases[0]
	//   & takes the address of (pointer to) the first element.
	//   We return a pointer so the caller gets a reference, not a copy.
	return &releases[0], nil
}

// ListTags fetches all tags for a GitLab project.
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

	return tags, nil
}

// doRequest makes an authenticated HTTP GET request to the GitLab API.
//
// GO LEARNING: Error Wrapping with %w
//   fmt.Errorf("context: %w", err) wraps the original error.
//   The caller can later "unwrap" it with errors.Is() or errors.As()
//   to check the root cause. This is Go's error chain mechanism.
func (c *Client) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// GO LEARNING: GitLab vs GitHub Auth Headers
	//   GitHub: "Authorization: Bearer <token>"
	//   GitLab: "PRIVATE-TOKEN: <token>"
	//   Each API has its own conventions — that's why we have separate providers.
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return buf, nil
}
