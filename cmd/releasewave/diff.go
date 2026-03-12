package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/k8s"
	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

var diffCmd = &cobra.Command{
	Use:   "diff [service]",
	Short: "Show the diff between deployed and latest release",
	Long:  "Compares the deployed version (from K8s) against the latest release,\nshowing the changelog for versions in between.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName := args[0]
		namespace, _ := cmd.Flags().GetString("namespace")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
		kctx, _ := cmd.Flags().GetString("context")

		// Find service in config.
		var svc *config.ServiceConfig
		for i := range cfg.Services {
			if cfg.Services[i].Name == serviceName {
				svc = &cfg.Services[i]
				break
			}
		}
		if svc == nil {
			return fmt.Errorf("service %q not found in config", serviceName)
		}

		parsed, err := config.ParseRepo(svc.Repo)
		if err != nil {
			return err
		}

		providers := map[string]provider.Provider{
			"github": gh.New(cfg.Tokens.GitHub, gh.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitHub, 10))),
			"gitlab": gl.New(cfg.Tokens.GitLab, gl.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitLab, 10))),
		}

		p, ok := providers[parsed.Platform]
		if !ok {
			return fmt.Errorf("unsupported platform: %s", parsed.Platform)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get latest release and deployed version concurrently.
		var (
			wg          sync.WaitGroup
			deployedVer string
		)

		type releaseResult struct {
			tag string
			url string
			err error
		}
		latestCh := make(chan releaseResult, 1)

		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
			if err != nil {
				latestCh <- releaseResult{err: err}
				return
			}
			latestCh <- releaseResult{tag: release.Tag, url: release.HTMLURL}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			k8sClient, err := k8s.New(kubeconfig, kctx)
			if err != nil {
				return
			}
			deployed, err := k8sClient.ListAll(ctx, namespace)
			if err != nil {
				return
			}
			for _, d := range deployed {
				if d.Name == serviceName {
					deployedVer = d.AppVersion
					return
				}
			}
		}()

		wg.Wait()

		lr := <-latestCh
		if lr.err != nil {
			return fmt.Errorf("get latest release: %w", lr.err)
		}

		fmt.Printf("Service:  %s\n", serviceName)
		fmt.Printf("Latest:   %s\n", lr.tag)
		if deployedVer != "" {
			fmt.Printf("Deployed: %s\n", deployedVer)
		} else {
			fmt.Printf("Deployed: unknown\n")
		}
		fmt.Printf("URL:      %s\n", lr.url)

		if deployedVer != "" && deployedVer != lr.tag {
			// Get changelog.
			releases, err := p.ListReleases(ctx, parsed.Owner, parsed.RepoName)
			if err == nil {
				fmt.Printf("\nChangelog (%s → %s):\n", deployedVer, lr.tag)
				inRange := false
				for _, r := range releases {
					if r.Tag == lr.tag {
						inRange = true
					}
					if inRange {
						fmt.Printf("  %s  %s", r.Tag, r.PublishedAt.Format("2006-01-02"))
						if r.Name != "" && r.Name != r.Tag {
							fmt.Printf("  %s", r.Name)
						}
						fmt.Println()
					}
					tag := r.Tag
					if tag == deployedVer || tag == "v"+deployedVer || strings.TrimPrefix(tag, "v") == deployedVer {
						break
					}
				}
			}
		} else if deployedVer == lr.tag {
			fmt.Println("\nUp to date!")
		}

		return nil
	},
}

func init() {
	diffCmd.Flags().String("namespace", "default", "Kubernetes namespace")
	diffCmd.Flags().String("kubeconfig", "", "path to kubeconfig file")
	diffCmd.Flags().String("context", "", "Kubernetes context")
	rootCmd.AddCommand(diffCmd)
}
