// Package web provides the web dashboard for ReleaseWave.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
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

// Handler returns an http.Handler that serves the web dashboard with htmx partials.
func Handler(cfg *config.Config, providers map[string]provider.Provider) (http.Handler, error) {
	tmpl, err := template.ParseFS(templateFS, "dashboard.html")
	if err != nil {
		return nil, fmt.Errorf("web: failed to parse embedded template: %w", err)
	}

	h := &dashboardHandler{cfg: cfg, providers: providers, tmpl: tmpl}

	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard", h.fullPage)
	mux.HandleFunc("/dashboard/partials/stats", h.partialStats)
	mux.HandleFunc("/dashboard/partials/services", h.partialServices)
	mux.HandleFunc("POST /dashboard/services", h.addService)
	mux.HandleFunc("DELETE /dashboard/services/{name}", h.deleteService)

	return mux, nil
}

type dashboardHandler struct {
	cfg       *config.Config
	providers map[string]provider.Provider
	tmpl      *template.Template
	mu        sync.RWMutex
}

func (h *dashboardHandler) fullPage(w http.ResponseWriter, r *http.Request) {
	data := h.getData(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.Execute(w, data); err != nil {
		slog.Error("web.render", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *dashboardHandler) partialStats(w http.ResponseWriter, r *http.Request) {
	data := h.getData(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "stats", data); err != nil {
		slog.Error("web.render.stats", "error", err)
	}
}

func (h *dashboardHandler) partialServices(w http.ResponseWriter, r *http.Request) {
	data := h.getData(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "services", data); err != nil {
		slog.Error("web.render.services", "error", err)
	}
}

func (h *dashboardHandler) addService(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	repo := r.FormValue("repo")
	registry := r.FormValue("registry")

	if name == "" || repo == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "name and repo are required"}); err != nil {
			slog.Error("web.write", "error", err)
		}
		return
	}

	parts := strings.Split(repo, "/")
	if len(parts) < 3 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "repo must be host/owner/repo format"}); err != nil {
			slog.Error("web.write", "error", err)
		}
		return
	}

	h.mu.Lock()
	for _, svc := range h.cfg.Services {
		if svc.Name == name {
			h.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "service already exists"}); err != nil {
				slog.Error("web.write", "error", err)
			}
			return
		}
	}
	h.cfg.Services = append(h.cfg.Services, config.ServiceConfig{
		Name:     name,
		Repo:     repo,
		Registry: registry,
	})
	h.mu.Unlock()

	slog.Info("web.add_service", "name", name, "repo", repo)

	// Return updated services partial.
	data := h.getData(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "services", data); err != nil {
		slog.Error("web.render.services", "error", err)
	}
}

func (h *dashboardHandler) deleteService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	h.mu.Lock()
	found := false
	services := make([]config.ServiceConfig, 0, len(h.cfg.Services))
	for _, svc := range h.cfg.Services {
		if svc.Name == name {
			found = true
			continue
		}
		services = append(services, svc)
	}
	if found {
		h.cfg.Services = services
	}
	h.mu.Unlock()

	if !found {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "service not found"}); err != nil {
			slog.Error("web.write", "error", err)
		}
		return
	}

	slog.Info("web.delete_service", "name", name)

	// Return updated services partial.
	data := h.getData(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "services", data); err != nil {
		slog.Error("web.render.services", "error", err)
	}
}

func (h *dashboardHandler) getData(ctx context.Context) dashboardData {
	return fetchDashboardData(ctx, h.cfg, h.providers)
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
