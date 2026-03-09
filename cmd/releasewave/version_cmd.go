package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			info := map[string]string{
				"version": version,
				"commit":  commit,
				"date":    date,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(info)
			return
		}
		fmt.Printf("releasewave %s (commit: %s, built: %s)\n", version, commit, date)
	},
}

func init() {
	versionCmd.Flags().Bool("json", false, "output as JSON")
	rootCmd.AddCommand(versionCmd)
}
