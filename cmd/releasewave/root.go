package main

import (
	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/logging"
)

var (
	cfgFile  string
	logLevel string
	logFmt   string
	cfg      *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "releasewave",
	Short: "Universal release/version aggregator for microservices",
	Long:  "ReleaseWave checks releases across GitHub, GitLab, and other platforms.\nRun as an MCP server for AI agent integration or use CLI commands directly.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		logging.Setup(logLevel, logFmt)

		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return err
		}
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default ~/.config/releasewave/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFmt, "log-format", "text", "log format (text, json)")
}
