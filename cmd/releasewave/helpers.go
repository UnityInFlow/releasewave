package main

import (
	"fmt"
	"os"
	"strings"

	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

// parseOwnerRepo splits "owner/repo" into two strings.
func parseOwnerRepo(arg string) (string, string) {
	parts := strings.SplitN(arg, "/", 2)
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "error: expected owner/repo format, got %q\n", arg)
		os.Exit(1)
	}
	return parts[0], parts[1]
}

// getGitHubClient creates a GitHub client from current config.
func getGitHubClient() *gh.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" && cfg != nil {
		token = cfg.Tokens.GitHub
	}

	var rps float64 = 5
	if cfg != nil && cfg.RateLimit.GitHub > 0 {
		rps = cfg.RateLimit.GitHub
	}

	return gh.New(token, gh.WithRateLimiter(ratelimit.New(rps, 10)))
}
