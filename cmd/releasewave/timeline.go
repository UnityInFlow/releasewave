package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
)

var timelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Show a timeline of recent releases across all configured services",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Services) == 0 {
			fmt.Println("No services configured.")
			return nil
		}

		days, _ := cmd.Flags().GetInt("days")
		cutoff := time.Now().AddDate(0, 0, -days)

		providers := map[string]provider.Provider{
			"github": gh.New(cfg.Tokens.GitHub, gh.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitHub, 10))),
			"gitlab": gl.New(cfg.Tokens.GitLab, gl.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitLab, 10))),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		type entry struct {
			Date    time.Time
			Service string
			Tag     string
			Name    string
			URL     string
		}

		var mu sync.Mutex
		var entries []entry
		var wg sync.WaitGroup

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

				releases, err := p.ListReleases(ctx, parsed.Owner, parsed.RepoName)
				if err != nil {
					return
				}

				for _, r := range releases {
					if r.PublishedAt.Before(cutoff) {
						break
					}
					mu.Lock()
					entries = append(entries, entry{
						Date:    r.PublishedAt,
						Service: svc.Name,
						Tag:     r.Tag,
						Name:    r.Name,
						URL:     r.HTMLURL,
					})
					mu.Unlock()
				}
			}(svc)
		}

		wg.Wait()

		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Date.After(entries[j].Date)
		})

		if len(entries) == 0 {
			fmt.Printf("No releases in the last %d days.\n", days)
			return nil
		}

		jsonFlag, _ := cmd.Flags().GetBool("json")
		if jsonFlag {
			type jsonEntry struct {
				Date    string `json:"date"`
				Service string `json:"service"`
				Tag     string `json:"tag"`
				Name    string `json:"name"`
				URL     string `json:"url"`
			}
			out := make([]jsonEntry, len(entries))
			for i, e := range entries {
				out[i] = jsonEntry{
					Date:    e.Date.Format("2006-01-02 15:04"),
					Service: e.Service,
					Tag:     e.Tag,
					Name:    e.Name,
					URL:     e.URL,
				}
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		fmt.Printf("Releases in the last %d days (%d total):\n\n", days, len(entries))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "DATE\tSERVICE\tTAG\tNAME\n")
		fmt.Fprintf(w, "----\t-------\t---\t----\n")
		for _, e := range entries {
			name := e.Name
			if len(name) > 40 {
				name = name[:37] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Date.Format("2006-01-02"), e.Service, e.Tag, name)
		}
		return w.Flush()
	},
}

func init() {
	timelineCmd.Flags().Int("days", 30, "number of days to look back")
	timelineCmd.Flags().Bool("json", false, "output as JSON")
	rootCmd.AddCommand(timelineCmd)
}
