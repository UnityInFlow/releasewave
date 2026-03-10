# ReleaseWave - TODO

## Phase 0: Go Learning Foundation ✅
> Learn Go fundamentals through building ReleaseWave step by step

- [x] Set up Go development environment (go 1.23+)
- [x] Learn Go basics: types, structs, interfaces, error handling
- [x] Learn Go concurrency: goroutines, channels, sync primitives
- [x] Learn Go modules and dependency management
- [x] Learn Go testing: unit tests, table-driven tests, mocks
- [x] Learn Go HTTP: net/http, middleware, JSON handling
- [x] Build a minimal CLI with cobra — "hello releasewave"

## Phase 1: Project Bootstrap ✅
- [x] Initialize Go module (`github.com/UnityInFlow/releasewave`)
- [x] Set up project structure (cmd/, internal/)
- [x] Add basic CLI skeleton (cobra)
- [x] Add configuration loading (YAML → Go structs)
- [x] Add structured logging (slog)
- [x] Set up CI/CD (GitHub Actions: lint, test, build)
- [x] Add Makefile with common commands
- [x] Add .goreleaser.yml for release automation

## Phase 2: Provider Interface & GitHub Provider ✅
- [x] Define Provider interface
- [x] Define core data models (Release, Tag, Service, Version)
- [x] Implement GitHub provider (REST API)
  - [x] ListReleases
  - [x] GetLatestRelease
  - [x] ListTags
- [x] Add authentication (token-based)
- [x] Add rate limiting handling
- [x] Write tests with httptest mock server

## Phase 3: GitLab Provider ✅
- [x] Implement GitLab provider (REST API)
  - [x] ListReleases
  - [x] GetLatestRelease
  - [x] ListTags
- [x] Add authentication (PRIVATE-TOKEN header)
- [x] Write tests

## Phase 4: MCP Server ✅
- [x] Integrate mcp-go SDK (mark3labs/mcp-go)
- [x] Implement stdio + HTTP+SSE transport
- [x] Register tool: `list_releases`
- [x] Register tool: `get_latest_release`
- [x] Register tool: `list_tags`
- [x] Register tool: `check_services`
- [x] Register tool: `find_outdated`
- [x] Test with Claude Code / VS Code
- [x] Add `releasewave install` command for auto-configuring AI tools

## Phase 5: Container Registry Support ✅
- [x] Implement OCI Distribution Spec client (go-containerregistry)
- [x] Universal registry support (GHCR, Docker Hub, GitLab Registry, ECR, etc.)
- [x] Register tool: `list_image_tags`
- [x] Register tool: `compare_image_tags`

## Phase 6: Kubernetes Integration ✅
- [x] Implement K8s client (client-go)
- [x] Read deployed versions from Deployments/StatefulSets
- [x] Register tool: `list_k8s_deployments`
- [x] Register tool: `compare_release_vs_deployed`

## Phase 7: Extended Tools ✅
- [x] `changelog_between_versions` — aggregate release notes
- [x] `release_timeline` — cross-service release timeline
- [x] `security_advisories` — CVE checking (OSV.dev API)
- [x] `dependency_matrix` — shared lib versions across services
- [x] `upgrade_plan` — suggest coordinated upgrades
- [x] `get_repo_file` — fetch file content from repos
- [x] `watch_releases` — detect new releases since last check
- [x] `discover_services` — auto-discover from K8s annotations
- [x] `service_graph` — service dependency graph from dependency files

## Phase 8: Polish & Distribution ✅
- [ ] Documentation (usage, config, provider setup)
- [x] Homebrew formula (Formula/releasewave.rb + GoReleaser brew tap)
- [x] Docker image (Dockerfile.goreleaser + GHCR)
- [x] Auto-discovery of services from K8s annotations
- [x] Web dashboard (Go html/template, /dashboard endpoint)
- [x] GitLab CLI support (--platform flag on all commands)
- [x] Webhook notifications (JSON POST on new releases)
- [x] Test suite (mcpserver, registry, security)

## Go Learning Milestones (mapped to phases)

| Phase | Go Concepts Learned |
|-------|-------------------|
| 0 | Syntax, types, control flow, packages |
| 1 | Modules, CLI frameworks, config parsing, project layout |
| 2 | Interfaces, HTTP clients, JSON marshaling, testing |
| 3 | Code reuse, interface implementations, DRY patterns |
| 4 | HTTP servers, middleware, protocol implementation |
| 5 | Low-level HTTP, auth flows, binary data |
| 6 | External SDKs, K8s API, complex structs |
| 7 | Concurrency patterns, caching, data aggregation |
| 8 | Build/release tooling, distribution, documentation |
