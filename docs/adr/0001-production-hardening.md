# ADR-0001: Production Hardening & Code Quality

**Date:** 2026-03-12
**Status:** Accepted
**Author:** @jirihermann

## Context

ReleaseWave shipped with 18 MCP tools, GitHub/GitLab providers, Slack/Discord notifications, a daemon, REST API, multi-tenancy, and a GitHub App integration. A code review identified 18 issues across P0-P3 severity levels that made the codebase unsuitable for production deployment.

Key problems:
- API routes were unreachable (double prefix: routes registered as `/api/v1/...` but mounted at `/api` with `StripPrefix`)
- SSE transport was broken (metrics middleware's `statusRecorder` did not implement `http.Flusher`)
- Race conditions on shared config mutation in the REST API
- No HTTP server timeouts (susceptible to slowloris)
- No request body limits on POST endpoints
- Unbounded Prometheus label cardinality from raw URL paths
- SQLite foreign keys silently unenforced (`PRAGMA foreign_keys` not enabled)
- API key auth leaked keys via query parameter support
- Auth error responses used `text/plain` instead of `application/json`
- Daemon `Stop()` panicked on double-close
- Signal handler called `os.Exit(0)`, bypassing deferred cleanup
- Silent error swallowing: `time.Parse`, `LastInsertId`, store writes
- GitHub App JWT generation was a non-functional stub
- 10 packages had zero test coverage

## Decision

### P0 Fixes (service-breaking)

1. **Fix API route prefix** — Changed routes from `/api/v1/services` to `/v1/services` since `serve.go` mounts at `/api` with `http.StripPrefix`.

2. **Restore SSE streaming** — Added `Flush()` method to `statusRecorder` that delegates to the underlying `http.Flusher` interface when available.

3. **Eliminate race condition** — Added `sync.RWMutex` to `apiHandler`. Write endpoints (`addService`, `deleteService`) acquire write lock. Read endpoints (`listServices`, `getTimeline`) acquire read lock and snapshot the services slice.

### P1 Fixes (security/reliability)

4. **HTTP server timeouts** — Added `ReadTimeout: 15s`, `WriteTimeout: 60s`, `IdleTimeout: 120s` to `http.Server`.

5. **Request body limit** — Added `http.MaxBytesReader(w, r.Body, 1<<20)` on POST endpoints.

6. **Prometheus cardinality** — Added `normalizePath()` to collapse dynamic segments (e.g. service names) into placeholders like `{name}`.

7. **SQLite foreign keys** — Added `PRAGMA foreign_keys = ON` in both tenant and API key store migrations.

8. **Remove query param auth** — Removed `?api_key=` support. Keys in URLs leak through server logs, browser history, and Referer headers. Only `Authorization: Bearer` and `X-API-Key` headers are supported.

9. **JSON error responses** — Auth middleware now sets `Content-Type: application/json` on error responses.

### P2 Fixes (robustness)

10. **Daemon double-close** — Wrapped `Stop()` with `sync.Once` to prevent panic.

11. **Signal handling** — Replaced `os.Exit(0)` with `signal.NotifyContext` so deferred cleanup runs.

12. **Concurrent release fetching** — `listServices` API endpoint now fetches releases concurrently.

13. **Shared version loading** — Extracted `loadKnownVersions()` method called from both `Start()` and `RunOnce()`.

14. **Store error logging** — Daemon now logs `SetKV` and `RecordRelease` errors instead of swallowing them.

### Error Handling

15. **time.Parse** — GitHub and GitLab providers now log a debug message for non-empty unparseable timestamps instead of silently producing zero times.

16. **LastInsertId** — Tenant `Create()` now returns an error if the driver fails to provide the inserted ID.

### GitHub App JWT

17. **Real JWT generation** — Replaced the stub with a working RS256 implementation using `golang-jwt/jwt/v5`. Claims: `iss=AppID`, `iat=now-60s`, `exp=now+10m`.

### Test Coverage

18. **8 new test files, 83 tests** covering previously untested packages: `api` (69.6%), `tenant` (81.9%), `errors` (100%), `githubapp` (44.4%), `metrics`, `model`, `logging` (91.7%), `ratelimit` (100%).

### Homebrew

19. **Enabled Homebrew formula** in `.goreleaser.yml` for `brew install` distribution via `UnityInFlow/homebrew-tap`.

## Consequences

- All API routes are now reachable
- SSE transport works through the metrics middleware
- Config mutations are thread-safe
- HTTP server is hardened against slow clients
- Prometheus won't OOM from label explosion
- SQLite foreign key constraints are enforced (cascade deletes work)
- API keys can only be sent via headers
- Test coverage increased from 11 to 21 tested packages
- GitHub App integration is fully functional
- New dependency: `golang-jwt/jwt/v5`

## Alternatives Considered

- **Separate mux for SSE** — Instead of fixing the Flusher delegation, we could have bypassed the metrics middleware for SSE endpoints. Rejected because metrics on SSE connections are valuable.
- **Config file persistence for addService/deleteService** — Instead of in-memory-only config mutation with a mutex, we could write back to `config.yaml`. Deferred for now since the API is secondary to MCP tools.
- **JWT library alternatives** — Considered `lestrrat-go/jwx` but `golang-jwt/jwt/v5` is the most widely adopted and has the simplest API for RS256.
