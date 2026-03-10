package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
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

// getProvider creates a provider for the given platform using current config.
func getProvider(platform string) provider.Provider {
	switch platform {
	case "gitlab":
		token := os.Getenv("GITLAB_TOKEN")
		if token == "" && cfg != nil {
			token = cfg.Tokens.GitLab
		}

		var rps float64 = 3
		if cfg != nil && cfg.RateLimit.GitLab > 0 {
			rps = cfg.RateLimit.GitLab
		}

		return gl.New(token, gl.WithRateLimiter(ratelimit.New(rps, 10)))

	default: // "github"
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
}
