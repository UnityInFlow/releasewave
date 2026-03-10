// Package web provides a minimal web dashboard for ReleaseWave.
package web

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/provider"
)

// serviceStatus holds the display data for a single service row.
type serviceStatus struct {
	Name       string
	Platform   string
	LatestTag  string
	ReleasedAt string
	URL        string
	Error      string
}

// dashboardData is the template data for the dashboard page.
type dashboardData struct {
	Services      []serviceStatus
	TotalServices int
	HealthyCount  int
	ErrorCount    int
	UpdatedAt     string
}

// Handler returns an http.Handler that serves the web dashboard.
func Handler(cfg *config.Config, providers map[string]provider.Provider) http.Handler {
	tmpl, err := template.ParseFS(templateFS, "dashboard.html")
	if err != nil {
		// This should not happen since the template is embedded at compile time.
		panic("web: failed to parse embedded template: " + err.Error())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		data := fetchDashboardData(r.Context(), cfg, providers)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			slog.Error("web.render", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	return mux
}

// fetchDashboardData queries all configured services concurrently and returns dashboard data.
func fetchDashboardData(ctx context.Context, cfg *config.Config, providers map[string]provider.Provider) dashboardData {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	statuses := make([]serviceStatus, len(cfg.Services))
	var wg sync.WaitGroup

	for i, svc := range cfg.Services {
		wg.Add(1)
		go func(idx int, svc config.ServiceConfig) {
			defer wg.Done()

			parsed, err := config.ParseRepo(svc.Repo)
			if err != nil {
				statuses[idx] = serviceStatus{Name: svc.Name, Error: err.Error()}
				return
			}

			p, ok := providers[parsed.Platform]
			if !ok {
				statuses[idx] = serviceStatus{
					Name:     svc.Name,
					Platform: parsed.Platform,
					Error:    "unsupported platform",
				}
				return
			}

			release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
			if err != nil {
				statuses[idx] = serviceStatus{
					Name:     svc.Name,
					Platform: parsed.Platform,
					Error:    err.Error(),
				}
				return
			}

			releasedAt := ""
			if !release.PublishedAt.IsZero() {
				releasedAt = release.PublishedAt.Format("2006-01-02")
			}

			statuses[idx] = serviceStatus{
				Name:       svc.Name,
				Platform:   parsed.Platform,
				LatestTag:  release.Tag,
				ReleasedAt: releasedAt,
				URL:        release.HTMLURL,
			}
		}(i, svc)
	}

	wg.Wait()

	healthy := 0
	errors := 0
	for _, s := range statuses {
		if s.Error != "" {
			errors++
		} else {
			healthy++
		}
	}

	return dashboardData{
		Services:      statuses,
		TotalServices: len(cfg.Services),
		HealthyCount:  healthy,
		ErrorCount:    errors,
		UpdatedAt:     time.Now().Format("2006-01-02 15:04:05"),
	}
}
