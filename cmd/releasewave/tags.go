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

var tagsCmd = &cobra.Command{
	Use:   "tags <owner/repo>",
	Short: "List tags for a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		owner, repo := parseOwnerRepo(args[0])
		platform, _ := cmd.Flags().GetString("platform")
		client := getProvider(platform)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		tags, err := client.ListTags(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("list tags: %w", err)
		}

		if len(tags) == 0 {
			fmt.Println("No tags found.")
			return nil
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(tags)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "TAG\tCOMMIT\n")
		fmt.Fprintf(w, "---\t------\n")
		for _, t := range tags {
			sha := t.Commit
			if len(sha) > 8 {
				sha = sha[:8]
			}
			fmt.Fprintf(w, "%s\t%s\n", t.Name, sha)
		}
		return w.Flush()
	},
}

func init() {
	tagsCmd.Flags().Bool("json", false, "output as JSON")
	tagsCmd.Flags().String("platform", "github", "git platform (github, gitlab)")
	rootCmd.AddCommand(tagsCmd)
}
