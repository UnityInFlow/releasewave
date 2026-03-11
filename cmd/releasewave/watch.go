package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch for new releases and print updates",
	Long:  "Polls configured services for new releases on an interval.\nPrints a notification line when a new release is detected.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Services) == 0 {
			fmt.Println("No services configured.")
			return nil
		}

		interval, _ := cmd.Flags().GetDuration("interval")
		once, _ := cmd.Flags().GetBool("once")

		providers := map[string]provider.Provider{
			"github": gh.New(cfg.Tokens.GitHub, gh.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitHub, 10))),
			"gitlab": gl.New(cfg.Tokens.GitLab, gl.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitLab, 10))),
		}

		known := make(map[string]string)
		first := true

		check := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			var mu sync.Mutex

			for _, svc := range cfg.Services {
				wg.Add(1)
				go func(svc config.ServiceConfig) {
					defer wg.Done()

					parsed, err := config.ParseRepo(svc.Repo)
					if err != nil {
						return
					}

					p, ok := providers[parsed.Platform]
					if !ok {
						return
					}

					release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
					if err != nil {
						return
					}

					mu.Lock()
					defer mu.Unlock()

					old, seen := known[svc.Name]
					known[svc.Name] = release.Tag

					if first {
						fmt.Printf("  %s: %s\n", svc.Name, release.Tag)
						return
					}

					if seen && old != release.Tag {
						fmt.Printf("[NEW] %s  %s → %s  %s\n",
							time.Now().Format("15:04:05"),
							svc.Name,
							release.Tag,
							release.HTMLURL,
						)
					}
				}(svc)
			}

			wg.Wait()
		}

		fmt.Printf("Watching %d services (interval: %s):\n", len(cfg.Services), interval)
		check()
		first = false

		if once {
			return nil
		}

		fmt.Printf("\nPolling for changes...\n")

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				check()
			case <-sigCh:
				fmt.Println("\nStopped.")
				return nil
			}
		}
	},
}

func init() {
	watchCmd.Flags().Duration("interval", 5*time.Minute, "polling interval (e.g. 1m, 5m, 30s)")
	watchCmd.Flags().Bool("once", false, "check once and exit (don't poll)")
	rootCmd.AddCommand(watchCmd)
}
