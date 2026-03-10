package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var releasesCmd = &cobra.Command{
	Use:   "releases <owner/repo>",
	Short: "List releases for a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		owner, repo := parseOwnerRepo(args[0])
		platform, _ := cmd.Flags().GetString("platform")
		client := getProvider(platform)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		releases, err := client.ListReleases(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("list releases: %w", err)
		}

		if len(releases) == 0 {
			fmt.Println("No releases found.")
			return nil
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(releases)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "TAG\tNAME\tDATE\tPRE-RELEASE\n")
		fmt.Fprintf(w, "---\t----\t----\t-----------\n")
		for _, r := range releases {
			pre := ""
			if r.Prerelease {
				pre = "yes"
			}
			date := r.PublishedAt.Format("2006-01-02")
			name := r.Name
			if len(name) > 50 {
				name = name[:47] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Tag, name, date, pre)
		}
		return w.Flush()
	},
}

func init() {
	releasesCmd.Flags().Bool("json", false, "output as JSON")
	releasesCmd.Flags().String("platform", "github", "git platform (github, gitlab)")
	rootCmd.AddCommand(releasesCmd)
}
