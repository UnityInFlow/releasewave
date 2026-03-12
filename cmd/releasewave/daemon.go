package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/daemon"
	"github.com/UnityInFlow/releasewave/internal/notify"
	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
	"github.com/UnityInFlow/releasewave/internal/store"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run a background polling daemon for release monitoring",
	Long:  "Polls configured services at a regular interval, recording releases\nand sending notifications when new versions are detected.",
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

		var notifier notify.Notifier
		if cfg.Notifications.Enabled {
			notifier = notify.FromConfig(
				cfg.Notifications.WebhookURL,
				cfg.Notifications.Slack.WebhookURL,
				cfg.Notifications.Discord.WebhookURL,
			)
		}

		var st *store.Store
		if cfg.Storage.Path != "" {
			var err error
			st, err = store.New(cfg.Storage.Path)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer st.Close()
		}

		d := daemon.New(cfg, providers, notifier, st, interval)

		if once {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			d.RunOnce(ctx)
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigCh
			fmt.Println("\nStopping daemon...")
			d.Stop()
			cancel()
		}()

		fmt.Printf("Daemon watching %d services (interval: %s)\n", len(cfg.Services), interval)
		d.Start(ctx)
		return nil
	},
}

func init() {
	daemonCmd.Flags().Duration("interval", 5*time.Minute, "polling interval (e.g. 1m, 5m, 30s)")
	daemonCmd.Flags().Bool("once", false, "run one poll cycle and exit")
	rootCmd.AddCommand(daemonCmd)
}
