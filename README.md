# ReleaseWave

[![CI](https://github.com/UnityInFlow/releasewave/actions/workflows/ci.yml/badge.svg)](https://github.com/UnityInFlow/releasewave/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.25+-blue.svg)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Universal release/version aggregator for microservices. Checks releases across GitHub, GitLab, and other platforms — exposed as an MCP server for AI agent integration or used as a standalone CLI.

## Features

- **Multi-platform**: GitHub and GitLab release tracking
- **MCP Server**: 18 tools accessible by Claude Code, Cursor, VS Code Copilot, and more
- **Container Registry**: Query image tags from any OCI-compatible registry (GHCR, Docker Hub, ECR, etc.)
- **Kubernetes**: Read deployed versions, auto-discover services, compare release vs deployed
- **Security**: CVE checking via OSV.dev database
- **Web Dashboard**: Real-time service status dashboard at `/dashboard`
- **Notifications**: Webhook notifications on new releases
- **CLI**: Direct commands for querying releases, tags, and service status
- **Concurrent**: Checks multiple services in parallel
- **Cached**: In-memory cache with configurable TTL
- **Rate-limited**: Per-provider rate limiting to respect API limits
- **Single binary**: No runtime dependencies, cross-platform

## Quick Start

### Install

```bash
# From source
go install github.com/UnityInFlow/releasewave/cmd/releasewave@latest

# Or download a release binary
# https://github.com/UnityInFlow/releasewave/releases

# Docker
docker run -p 7891:7891 ghcr.io/unityinflow/releasewave:latest

# Or build from repo
git clone https://github.com/UnityInFlow/releasewave.git
cd releasewave
make build
```

### Configure

```bash
# Generate default config
releasewave init

# Edit config to add your services
# Config location: ~/.config/releasewave/config.yaml
```

### Use as MCP Server

```bash
# Auto-configure Claude Code, Cursor, VS Code
releasewave install

# Or start manually
releasewave serve                    # stdio (default, for AI tools)
releasewave serve --transport=sse    # HTTP+SSE on port 7891 (+ web dashboard)
```

### Use as CLI

```bash
releasewave releases docker/compose
releasewave latest kubernetes/kubernetes
releasewave tags golang/go
releasewave check                     # check all configured services

# GitLab support
releasewave releases gitlab-org/gitlab --platform gitlab
releasewave latest my-org/my-project --platform gitlab

# Kubernetes auto-discovery
releasewave discover --namespace production
releasewave discover --merge    # auto-add discovered services to config
```

## MCP Tools

### Core Tools

| Tool | Description |
|------|-------------|
| `list_releases` | List releases for a GitHub/GitLab repository |
| `get_latest_release` | Get the latest release with notes |
| `list_tags` | List git tags with commit SHAs |
| `check_services` | Check all configured services |
| `find_outdated` | Find services behind their latest release |

### Container Registry Tools

| Tool | Description |
|------|-------------|
| `list_image_tags` | List tags from any OCI registry (GHCR, Docker Hub, ECR, etc.) |
| `compare_image_tags` | Check if two image tags point to the same digest |

### Kubernetes Tools

| Tool | Description |
|------|-------------|
| `list_k8s_deployments` | List deployments/statefulsets with their image versions |
| `compare_release_vs_deployed` | Compare latest release vs what's deployed in K8s |
| `discover_services` | Auto-discover services from K8s annotations or image names |

### Extended Analysis Tools

| Tool | Description |
|------|-------------|
| `changelog_between_versions` | Aggregate release notes between two versions |
| `security_advisories` | Check for CVEs affecting a package version (OSV.dev) |
| `release_timeline` | Cross-service release timeline sorted by date |

### Dependency & Upgrade Tools

| Tool | Description |
|------|-------------|
| `get_repo_file` | Fetch file content from a repo (go.mod, package.json, etc.) |
| `dependency_matrix` | Analyze shared dependencies across configured services |
| `upgrade_plan` | Generate prioritized upgrade plan for outdated services |
| `watch_releases` | Detect new releases and send webhook notifications |
| `service_graph` | Build a dependency graph showing shared libraries across services |

## Configuration

```yaml
# ~/.config/releasewave/config.yaml

services:
  - name: my-api
    repo: github.com/my-org/my-api
  - name: billing
    repo: gitlab.com/my-org/billing

tokens:
  github: ""   # or set GITHUB_TOKEN env var
  gitlab: ""   # or set GITLAB_TOKEN env var

cache:
  ttl: 15m

server:
  port: 7891

rate_limit:
  github: 5    # requests per second
  gitlab: 3

notifications:
  enabled: false
  webhook_url: "https://hooks.slack.com/services/..."

log:
  level: info
  format: text
```

### Kubernetes Auto-Discovery

ReleaseWave can auto-discover services from your Kubernetes cluster using annotations:

```yaml
# Add to your Deployment/StatefulSet metadata
metadata:
  annotations:
    releasewave.io/repo: "github.com/my-org/my-api"
    releasewave.io/name: "my-api"
```

If no annotations are present, ReleaseWave will attempt to infer the repository from the container image name.

## Web Dashboard

When running in SSE mode, a web dashboard is available at `http://localhost:7891/dashboard` showing real-time status of all configured services.

```bash
releasewave serve --transport=sse --port=7891
# Open http://localhost:7891/dashboard
```

## Docker

```bash
# Pre-built image
docker run -p 7891:7891 \
  -v ~/.config/releasewave:/home/releasewave/.config/releasewave \
  ghcr.io/unityinflow/releasewave:latest serve --transport=sse

# Or build locally
docker build -t releasewave .
```

## Development

```bash
make build     # build binary
make test      # run tests with race detection
make lint      # run linters
make clean     # clean build artifacts
```

## Architecture

```
cmd/releasewave/          CLI entry point (Cobra commands)
internal/
  config/                 YAML config loading + validation
  model/                  Core data types (Release, Tag, Service)
  provider/               Provider interface + cached decorator
    github/               GitHub REST API client
    gitlab/               GitLab REST API client
  mcpserver/              MCP server (stdio + SSE, 18 tools)
  registry/               OCI container registry client
  k8s/                    Kubernetes integration (client-go)
  security/               Vulnerability checking (OSV.dev API)
  discovery/              K8s service auto-discovery
  notify/                 Webhook notification system
  web/                    Web dashboard (html/template)
  cache/                  Thread-safe in-memory TTL cache
  ratelimit/              Token-bucket rate limiter
  errors/                 Typed errors (NotFound, RateLimit, Auth)
  logging/                Structured logging setup (slog)
```

## License

MIT
