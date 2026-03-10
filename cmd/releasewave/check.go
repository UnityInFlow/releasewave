package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check all configured services for latest versions",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Services) == 0 {
			fmt.Println("No services configured.")
			fmt.Println("Add services to ~/.config/releasewave/config.yaml or run: releasewave init")
			return nil
		}

		providers := map[string]provider.Provider{
			"github": gh.New(cfg.Tokens.GitHub, gh.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitHub, 10))),
			"gitlab": gl.New(cfg.Tokens.GitLab, gl.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitLab, 10))),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		type result struct {
			name     string
			platform string
			tag      string
			url      string
			err      string
		}

		results := make([]result, len(cfg.Services))
		var wg sync.WaitGroup

		for i, svc := range cfg.Services {
			wg.Add(1)
			go func(idx int, svc config.ServiceConfig) {
				defer wg.Done()

				parsed, err := config.ParseRepo(svc.Repo)
				if err != nil {
					results[idx] = result{name: svc.Name, err: err.Error()}
					return
				}

				p, ok := providers[parsed.Platform]
				if !ok {
					results[idx] = result{name: svc.Name, platform: parsed.Platform, err: "unsupported platform"}
					return
				}

				release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
				if err != nil {
					results[idx] = result{name: svc.Name, platform: parsed.Platform, err: err.Error()}
					return
				}

				results[idx] = result{
					name:     svc.Name,
					platform: parsed.Platform,
					tag:      release.Tag,
					url:      release.HTMLURL,
				}
			}(i, svc)
		}

		wg.Wait()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "SERVICE\tPLATFORM\tLATEST\tURL\n")
		fmt.Fprintf(w, "-------\t--------\t------\t---\n")
		for _, r := range results {
			if r.err != "" {
				fmt.Fprintf(w, "%s\t%s\t%s\t\n", r.name, r.platform, "error: "+r.err)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.name, r.platform, r.tag, r.url)
			}
		}
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
