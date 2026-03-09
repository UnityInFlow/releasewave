package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/k8s"
	"github.com/UnityInFlow/releasewave/internal/registry"
	"github.com/UnityInFlow/releasewave/internal/security"
)

// registerExtendedTools adds Phase 5-7 tools to the server.
func (s *Server) registerExtendedTools() {
	// ── Phase 5: Container Registry Tools ────────────────────────────

	s.mcp.AddTool(
		mcp.NewTool("list_image_tags",
			mcp.WithDescription("List container image tags from any OCI-compatible registry (Docker Hub, GHCR, GitLab Registry, ECR, etc.)."),
			mcp.WithString("image", mcp.Description("Full image reference, e.g. ghcr.io/org/app, docker.io/library/nginx"), mcp.Required()),
		),
		s.handleListImageTags,
	)

	s.mcp.AddTool(
		mcp.NewTool("compare_image_tags",
			mcp.WithDescription("Check if two image tags point to the same digest (same build)."),
			mcp.WithString("image", mcp.Description("Image reference"), mcp.Required()),
			mcp.WithString("tag1", mcp.Description("First tag to compare"), mcp.Required()),
			mcp.WithString("tag2", mcp.Description("Second tag to compare"), mcp.Required()),
		),
		s.handleCompareImageTags,
	)

	// ── Phase 6: Kubernetes Tools ────────────────────────────────────

	s.mcp.AddTool(
		mcp.NewTool("list_k8s_deployments",
			mcp.WithDescription("List deployments and statefulsets running in a Kubernetes cluster with their image versions."),
			mcp.WithString("namespace", mcp.Description("Kubernetes namespace (empty for all namespaces)"), mcp.DefaultString("")),
			mcp.WithString("kubeconfig", mcp.Description("Path to kubeconfig file (default: ~/.kube/config)"), mcp.DefaultString("")),
			mcp.WithString("context", mcp.Description("Kubernetes context to use (default: current context)"), mcp.DefaultString("")),
		),
		s.handleListK8sDeployments,
	)

	s.mcp.AddTool(
		mcp.NewTool("compare_release_vs_deployed",
			mcp.WithDescription("Compare the latest release version against what's deployed in Kubernetes for configured services."),
			mcp.WithString("namespace", mcp.Description("Kubernetes namespace"), mcp.DefaultString("default")),
			mcp.WithString("kubeconfig", mcp.Description("Path to kubeconfig"), mcp.DefaultString("")),
			mcp.WithString("context", mcp.Description("Kubernetes context"), mcp.DefaultString("")),
		),
		s.handleCompareReleaseVsDeployed,
	)

	// ── Phase 7: Extended Analysis Tools ─────────────────────────────

	s.mcp.AddTool(
		mcp.NewTool("changelog_between_versions",
			mcp.WithDescription("Get aggregated release notes between two versions of a repository."),
			mcp.WithString("owner", mcp.Description("Repository owner"), mcp.Required()),
			mcp.WithString("repo", mcp.Description("Repository name"), mcp.Required()),
			mcp.WithString("from", mcp.Description("Starting version tag (older)"), mcp.Required()),
			mcp.WithString("to", mcp.Description("Ending version tag (newer)"), mcp.Required()),
			mcp.WithString("platform", mcp.Description("Git platform"), mcp.Enum("github", "gitlab"), mcp.DefaultString("github")),
		),
		s.handleChangelogBetweenVersions,
	)

	s.mcp.AddTool(
		mcp.NewTool("security_advisories",
			mcp.WithDescription("Check for known security vulnerabilities (CVEs) affecting a package version using the OSV.dev database."),
			mcp.WithString("ecosystem", mcp.Description("Package ecosystem: Go, npm, PyPI, Maven, crates.io, NuGet"), mcp.Required()),
			mcp.WithString("package", mcp.Description("Package name (e.g. golang.org/x/net, express, django)"), mcp.Required()),
			mcp.WithString("version", mcp.Description("Version to check"), mcp.Required()),
		),
		s.handleSecurityAdvisories,
	)

	s.mcp.AddTool(
		mcp.NewTool("release_timeline",
			mcp.WithDescription("Show a timeline of recent releases across all configured services, sorted by date."),
			mcp.WithNumber("days", mcp.Description("Number of days to look back")),
		),
		s.handleReleaseTimeline,
	)
}

// ── Phase 5: Container Registry Handlers ─────────────────────────────

func (s *Server) handleListImageTags(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	image := request.GetString("image", "")
	slog.Info("tool.call", "tool", "list_image_tags", "image", image)

	if image == "" {
		return mcp.NewToolResultError("image is required"), nil
	}

	client := registry.New()
	info, err := client.ListTags(ctx, image)
	if err != nil {
		slog.Error("tool.error", "tool", "list_image_tags", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to list image tags: %v", err)), nil
	}

	// Limit to first 50 tags for readability
	if len(info.Tags) > 50 {
		info.Tags = info.Tags[:50]
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleCompareImageTags(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	image := request.GetString("image", "")
	tag1 := request.GetString("tag1", "")
	tag2 := request.GetString("tag2", "")
	slog.Info("tool.call", "tool", "compare_image_tags", "image", image, "tag1", tag1, "tag2", tag2)

	if image == "" || tag1 == "" || tag2 == "" {
		return mcp.NewToolResultError("image, tag1, and tag2 are required"), nil
	}

	client := registry.New()
	same, err := client.CompareTag(ctx, image, tag1, tag2)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("comparison failed: %v", err)), nil
	}

	result := map[string]any{
		"image":      image,
		"tag1":       tag1,
		"tag2":       tag2,
		"same_image": same,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// ── Phase 6: Kubernetes Handlers ─────────────────────────────────────

func (s *Server) handleListK8sDeployments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := request.GetString("namespace", "")
	kubeconfig := request.GetString("kubeconfig", "")
	kctx := request.GetString("context", "")
	slog.Info("tool.call", "tool", "list_k8s_deployments", "namespace", namespace)

	client, err := k8s.New(kubeconfig, kctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("k8s connection failed: %v", err)), nil
	}

	services, err := client.ListAll(ctx, namespace)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list deployments: %v", err)), nil
	}

	data, _ := json.MarshalIndent(services, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleCompareReleaseVsDeployed(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := request.GetString("namespace", "default")
	kubeconfig := request.GetString("kubeconfig", "")
	kctx := request.GetString("context", "")
	slog.Info("tool.call", "tool", "compare_release_vs_deployed", "namespace", namespace)

	if len(s.config.Services) == 0 {
		return mcp.NewToolResultText("No services configured."), nil
	}

	// Connect to K8s
	k8sClient, err := k8s.New(kubeconfig, kctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("k8s connection failed: %v", err)), nil
	}

	// Get deployed services
	deployed, err := k8sClient.ListAll(ctx, namespace)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list deployments: %v", err)), nil
	}

	// Build lookup map: deployment name → deployed version
	deployedVersions := make(map[string]string)
	for _, d := range deployed {
		deployedVersions[d.Name] = d.AppVersion
	}

	type comparison struct {
		Service         string `json:"service"`
		Platform        string `json:"platform"`
		LatestRelease   string `json:"latest_release"`
		DeployedVersion string `json:"deployed_version"`
		UpToDate        bool   `json:"up_to_date"`
		Error           string `json:"error,omitempty"`
	}

	results := make([]comparison, len(s.config.Services))
	var wg sync.WaitGroup

	for i, svc := range s.config.Services {
		wg.Add(1)
		go func(idx int, svc config.ServiceConfig) {
			defer wg.Done()

			parsed, err := config.ParseRepo(svc.Repo)
			if err != nil {
				results[idx] = comparison{Service: svc.Name, Error: err.Error()}
				return
			}

			p, err := s.getProvider(parsed.Platform)
			if err != nil {
				results[idx] = comparison{Service: svc.Name, Platform: parsed.Platform, Error: err.Error()}
				return
			}

			release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
			if err != nil {
				results[idx] = comparison{Service: svc.Name, Platform: parsed.Platform, Error: err.Error()}
				return
			}

			deployedVer := deployedVersions[svc.Name]
			upToDate := deployedVer != "" && (deployedVer == release.Tag || "v"+deployedVer == release.Tag || deployedVer == strings.TrimPrefix(release.Tag, "v"))

			results[idx] = comparison{
				Service:         svc.Name,
				Platform:        parsed.Platform,
				LatestRelease:   release.Tag,
				DeployedVersion: deployedVer,
				UpToDate:        upToDate,
			}
		}(i, svc)
	}

	wg.Wait()

	data, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// ── Phase 7: Extended Analysis Handlers ──────────────────────────────

func (s *Server) handleChangelogBetweenVersions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	owner := request.GetString("owner", "")
	repo := request.GetString("repo", "")
	from := request.GetString("from", "")
	to := request.GetString("to", "")
	platform := request.GetString("platform", "github")
	slog.Info("tool.call", "tool", "changelog_between_versions", "owner", owner, "repo", repo, "from", from, "to", to)

	if owner == "" || repo == "" || from == "" || to == "" {
		return mcp.NewToolResultError("owner, repo, from, and to are required"), nil
	}

	p, err := s.getProvider(platform)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	releases, err := p.ListReleases(ctx, owner, repo)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list releases: %v", err)), nil
	}

	// Find releases between from and to
	var inRange bool
	var changelog []map[string]string

	// Releases are typically newest-first, so we need to handle ordering
	for _, r := range releases {
		if r.Tag == to {
			inRange = true
		}
		if inRange {
			entry := map[string]string{
				"tag":  r.Tag,
				"name": r.Name,
				"date": r.PublishedAt.Format("2006-01-02"),
			}
			if r.Body != "" {
				body := r.Body
				if len(body) > 500 {
					body = body[:497] + "..."
				}
				entry["notes"] = body
			}
			changelog = append(changelog, entry)
		}
		if r.Tag == from {
			break
		}
	}

	if len(changelog) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No releases found between %s and %s", from, to)), nil
	}

	result := map[string]any{
		"repository": fmt.Sprintf("%s/%s", owner, repo),
		"from":       from,
		"to":         to,
		"releases":   len(changelog),
		"changelog":  changelog,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleSecurityAdvisories(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ecosystem := request.GetString("ecosystem", "")
	pkg := request.GetString("package", "")
	version := request.GetString("version", "")
	slog.Info("tool.call", "tool", "security_advisories", "ecosystem", ecosystem, "package", pkg, "version", version)

	if ecosystem == "" || pkg == "" || version == "" {
		return mcp.NewToolResultError("ecosystem, package, and version are required"), nil
	}

	client := security.New()
	vulns, err := client.QueryByPackage(ctx, ecosystem, pkg, version)
	if err != nil {
		slog.Error("tool.error", "tool", "security_advisories", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("vulnerability check failed: %v", err)), nil
	}

	result := map[string]any{
		"ecosystem":       ecosystem,
		"package":         pkg,
		"version":         version,
		"total_vulns":     len(vulns),
		"vulnerabilities": vulns,
	}

	if len(vulns) == 0 {
		result["status"] = "no known vulnerabilities"
	} else {
		result["status"] = fmt.Sprintf("%d vulnerabilities found", len(vulns))
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleReleaseTimeline(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	days := request.GetInt("days", 30)
	slog.Info("tool.call", "tool", "release_timeline", "days", days)

	if len(s.config.Services) == 0 {
		return mcp.NewToolResultText("No services configured."), nil
	}

	cutoff := time.Now().AddDate(0, 0, -days)

	type timelineEntry struct {
		Date    string `json:"date"`
		Service string `json:"service"`
		Tag     string `json:"tag"`
		Name    string `json:"name"`
		URL     string `json:"url"`
	}

	var mu sync.Mutex
	var entries []timelineEntry
	var wg sync.WaitGroup

	for _, svc := range s.config.Services {
		wg.Add(1)
		go func(svc config.ServiceConfig) {
			defer wg.Done()

			parsed, err := config.ParseRepo(svc.Repo)
			if err != nil {
				return
			}

			p, err := s.getProvider(parsed.Platform)
			if err != nil {
				return
			}

			releases, err := p.ListReleases(ctx, parsed.Owner, parsed.RepoName)
			if err != nil {
				return
			}

			for _, r := range releases {
				if r.PublishedAt.Before(cutoff) {
					break // Releases are newest-first, so we can stop
				}
				mu.Lock()
				entries = append(entries, timelineEntry{
					Date:    r.PublishedAt.Format("2006-01-02 15:04"),
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

	// Sort by date descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date > entries[j].Date
	})

	result := map[string]any{
		"period":   fmt.Sprintf("last %d days", days),
		"total":    len(entries),
		"timeline": entries,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
