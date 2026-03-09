package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		cfgPath, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}

		if _, err := os.Stat(cfgPath); err == nil && !force {
			return fmt.Errorf("config already exists at %s (use --force to overwrite)", cfgPath)
		}

		dir := filepath.Dir(cfgPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}

		if err := os.WriteFile(cfgPath, []byte(config.ExampleConfig), 0o644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}

		fmt.Printf("Config created at %s\n", cfgPath)
		fmt.Println("Edit it to add your services and API tokens.")
		return nil
	},
}

func init() {
	initCmd.Flags().Bool("force", false, "overwrite existing config")
	rootCmd.AddCommand(initCmd)
}
