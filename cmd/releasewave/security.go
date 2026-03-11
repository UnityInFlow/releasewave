package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/security"
)

var securityCmd = &cobra.Command{
	Use:   "security <ecosystem> <package> <version>",
	Short: "Check for known vulnerabilities (CVEs) in a package version",
	Long:  "Query the OSV.dev database for known vulnerabilities.\nEcosystems: Go, npm, PyPI, Maven, crates.io, NuGet, etc.",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ecosystem, pkg, ver := args[0], args[1], args[2]

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := security.New()
		vulns, err := client.QueryByPackage(ctx, ecosystem, pkg, ver)
		if err != nil {
			return fmt.Errorf("vulnerability check: %w", err)
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			out := map[string]any{
				"ecosystem":       ecosystem,
				"package":         pkg,
				"version":         ver,
				"total":           len(vulns),
				"vulnerabilities": vulns,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		if len(vulns) == 0 {
			fmt.Printf("No known vulnerabilities for %s %s@%s\n", ecosystem, pkg, ver)
			return nil
		}

		fmt.Printf("Found %d vulnerabilities for %s %s@%s:\n\n", len(vulns), ecosystem, pkg, ver)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "ID\tSEVERITY\tSUMMARY\n")
		fmt.Fprintf(w, "--\t--------\t-------\n")
		for _, v := range vulns {
			summary := v.Summary
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
			severity := v.Severity
			if severity == "" {
				severity = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", v.ID, severity, summary)
		}
		return w.Flush()
	},
}

func init() {
	securityCmd.Flags().Bool("json", false, "output as JSON")
	rootCmd.AddCommand(securityCmd)
}
