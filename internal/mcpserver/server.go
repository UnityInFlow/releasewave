package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/UnityInFlow/releasewave/internal/cache"
	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/middleware"
	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
	"github.com/UnityInFlow/releasewave/internal/ratelimit"
	"github.com/UnityInFlow/releasewave/internal/store"
)

// Server wraps the MCP server and its dependencies.
type Server struct {
	mcp               *server.MCPServer
	sse               *server.SSEServer
	httpServer        *http.Server
	config            *config.Config
	providers         map[string]provider.Provider
	store             *store.Store
	mu                sync.Mutex
	lastKnownVersions map[string]string
}

// marshalResult marshals v as indented JSON and wraps it in an MCP text result.
func marshalResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// New creates a new ReleaseWave MCP server with all tools registered.
func New(cfg *config.Config, version string) (*Server, error) {
	mcpServer := server.NewMCPServer(
		"releasewave",
		version,
		server.WithToolCapabilities(true),
	)

	ttl := cfg.Cache.TTLDuration()
	c := cache.New(ttl)

	providers := make(map[string]provider.Provider)

	var ghProvider provider.Provider = gh.New(cfg.Tokens.GitHub,
		gh.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitHub, 10)),
	)
	ghProvider = provider.NewCachedProvider(ghProvider, c)
	providers["github"] = ghProvider

	var glProvider provider.Provider = gl.New(cfg.Tokens.GitLab,
		gl.WithRateLimiter(ratelimit.New(cfg.RateLimit.GitLab, 10)),
	)
	glProvider = provider.NewCachedProvider(glProvider, c)
	providers["gitlab"] = glProvider

	s := &Server{
		mcp:       mcpServer,
		config:    cfg,
		providers: providers,
	}

	// Initialize SQLite store if configured.
	if cfg.Storage.Path != "" {
		st, err := store.New(cfg.Storage.Path)
		if err != nil {
			return nil, fmt.Errorf("open store: %w", err)
		}
		s.store = st
	}

	s.registerTools()
	s.registerExtendedTools()
	return s, nil
}

func (s *Server) registerTools() {
	s.mcp.AddTool(
		mcp.NewTool("list_releases",
			mcp.WithDescription("List releases for a repository. Returns release tags, names, dates, and release notes."),
			mcp.WithString("owner", mcp.Description("Repository owner (org or user)"), mcp.Required()),
			mcp.WithString("repo", mcp.Description("Repository name"), mcp.Required()),
			mcp.WithString("platform", mcp.Description("Git platform"), mcp.Enum("github", "gitlab"), mcp.DefaultString("github")),
		),
		s.handleListReleases,
	)

	s.mcp.AddTool(
		mcp.NewTool("get_latest_release",
			mcp.WithDescription("Get the latest release for a repository. Returns tag, name, date, URL, and release notes."),
			mcp.WithString("owner", mcp.Description("Repository owner (org or user)"), mcp.Required()),
			mcp.WithString("repo", mcp.Description("Repository name"), mcp.Required()),
			mcp.WithString("platform", mcp.Description("Git platform"), mcp.Enum("github", "gitlab"), mcp.DefaultString("github")),
		),
		s.handleGetLatestRelease,
	)

	s.mcp.AddTool(
		mcp.NewTool("list_tags",
			mcp.WithDescription("List git tags for a repository. Returns tag names and commit SHAs."),
			mcp.WithString("owner", mcp.Description("Repository owner (org or user)"), mcp.Required()),
			mcp.WithString("repo", mcp.Description("Repository name"), mcp.Required()),
			mcp.WithString("platform", mcp.Description("Git platform"), mcp.Enum("github", "gitlab"), mcp.DefaultString("github")),
		),
		s.handleListTags,
	)

	s.mcp.AddTool(
		mcp.NewTool("check_services",
			mcp.WithDescription("Check latest versions of all configured microservices. Returns service names, platforms, and their latest release tags."),
		),
		s.handleCheckServices,
	)

	s.mcp.AddTool(
		mcp.NewTool("find_outdated",
			mcp.WithDescription("Find services that may be outdated by comparing configured services. Returns a summary of each service's latest release."),
		),
		s.handleFindOutdated,
	)
}

func (s *Server) getProvider(platform string) (provider.Provider, error) {
	if platform == "" {
		platform = "github"
	}
	p, ok := s.providers[platform]
	if !ok {
		return nil, fmt.Errorf("unsupported platform: %s (supported: github, gitlab)", platform)
	}
	return p, nil
}

func (s *Server) handleListReleases(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner := request.GetString("owner", "")
	repo := request.GetString("repo", "")
	platform := request.GetString("platform", "github")

	slog.Info("tool.call", "tool", "list_releases", "owner", owner, "repo", repo, "platform", platform)

	if owner == "" || repo == "" {
		return mcp.NewToolResultError("owner and repo are required"), nil
	}

	p, err := s.getProvider(platform)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	releases, err := p.ListReleases(ctx, owner, repo)
	if err != nil {
		slog.Error("tool.error", "tool", "list_releases", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to list releases: %v", err)), nil
	}

	return marshalResult(releases)
}

func (s *Server) handleGetLatestRelease(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner := request.GetString("owner", "")
	repo := request.GetString("repo", "")
	platform := request.GetString("platform", "github")

	slog.Info("tool.call", "tool", "get_latest_release", "owner", owner, "repo", repo, "platform", platform)

	if owner == "" || repo == "" {
		return mcp.NewToolResultError("owner and repo are required"), nil
	}

	p, err := s.getProvider(platform)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	release, err := p.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		slog.Error("tool.error", "tool", "get_latest_release", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to get latest release: %v", err)), nil
	}

	return marshalResult(release)
}

func (s *Server) handleListTags(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner := request.GetString("owner", "")
	repo := request.GetString("repo", "")
	platform := request.GetString("platform", "github")

	slog.Info("tool.call", "tool", "list_tags", "owner", owner, "repo", repo, "platform", platform)

	if owner == "" || repo == "" {
		return mcp.NewToolResultError("owner and repo are required"), nil
	}

	p, err := s.getProvider(platform)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tags, err := p.ListTags(ctx, owner, repo)
	if err != nil {
		slog.Error("tool.error", "tool", "list_tags", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to list tags: %v", err)), nil
	}

	return marshalResult(tags)
}

// handleCheckServices checks all services concurrently.
func (s *Server) handleCheckServices(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slog.Info("tool.call", "tool", "check_services")

	if len(s.config.Services) == 0 {
		return mcp.NewToolResultText("No services configured. Add services to ~/.config/releasewave/config.yaml"), nil
	}

	type result struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
		Latest   string `json:"latest_release"`
		URL      string `json:"url"`
		Error    string `json:"error,omitempty"`
	}

	results := make([]result, len(s.config.Services))
	var wg sync.WaitGroup

	for i, svc := range s.config.Services {
		wg.Add(1)
		go func(idx int, svc config.ServiceConfig) {
			defer wg.Done()

			parsed, err := config.ParseRepo(svc.Repo)
			if err != nil {
				results[idx] = result{Name: svc.Name, Error: err.Error()}
				return
			}

			p, err := s.getProvider(parsed.Platform)
			if err != nil {
				results[idx] = result{Name: svc.Name, Platform: parsed.Platform, Error: err.Error()}
				return
			}

			release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
			if err != nil {
				results[idx] = result{Name: svc.Name, Platform: parsed.Platform, Error: err.Error()}
				return
			}

			results[idx] = result{
				Name:     svc.Name,
				Platform: parsed.Platform,
				Latest:   release.Tag,
				URL:      release.HTMLURL,
			}
		}(i, svc)
	}

	wg.Wait()

	return marshalResult(results)
}

func (s *Server) handleFindOutdated(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slog.Info("tool.call", "tool", "find_outdated")

	if len(s.config.Services) == 0 {
		return mcp.NewToolResultText("No services configured."), nil
	}

	type serviceStatus struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
		Latest   string `json:"latest_release"`
		URL      string `json:"url"`
		Error    string `json:"error,omitempty"`
	}

	statuses := make([]serviceStatus, len(s.config.Services))
	var wg sync.WaitGroup

	for i, svc := range s.config.Services {
		wg.Add(1)
		go func(idx int, svc config.ServiceConfig) {
			defer wg.Done()

			parsed, err := config.ParseRepo(svc.Repo)
			if err != nil {
				statuses[idx] = serviceStatus{Name: svc.Name, Error: err.Error()}
				return
			}

			p, err := s.getProvider(parsed.Platform)
			if err != nil {
				statuses[idx] = serviceStatus{Name: svc.Name, Platform: parsed.Platform, Error: err.Error()}
				return
			}

			release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
			if err != nil {
				statuses[idx] = serviceStatus{Name: svc.Name, Platform: parsed.Platform, Error: err.Error()}
				return
			}

			statuses[idx] = serviceStatus{
				Name:     svc.Name,
				Platform: parsed.Platform,
				Latest:   release.Tag,
				URL:      release.HTMLURL,
			}
		}(i, svc)
	}

	wg.Wait()

	return marshalResult(statuses)
}

// ServeStdio starts the MCP server using stdio transport (for Claude Code, Cursor, etc.).
func (s *Server) ServeStdio() error {
	slog.Info("server.start", "transport", "stdio")
	return server.ServeStdio(s.mcp)
}

// Start starts the MCP server using HTTP+SSE transport.
func (s *Server) Start(addr string) error {
	s.sse = server.NewSSEServer(s.mcp,
		server.WithBaseURL(fmt.Sprintf("http://localhost%s", addr)),
	)

	slog.Info("server.start", "transport", "sse", "addr", addr)
	return s.sse.Start(addr)
}

// StartWithHandlers starts the MCP server and also registers additional HTTP handlers.
func (s *Server) StartWithHandlers(addr string, extraHandlers map[string]http.Handler) error {
	s.sse = server.NewSSEServer(s.mcp,
		server.WithBaseURL(fmt.Sprintf("http://localhost%s", addr)),
	)

	// Build a mux that serves both the extra handlers and the SSE server.
	mux := http.NewServeMux()
	auth := middleware.APIKeyAuth(s.config.Server.APIKey)

	// Health endpoint for monitoring — unauthenticated.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Prometheus metrics endpoint — unauthenticated.
	mux.Handle("/metrics", promhttp.Handler())

	for pattern, handler := range extraHandlers {
		mux.Handle(pattern+"/", http.StripPrefix(pattern, auth(handler)))
		mux.Handle(pattern, auth(handler))
	}
	// Fall through to SSE server for MCP endpoints — authenticated.
	mux.Handle("/", auth(s.sse))

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      middleware.Metrics(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	slog.Info("server.start", "transport", "sse", "addr", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the HTTP and SSE servers.
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("http server: %w", err))
		}
	}
	if s.sse != nil {
		if err := s.sse.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("sse server: %w", err))
		}
	}
	if s.store != nil {
		if err := s.store.Close(); err != nil {
			errs = append(errs, fmt.Errorf("store: %w", err))
		}
	}
	return errors.Join(errs...)
}

// MCPServer returns the underlying MCP server (for custom transport wiring).
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcp
}

// Config returns the server's configuration.
func (s *Server) Config() *config.Config {
	return s.config
}

// Providers returns the server's provider map.
func (s *Server) Providers() map[string]provider.Provider {
	return s.providers
}

// Store returns the server's persistence store (may be nil).
func (s *Server) Store() *store.Store {
	return s.store
}

// Info returns a summary for display.
func (s *Server) Info() string {
	var b strings.Builder
	b.WriteString("ReleaseWave MCP Server\n")
	b.WriteString("Tools: list_releases, get_latest_release, list_tags, check_services, find_outdated,\n")
	b.WriteString("       list_image_tags, compare_image_tags, list_k8s_deployments,\n")
	b.WriteString("       compare_release_vs_deployed, changelog_between_versions,\n")
	b.WriteString("       security_advisories, release_timeline, get_repo_file,\n")
	b.WriteString("       dependency_matrix, upgrade_plan, watch_releases,\n")
	b.WriteString("       service_graph, discover_services,\n")
	b.WriteString("       release_diff, release_history\n")
	b.WriteString("Providers: github, gitlab\n")
	b.WriteString(fmt.Sprintf("Services configured: %d\n", len(s.config.Services)))
	return b.String()
}
