# CLAUDE.md -- market-scanner

Instructions for Claude Code and other AI agents working on this codebase.

## Project Overview

market-scanner is a pre-build competitor scanner for the Claude Code Factory. Before the factory builds a product, market-scanner searches GitHub, npm, and PyPI for existing solutions. It computes a novelty score (0.0-1.0) and recommends whether to proceed or skip. This prevents the factory from wasting build cycles on problems that are already well-solved.

## Architecture

Single Go binary (`main.go`) with three subcommands via cobra:
- `scan` -- one-off scan of a product name + problem description
- `scan-queue` -- batch process all pending items in the SQLite build queue
- `serve` -- HTTP API server (Gin framework)

### Package Layout

```
main.go                         # CLI entrypoint, cobra command definitions
internal/
  config/config.go              # Env-based config: GITHUB_TOKEN, NOVELTY_THRESHOLD, etc.
  db/db.go                      # SQLite persistence (WAL mode): scan_results, build_queue tables
  scanner/
    scanner.go                  # Orchestrator: runs GitHub/npm/PyPI concurrently, computes novelty
    github.go                   # GitHub Search API client (repos by name, topic, keywords)
    npm.go                      # npm registry search client
    pypi.go                     # PyPI JSON API lookups + warehouse HTML search parsing
    report.go                   # Human-readable report formatting (full + compact)
  api/server.go                 # Gin HTTP API: /health, /scan/:name, POST /scan, /reports
deploy/cronjob.yaml             # K8s CronJob spec for factory namespace
```

### Data Flow

1. Input: product name + optional problem description
2. Build search queries: direct name, topic match, keyword extraction from problem (stop-word filtered)
3. Concurrent execution: GitHub, npm, PyPI searches all fire in goroutines
4. Collect results: GitHub failure is fatal, npm/PyPI failures are non-fatal
5. Compute novelty: weighted penalty system (GitHub max 0.60, npm max 0.25, PyPI max 0.15)
6. Generate recommendation based on score vs threshold
7. Persist to SQLite and return result

### Key Design Decisions

- **CGO required**: SQLite driver (mattn/go-sqlite3) needs CGO. Always build with `CGO_ENABLED=1`.
- **WAL mode + busy_timeout=10000**: Handles concurrent reads from API server while CronJob writes.
- **GitHub is the primary signal**: Gets 60% of the novelty penalty budget. Star count is the strongest competition indicator.
- **Name similarity**: Uses substring matching (0.8 weight) + word overlap (0.4 weight), not fuzzy matching. Simple and fast.
- **Non-fatal npm/PyPI**: If these registries are down, the scan still completes with GitHub-only data.
- **Exit code semantics**: `scan` exits 1 when novelty < threshold, enabling CI/pipeline gating.

## Build and Test

```bash
make build        # CGO_ENABLED=1 go build -o bin/market-scanner
make test         # CGO_ENABLED=1 go test -v -race -count=1 ./...
make lint         # golangci-lint run ./...
make serve        # Build and start HTTP server
make docker       # Multi-stage Docker build
make docker-arm64 # arm64 image for K8s cluster (buildx + push)
make deploy       # kubectl apply -f deploy/cronjob.yaml
```

## Environment Variables

| Variable | Default | Notes |
|----------|---------|-------|
| `GITHUB_TOKEN` | _(none)_ | GitHub PAT. Required for reasonable rate limits. |
| `FACTORY_DATA_DIR` | `/tmp/factory-data` | Base data directory |
| `DB_PATH` | `$FACTORY_DATA_DIR/market-scanner.db` | SQLite file path |
| `NOVELTY_THRESHOLD` | `0.6` | Score below which builds are skipped |
| `LISTEN_ADDR` | `:8090` | API server bind address |

## Conventions and Patterns

- All HTTP handlers live in `internal/api/server.go`, registered in the `routes()` method
- All scanner logic is in `internal/scanner/` -- one file per registry plus the orchestrator
- Tests use `_test.go` suffix, co-located with source files
- Error handling: return `fmt.Errorf("context: %w", err)` for wrapping
- JSON struct tags on all public types for API serialization
- No global state; config is loaded once and passed down
- Database auto-migrates on `Open()` (CREATE TABLE IF NOT EXISTS)

## Common Tasks

### Add a new registry scanner

1. Create `internal/scanner/newregistry.go` with a `SearchNewRegistry(ctx, name, problem)` function
2. Add a result type (e.g., `NewRegistryResult`)
3. Add a channel + goroutine in `scanner.go` `Scan()` method, following the GitHub/npm/PyPI pattern
4. Add the results to `ScanResult` struct
5. Add a penalty calculation in `computeNovelty()`
6. Add a section in `report.go` `FormatReport()`

### Add a new API endpoint

1. Add the handler method on `*Server` in `internal/api/server.go`
2. Register the route in the `routes()` method
3. Follow the existing pattern: parse input, call scanner/db, return JSON

### Adjust novelty scoring

All scoring logic is in `scanner.go` `computeNovelty()`. The three penalty caps (0.60, 0.25, 0.15) sum to 1.0. Adjusting one cap should consider the balance across all registries.

## Dependencies

- `github.com/spf13/cobra` -- CLI framework
- `github.com/gin-gonic/gin` -- HTTP framework
- `github.com/mattn/go-sqlite3` -- SQLite driver (CGO)

No other runtime dependencies. All registry searches use `net/http` directly.

## Deployment

Runs in the `factory` K8s namespace as a CronJob (5 AM daily). Shares the `factory-data` PVC with other factory components. GitHub token comes from `factory-secrets` Secret. Container image at `ghcr.io/timholm/market-scanner:latest` (arm64).
