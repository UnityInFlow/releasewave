# ReleaseWave

[![CI](https://github.com/UnityInFlow/releasewave/actions/workflows/ci.yml/badge.svg)](https://github.com/UnityInFlow/releasewave/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.23+-blue.svg)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Universal release/version aggregator for microservices. Checks releases across GitHub, GitLab, and other platforms — exposed as an MCP server for AI agent integration or used as a standalone CLI.

## Features

- **Multi-platform**: GitHub and GitLab release tracking
- **MCP Server**: 12 tools accessible by Claude Code, Cursor, VS Code Copilot, and more
- **Container Registry**: Query image tags from any OCI-compatible registry (GHCR, Docker Hub, ECR, etc.)
- **Kubernetes**: Read deployed versions from Deployments/StatefulSets
- **Security**: CVE checking via OSV.dev database
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
releasewave serve --transport=sse    # HTTP+SSE on port 7891
```

### Use as CLI

```bash
releasewave releases docker/compose
releasewave latest kubernetes/kubernetes
releasewave tags golang/go
releasewave check                     # check all configured services
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

### Extended Analysis Tools

| Tool | Description |
|------|-------------|
| `changelog_between_versions` | Aggregate release notes between two versions |
| `security_advisories` | Check for CVEs affecting a package version (OSV.dev) |
| `release_timeline` | Cross-service release timeline sorted by date |

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

log:
  level: info
  format: text
```

## Docker

```bash
docker build -t releasewave .
docker run -p 7891:7891 -v ~/.config/releasewave:/home/releasewave/.config/releasewave releasewave
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
  mcpserver/              MCP server (stdio + SSE, 12 tools)
  registry/               OCI container registry client
  k8s/                    Kubernetes integration (client-go)
  security/               Vulnerability checking (OSV.dev API)
  cache/                  Thread-safe in-memory TTL cache
  ratelimit/              Token-bucket rate limiter
  errors/                 Typed errors (NotFound, RateLimit, Auth)
  logging/                Structured logging setup (slog)
```

## License

MIT
