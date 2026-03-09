# ReleaseWave - Deep Dive Analysis

## Concept
Universal release/version aggregator for microservices exposed as an MCP server.
One tool to check what's released, what's deployed, and what's outdated — across all platforms.

## Inspiration
- [libsrc-mcp](https://github.com/fprochazka/libsrc-mcp) — MCP server for dependency source code inspection
- Extended with multi-platform release tracking, deployment status, and security advisories

## Architecture

### Core: MCP Server in Go
- Single binary, fast startup, native concurrency
- HTTP+SSE transport for MCP protocol
- Plugin-based provider system

### Provider Interface
```go
type Provider interface {
    Name() string
    ListReleases(ctx context.Context, repo string) ([]Release, error)
    GetLatestRelease(ctx context.Context, repo string) (*Release, error)
    GetTags(ctx context.Context, repo string) ([]Tag, error)
    CompareVersions(ctx context.Context, repo string, from, to string) (*Diff, error)
}
```

### Platform Providers

#### Git Platforms (Release/Tag sources)
| Provider | API | Priority |
|----------|-----|----------|
| GitHub | REST v3 + GraphQL v4 | P0 |
| GitLab | REST v4 | P0 |
| Bitbucket | REST v2 | P1 |
| Gitea/Forgejo | REST v1 | P2 |
| Azure DevOps | REST | P2 |

#### Container Registries (Deployed image versions)
| Provider | API | Priority |
|----------|-----|----------|
| Docker Hub | Registry v2 | P1 |
| GHCR | Registry v2 (OCI) | P0 |
| GitLab Container Registry | Registry v2 | P0 |
| AWS ECR | AWS SDK | P1 |
| Google GCR/Artifact Registry | GCP SDK | P2 |
| Azure ACR | Azure SDK | P2 |
| Harbor | Registry v2 | P2 |

#### Deployment Targets (Running version sources)
| Provider | API | Priority |
|----------|-----|----------|
| Kubernetes | client-go | P0 |
| ArgoCD | REST API | P1 |
| Flux | K8s CRDs | P1 |
| Docker Compose | Docker API | P2 |
| Nomad | HTTP API | P2 |
| AWS ECS | AWS SDK | P2 |

#### Package Registries (Library version sources)
| Provider | API | Priority |
|----------|-----|----------|
| npm | Registry API | P1 |
| PyPI | JSON API | P1 |
| Maven Central | Search API | P1 |
| pkg.go.dev | Module proxy | P0 |
| crates.io | REST API | P2 |
| NuGet | REST API | P2 |

## MCP Tools

### Core Tools
1. **find_releases** — List releases for a service from its git platform
2. **check_latest_version** — Get latest release/tag for a service
3. **compare_release_vs_deployed** — Compare what's released vs what's running
4. **list_all_services** — List all configured services with current status
5. **find_outdated_services** — Find services where deployed != latest

### Extended Tools
6. **changelog_between_versions** — Aggregated changelog between two versions
7. **release_timeline** — Timeline view of releases across all services
8. **security_advisories** — CVEs affecting deployed versions
9. **dependency_matrix** — Shared library versions across services
10. **diff_versions** — Code diff between two versions of a service
11. **service_graph** — Dependency/communication graph between services
12. **upgrade_plan** — Suggest coordinated upgrade path

## Data Flow

```
Config (YAML)
    │
    ▼
┌──────────────┐     ┌──────────────────┐     ┌──────────────────┐
│ Git Platforms │     │ Container Regs   │     │ Deploy Targets   │
│ (releases)   │     │ (image tags)     │     │ (running vers)   │
└──────┬───────┘     └────────┬─────────┘     └────────┬─────────┘
       │                      │                        │
       ▼                      ▼                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Version Resolver                           │
│  - Normalize semver across platforms                            │
│  - Match release tags to container image tags                   │
│  - Map deployed image digests to versions                       │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                       MCP Server                                │
│  - Expose tools via HTTP+SSE                                    │
│  - Cache results with configurable TTL                          │
│  - Parallel queries across providers                            │
└─────────────────────────────────────────────────────────────────┘
```

## Configuration Example

```yaml
# ~/.config/releasewave/config.yaml
services:
  - name: user-api
    repo: github.com/UnityInFlow/user-api
    registry: ghcr.io/unityinflow/user-api
    k8s:
      context: prod-cluster
      namespace: default
      deployment: user-api

  - name: billing-service
    repo: gitlab.com/unityinflow/billing
    registry: registry.gitlab.com/unityinflow/billing
    k8s:
      context: prod-cluster
      namespace: billing
      deployment: billing-service

  - name: gateway
    repo: github.com/UnityInFlow/gateway
    registry: ghcr.io/unityinflow/gateway
    argocd:
      server: argocd.internal
      app: gateway

cache:
  ttl: 15m

server:
  port: 7891
```

## Competitive Landscape

| Tool | Scope | vs ReleaseWave |
|------|-------|----------------|
| Renovate/Dependabot | Single-repo dep updates | RW: cross-service, multi-platform |
| Backstage | Service catalog (manual) | RW: active version checking, automated |
| ArgoCD | K8s GitOps status | RW: not K8s-only, aggregates all sources |
| Prometheus/Grafana | Metrics & dashboards | RW: release-focused, AI-agent accessible |
| libsrc-mcp | Dep source code access | RW: release tracking, deployment status |

## Key Design Decisions
1. **Go** — single binary, fast, great concurrency, native Go module understanding
2. **MCP protocol** — accessible by AI agents (Claude Code, Copilot, Cursor)
3. **Provider plugins** — easy to extend with new platforms
4. **Config-driven** — YAML config for service definitions
5. **Cache layer** — avoid hammering APIs, configurable TTL
6. **Semver normalization** — consistent version comparison across platforms
