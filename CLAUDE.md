# ReleaseWave

Universal release/version aggregator for microservices with MCP server support.

## Build & Test

```bash
make build        # CGO_ENABLED=0 go build -o bin/releasewave
make test         # go test -race -cover ./...
make lint         # golangci-lint run ./...
gofmt -w .        # format all Go files
go vet ./...      # static analysis
```

## Architecture

- **CLI**: `cmd/releasewave/` — Cobra commands (`root.go` defines globals: `cfg`, `cfgFile`)
- **MCP Server**: `internal/mcpserver/` — 18 MCP tools, SSE+stdio transports
- **Providers**: `internal/provider/github/`, `internal/provider/gitlab/` — implement `provider.Provider` interface
- **REST API**: `internal/api/` — mounted at `/api` with `StripPrefix`, routes use `/v1/...`
- **Config**: `internal/config/` — YAML config with env var overrides (`GITHUB_TOKEN`, `GITLAB_TOKEN`, `RELEASEWAVE_API_KEY`)
- **Storage**: `internal/store/` — SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Tenants**: `internal/tenant/` — multi-tenant CRUD + API key management (requires `PRAGMA foreign_keys = ON`)

## Conventions

- All HTTP error responses must be JSON with `Content-Type: application/json`
- Use `marshalResult()` in mcpserver for all MCP tool JSON responses (never `json.MarshalIndent` with `_`)
- SSE transport requires `http.Flusher` — any `ResponseWriter` wrapper must delegate `Flush()`
- No `WriteTimeout` on the HTTP server (kills SSE connections); use `ReadTimeout` + `IdleTimeout`
- Prometheus labels must have bounded cardinality — use `normalizePath()` in metrics middleware
- Store errors must be logged, never swallowed with `_ =`
- Auth supports `Authorization: Bearer` and `X-API-Key` headers only (no query params)
- SQLite stores must enable `PRAGMA foreign_keys = ON` before migrations

## Testing

- Tests use `modernc.org/sqlite` with `:memory:` databases
- Provider tests use `httptest.NewServer` with mock responses
- API tests use `httptest.NewRecorder` with a `mockProvider`
- Run with `-race` flag in CI
