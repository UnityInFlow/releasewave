// Package api provides REST API endpoints for ReleaseWave.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/provider"
	"github.com/UnityInFlow/releasewave/internal/store"
)

// Handler returns an http.Handler with REST API endpoints.
func Handler(cfg *config.Config, providers map[string]provider.Provider, st *store.Store) http.Handler {
	mux := http.NewServeMux()
	a := &apiHandler{cfg: cfg, providers: providers, store: st}

	mux.HandleFunc("GET /v1/services", a.listServices)
	mux.HandleFunc("GET /v1/services/{name}/releases", a.getServiceReleases)
	mux.HandleFunc("GET /v1/timeline", a.getTimeline)
	mux.HandleFunc("POST /v1/services", a.addService)
	mux.HandleFunc("DELETE /v1/services/{name}", a.deleteService)

	return mux
}

type apiHandler struct {
	cfg       *config.Config
	providers map[string]provider.Provider
	store     *store.Store
	mu        sync.RWMutex
}

func (a *apiHandler) listServices(w http.ResponseWriter, r *http.Request) {
	type svcInfo struct {
		Name     string `json:"name"`
		Repo     string `json:"repo"`
		Registry string `json:"registry,omitempty"`
		Latest   string `json:"latest_release,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	a.mu.RLock()
	svcs := make([]config.ServiceConfig, len(a.cfg.Services))
	copy(svcs, a.cfg.Services)
	a.mu.RUnlock()

	// Fetch latest releases concurrently.
	services := make([]svcInfo, len(svcs))
	var wg sync.WaitGroup
	ctx := r.Context()

	for i, svc := range svcs {
		wg.Add(1)
		go func(idx int, svc config.ServiceConfig) {
			defer wg.Done()
			info := svcInfo{Name: svc.Name, Repo: svc.Repo, Registry: svc.Registry}

			parsed, err := config.ParseRepo(svc.Repo)
			if err != nil {
				info.Error = err.Error()
				services[idx] = info
				return
			}

			p, ok := a.providers[parsed.Platform]
			if !ok {
				info.Error = "unsupported platform"
				services[idx] = info
				return
			}

			release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
			if err != nil {
				info.Error = err.Error()
			} else {
				info.Latest = release.Tag
			}
			services[idx] = info
		}(i, svc)
	}
	wg.Wait()

	writeJSON(w, http.StatusOK, map[string]any{
		"total":    len(services),
		"services": services,
	})
}

func (a *apiHandler) getServiceReleases(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if a.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "no storage configured",
		})
		return
	}

	history, err := a.store.GetHistory(name, 100)
	if err != nil {
		slog.Error("api.get_releases", "service", name, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to query history",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service":  name,
		"total":    len(history),
		"releases": history,
	})
}

func (a *apiHandler) getTimeline(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "no storage configured",
		})
		return
	}

	// Aggregate releases across all services, newest first.
	type entry struct {
		Service     string    `json:"service"`
		Tag         string    `json:"tag"`
		Platform    string    `json:"platform"`
		URL         string    `json:"url"`
		PublishedAt time.Time `json:"published_at"`
	}

	a.mu.RLock()
	svcs := make([]config.ServiceConfig, len(a.cfg.Services))
	copy(svcs, a.cfg.Services)
	a.mu.RUnlock()

	var timeline []entry
	for _, svc := range svcs {
		history, err := a.store.GetHistory(svc.Name, 20)
		if err != nil {
			continue
		}
		for _, r := range history {
			timeline = append(timeline, entry{
				Service:     r.Service,
				Tag:         r.Tag,
				Platform:    r.Platform,
				URL:         r.URL,
				PublishedAt: r.PublishedAt,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":    len(timeline),
		"timeline": timeline,
	})
}

func (a *apiHandler) addService(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	var req struct {
		Name     string `json:"name"`
		Repo     string `json:"repo"`
		Registry string `json:"registry"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name == "" || req.Repo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and repo are required"})
		return
	}

	parts := strings.Split(req.Repo, "/")
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repo must be host/owner/repo format"})
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check for duplicates under lock.
	for _, svc := range a.cfg.Services {
		if svc.Name == req.Name {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "service already exists"})
			return
		}
	}

	a.cfg.Services = append(a.cfg.Services, config.ServiceConfig{
		Name:     req.Name,
		Repo:     req.Repo,
		Registry: req.Registry,
	})

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "name": req.Name})
}

func (a *apiHandler) deleteService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	a.mu.Lock()
	defer a.mu.Unlock()

	found := false
	services := make([]config.ServiceConfig, 0, len(a.cfg.Services))
	for _, svc := range a.cfg.Services {
		if svc.Name == name {
			found = true
			continue
		}
		services = append(services, svc)
	}

	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}

	a.cfg.Services = services
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api.write", "error", err)
	}
}
