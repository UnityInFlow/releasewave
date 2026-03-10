package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var latestCmd = &cobra.Command{
	Use:   "latest <owner/repo>",
	Short: "Show the latest release for a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		owner, repo := parseOwnerRepo(args[0])
		platform, _ := cmd.Flags().GetString("platform")
		client := getProvider(platform)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		release, err := client.GetLatestRelease(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("get latest release: %w", err)
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(release)
		}

		fmt.Printf("Repository:  %s/%s\n", owner, repo)
		fmt.Printf("Latest:      %s\n", release.Tag)
		fmt.Printf("Name:        %s\n", release.Name)
		fmt.Printf("Published:   %s\n", release.PublishedAt.Format("2006-01-02 15:04"))
		fmt.Printf("URL:         %s\n", release.HTMLURL)
		if release.Prerelease {
			fmt.Println("Pre-release: yes")
		}
		if release.Body != "" {
			body := release.Body
			if len(body) > 500 {
				body = body[:497] + "..."
			}
			fmt.Printf("\n--- Release Notes ---\n%s\n", body)
		}
		return nil
	},
}

func init() {
	latestCmd.Flags().Bool("json", false, "output as JSON")
	latestCmd.Flags().String("platform", "github", "git platform (github, gitlab)")
	rootCmd.AddCommand(latestCmd)
}
