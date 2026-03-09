// Package mcpserver creates and configures the ReleaseWave MCP server.
//
// ============================================================================
// GO LEARNING: Building an HTTP Server
// ============================================================================
//
// This file ties everything together:
//   - Creates an MCP server instance
//   - Registers tools (functions the AI agent can call)
//   - Starts the HTTP+SSE transport
//
// Key Go concepts in this file:
//   1. Function closures — the tool handlers capture provider variables
//   2. Type assertions — extracting typed values from interface{} (any)
//   3. JSON marshaling — converting Go structs to JSON for tool responses
//   4. Goroutines (not yet, but the server handles them internally)
//
// MCP (Model Context Protocol) concepts:
//   - Tools are functions that an AI agent can call
//   - Each tool has a name, description, and input schema (what params it accepts)
//   - The handler receives the params and returns a text/JSON result
//   - Transport: HTTP+SSE allows real-time streaming communication
//
// ============================================================================

package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/provider"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
	gl "github.com/UnityInFlow/releasewave/internal/provider/gitlab"
)

// Server wraps the MCP server and its dependencies.
//
// GO LEARNING: Struct Composition
//   This struct holds all the "wiring" — the MCP server, config, and providers.
//   It's the central point where we connect everything together.
type Server struct {
	mcp       *server.MCPServer
	sse       *server.SSEServer
	config    *config.Config
	providers map[string]provider.Provider // "github" → GitHub client, "gitlab" → GitLab client
}

// New creates a new ReleaseWave MCP server with all tools registered.
//
// GO LEARNING: Dependency Injection (the Go way)
//   We pass the config in and create providers from it.
//   This makes testing easier — you could pass a test config.
//   Go doesn't use DI frameworks — just pass dependencies as arguments.
func New(cfg *config.Config) *Server {
	// Create the MCP server instance
	//
	// GO LEARNING: Variadic Options Pattern
	//   server.NewMCPServer takes optional arguments (...ServerOption).
	//   This is the "functional options" pattern — very common in Go.
	//   Each WithXxx() returns an option that configures one aspect.
	mcpServer := server.NewMCPServer(
		"releasewave",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	// Create providers based on available tokens
	//
	// GO LEARNING: Maps
	//   map[string]provider.Provider is a hash map.
	//   The key is a string ("github", "gitlab"), the value is any Provider.
	//   This is polymorphism in action — different implementations, same interface.
	providers := make(map[string]provider.Provider)
	providers["github"] = gh.New(cfg.Tokens.GitHub)
	providers["gitlab"] = gl.New(cfg.Tokens.GitLab)

	s := &Server{
		mcp:       mcpServer,
		config:    cfg,
		providers: providers,
	}

	// Register all tools
	s.registerTools()

	return s
}

// registerTools adds all MCP tools to the server.
//
// GO LEARNING: Method Organization
//   We keep tool registration in a separate method for readability.
//   Each tool is defined with:
//     1. mcp.NewTool — name + schema (what the tool accepts)
//     2. A handler function — what happens when the tool is called
func (s *Server) registerTools() {
	// ── Tool 1: list_releases ──────────────────────────────────────────
	//
	// GO LEARNING: mcp.NewTool with Options
	//   mcp.WithDescription sets the tool's description (shown to the AI).
	//   mcp.WithString defines a string parameter.
	//   mcp.Required() makes the parameter mandatory.
	//   mcp.Enum() restricts to specific values.
	s.mcp.AddTool(
		mcp.NewTool("list_releases",
			mcp.WithDescription("List releases for a repository. Returns release tags, names, dates, and release notes."),
			mcp.WithString("owner",
				mcp.Description("Repository owner (org or user)"),
				mcp.Required(),
			),
			mcp.WithString("repo",
				mcp.Description("Repository name"),
				mcp.Required(),
			),
			mcp.WithString("platform",
				mcp.Description("Git platform"),
				mcp.Enum("github", "gitlab"),
				mcp.DefaultString("github"),
			),
		),
		s.handleListReleases,
	)

	// ── Tool 2: get_latest_release ─────────────────────────────────────
	s.mcp.AddTool(
		mcp.NewTool("get_latest_release",
			mcp.WithDescription("Get the latest release for a repository. Returns tag, name, date, URL, and release notes."),
			mcp.WithString("owner",
				mcp.Description("Repository owner (org or user)"),
				mcp.Required(),
			),
			mcp.WithString("repo",
				mcp.Description("Repository name"),
				mcp.Required(),
			),
			mcp.WithString("platform",
				mcp.Description("Git platform"),
				mcp.Enum("github", "gitlab"),
				mcp.DefaultString("github"),
			),
		),
		s.handleGetLatestRelease,
	)

	// ── Tool 3: list_tags ──────────────────────────────────────────────
	s.mcp.AddTool(
		mcp.NewTool("list_tags",
			mcp.WithDescription("List git tags for a repository. Returns tag names and commit SHAs."),
			mcp.WithString("owner",
				mcp.Description("Repository owner (org or user)"),
				mcp.Required(),
			),
			mcp.WithString("repo",
				mcp.Description("Repository name"),
				mcp.Required(),
			),
			mcp.WithString("platform",
				mcp.Description("Git platform"),
				mcp.Enum("github", "gitlab"),
				mcp.DefaultString("github"),
			),
		),
		s.handleListTags,
	)

	// ── Tool 4: check_services ─────────────────────────────────────────
	s.mcp.AddTool(
		mcp.NewTool("check_services",
			mcp.WithDescription("Check latest versions of all configured microservices. Returns service names, platforms, and their latest release tags."),
		),
		s.handleCheckServices,
	)

	// ── Tool 5: find_outdated ──────────────────────────────────────────
	s.mcp.AddTool(
		mcp.NewTool("find_outdated",
			mcp.WithDescription("Find services that may be outdated by comparing configured services. Returns a summary of each service's latest release."),
		),
		s.handleFindOutdated,
	)
}

// ============================================================================
// GO LEARNING: Tool Handler Functions
// ============================================================================
//
// Each handler has the signature:
//   func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
//
// The request contains the parameters the AI agent passed.
// We extract them with request.GetString("param_name", "default").
//
// We return either:
//   - mcp.NewToolResultText("some text") for success
//   - nil, fmt.Errorf("...") for errors
//
// ============================================================================

// getProvider returns the right provider based on the platform parameter.
//
// GO LEARNING: Map Lookup with "comma ok" Pattern
//   value, ok := myMap[key]
//   If the key exists: ok=true, value=the value
//   If not: ok=false, value=zero value
//   This avoids panics on missing keys — always use this pattern!
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
	// GO LEARNING: Extracting Parameters
	//   request.GetString("name", "default") returns the param value,
	//   or the default if not provided. No need for manual type assertions.
	owner := request.GetString("owner", "")
	repo := request.GetString("repo", "")
	platform := request.GetString("platform", "github")

	if owner == "" || repo == "" {
		return mcp.NewToolResultError("owner and repo are required"), nil
	}

	p, err := s.getProvider(platform)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	releases, err := p.ListReleases(ctx, owner, repo)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list releases: %v", err)), nil
	}

	// GO LEARNING: json.MarshalIndent for Pretty JSON
	//   json.Marshal produces compact JSON.
	//   json.MarshalIndent adds indentation — better for AI agents to read.
	//   Args: (value, prefix, indent)
	data, err := json.MarshalIndent(releases, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetLatestRelease(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner := request.GetString("owner", "")
	repo := request.GetString("repo", "")
	platform := request.GetString("platform", "github")

	if owner == "" || repo == "" {
		return mcp.NewToolResultError("owner and repo are required"), nil
	}

	p, err := s.getProvider(platform)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	release, err := p.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get latest release: %v", err)), nil
	}

	data, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleListTags(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner := request.GetString("owner", "")
	repo := request.GetString("repo", "")
	platform := request.GetString("platform", "github")

	if owner == "" || repo == "" {
		return mcp.NewToolResultError("owner and repo are required"), nil
	}

	p, err := s.getProvider(platform)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tags, err := p.ListTags(ctx, owner, repo)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list tags: %v", err)), nil
	}

	data, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleCheckServices(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if len(s.config.Services) == 0 {
		return mcp.NewToolResultText("No services configured. Add services to ~/.config/releasewave/config.yaml"), nil
	}

	// GO LEARNING: strings.Builder
	//   Efficiently builds strings by appending. Much better than string
	//   concatenation with + (which creates a new string each time).
	//   Similar to StringBuilder in Java/C#.
	var result strings.Builder
	result.WriteString("Service Status:\n\n")

	for _, svc := range s.config.Services {
		parsed, err := config.ParseRepo(svc.Repo)
		if err != nil {
			result.WriteString(fmt.Sprintf("- %s: error parsing repo — %v\n", svc.Name, err))
			continue
		}

		p, err := s.getProvider(parsed.Platform)
		if err != nil {
			result.WriteString(fmt.Sprintf("- %s: %v\n", svc.Name, err))
			continue
		}

		release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
		if err != nil {
			result.WriteString(fmt.Sprintf("- %s (%s): error — %v\n", svc.Name, parsed.Platform, err))
			continue
		}

		result.WriteString(fmt.Sprintf("- %s (%s): %s — %s [%s]\n",
			svc.Name, parsed.Platform, release.Tag, release.Name, release.HTMLURL))
	}

	return mcp.NewToolResultText(result.String()), nil
}

func (s *Server) handleFindOutdated(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if len(s.config.Services) == 0 {
		return mcp.NewToolResultText("No services configured."), nil
	}

	// GO LEARNING: Struct for collecting results
	//   We build a slice of result structs, then marshal to JSON.
	//   This is cleaner than building strings for structured data.
	type serviceStatus struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
		Latest   string `json:"latest_release"`
		URL      string `json:"url"`
		Error    string `json:"error,omitempty"` // omitempty: skip this field if empty
	}

	var statuses []serviceStatus

	for _, svc := range s.config.Services {
		parsed, err := config.ParseRepo(svc.Repo)
		if err != nil {
			statuses = append(statuses, serviceStatus{
				Name:  svc.Name,
				Error: err.Error(),
			})
			continue
		}

		p, err := s.getProvider(parsed.Platform)
		if err != nil {
			statuses = append(statuses, serviceStatus{
				Name:     svc.Name,
				Platform: parsed.Platform,
				Error:    err.Error(),
			})
			continue
		}

		release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
		if err != nil {
			statuses = append(statuses, serviceStatus{
				Name:     svc.Name,
				Platform: parsed.Platform,
				Error:    err.Error(),
			})
			continue
		}

		statuses = append(statuses, serviceStatus{
			Name:     svc.Name,
			Platform: parsed.Platform,
			Latest:   release.Tag,
			URL:      release.HTMLURL,
		})
	}

	data, _ := json.MarshalIndent(statuses, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// Start starts the MCP server on the given address.
//
// GO LEARNING: Blocking Calls
//   sseServer.Start() blocks (doesn't return until the server stops).
//   That's normal for servers — they run until interrupted (Ctrl+C).
//   In production, you'd handle graceful shutdown with signal handling.
func (s *Server) Start(addr string) error {
	s.sse = server.NewSSEServer(s.mcp,
		server.WithBaseURL(fmt.Sprintf("http://localhost%s", addr)),
	)

	fmt.Printf("ReleaseWave MCP server starting on %s\n", addr)
	fmt.Printf("SSE endpoint: http://localhost%s/sse\n", addr)
	fmt.Printf("Tools: list_releases, get_latest_release, list_tags, check_services, find_outdated\n")

	return s.sse.Start(addr)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.sse != nil {
		return s.sse.Shutdown(ctx)
	}
	return nil
}
