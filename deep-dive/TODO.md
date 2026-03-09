# ReleaseWave - TODO

## Phase 0: Go Learning Foundation
> Learn Go fundamentals through building ReleaseWave step by step

- [ ] Set up Go development environment (go 1.22+)
- [ ] Learn Go basics: types, structs, interfaces, error handling
- [ ] Learn Go concurrency: goroutines, channels, sync primitives
- [ ] Learn Go modules and dependency management
- [ ] Learn Go testing: unit tests, table-driven tests, mocks
- [ ] Learn Go HTTP: net/http, middleware, JSON handling
- [ ] Build a minimal CLI with cobra/urfave — "hello releasewave"

## Phase 1: Project Bootstrap
- [ ] Initialize Go module (`github.com/UnityInFlow/releasewave`)
- [ ] Set up project structure (cmd/, internal/, pkg/)
- [ ] Add basic CLI skeleton (cobra or urfave/cli)
- [ ] Add configuration loading (YAML → Go structs with viper)
- [ ] Add structured logging (slog)
- [ ] Set up CI/CD (GitHub Actions: lint, test, build)
- [ ] Add Makefile with common commands
- [ ] Add .goreleaser.yml for release automation

## Phase 2: Provider Interface & GitHub Provider
- [ ] Define Provider interface
- [ ] Define core data models (Release, Tag, Service, Version)
- [ ] Implement GitHub provider (REST API with go-github)
  - [ ] ListReleases
  - [ ] GetLatestRelease
  - [ ] GetTags
  - [ ] CompareVersions
- [ ] Add authentication (token-based)
- [ ] Add rate limiting handling
- [ ] Write tests with recorded HTTP responses (go-vcr)

## Phase 3: GitLab Provider
- [ ] Implement GitLab provider (REST API with go-gitlab)
  - [ ] ListReleases
  - [ ] GetLatestRelease
  - [ ] GetTags
  - [ ] CompareVersions
- [ ] Add authentication (PAT / OAuth)
- [ ] Write tests

## Phase 4: MCP Server
- [ ] Integrate mcp-go SDK (mark3labs/mcp-go)
- [ ] Implement HTTP+SSE transport
- [ ] Register first tool: `list_all_services`
- [ ] Register tool: `find_releases`
- [ ] Register tool: `check_latest_version`
- [ ] Register tool: `find_outdated_services`
- [ ] Test with Claude Code / VS Code
- [ ] Add `releasewave install` command for auto-configuring AI tools

## Phase 5: Container Registry Support
- [ ] Implement OCI Distribution Spec client
- [ ] GHCR provider
- [ ] GitLab Container Registry provider
- [ ] Docker Hub provider
- [ ] Map image tags to release versions

## Phase 6: Kubernetes Integration
- [ ] Implement K8s provider (client-go)
- [ ] Read deployed versions from Deployments/StatefulSets
- [ ] Register tool: `compare_release_vs_deployed`
- [ ] Add multi-cluster support

## Phase 7: Extended Tools
- [ ] `changelog_between_versions` — aggregate release notes
- [ ] `release_timeline` — cross-service release timeline
- [ ] `security_advisories` — CVE checking (OSV.dev API)
- [ ] `dependency_matrix` — shared lib versions across services
- [ ] `service_graph` — service communication map
- [ ] `upgrade_plan` — suggest coordinated upgrades

## Phase 8: Polish & Distribution
- [ ] Documentation (usage, config, provider setup)
- [ ] Homebrew formula
- [ ] Docker image
- [ ] Auto-discovery of services from K8s/ArgoCD
- [ ] Web dashboard (optional, htmx or templ)

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
