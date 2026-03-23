# market-scanner

Pre-build competitor intelligence for the [Claude Code Factory](https://github.com/timholm/claude-code-factory). Searches GitHub, npm, and PyPI for existing solutions before the factory invests build time on a product idea. Prevents wasted effort on already-solved problems by computing a novelty score and issuing a proceed/skip recommendation.

## How It Works

Given a product name and problem description, market-scanner:

1. **Searches GitHub** for repositories with 100+ stars matching the name, topics, and problem keywords via the GitHub Search API
2. **Searches npm** for packages with relevance scoring across name and keyword queries via the npm registry API
3. **Searches PyPI** via direct JSON API package lookups (with name variations) and warehouse HTML search result parsing
4. **Computes a novelty score** (0.0 = saturated market, 1.0 = completely novel) using a weighted algorithm that factors in star counts, name similarity, and cross-registry presence
5. **Returns a recommendation**: `PROCEED`, `PROCEED_WITH_CAUTION`, or `SKIP`
6. **Persists results** to SQLite for historical tracking and factory pipeline integration

All three registry searches run concurrently. GitHub failure is fatal; npm and PyPI failures are non-fatal (the scan completes with partial data).

## Architecture

```
market-scanner
├── main.go                          # CLI entrypoint (cobra): scan, scan-queue, serve
├── internal/
│   ├── api/server.go                # Gin HTTP API: /health, /scan/:name, POST /scan, /reports
│   ├── config/config.go             # Env-based config with sensible defaults
│   ├── db/db.go                     # SQLite (WAL mode) persistence: scan_results + build_queue
│   └── scanner/
│       ├── scanner.go               # Orchestrator: concurrent search + novelty scoring algorithm
│       ├── github.go                # GitHub Search API client (repos, topics, keyword queries)
│       ├── npm.go                   # npm registry search client with quality/popularity scores
│       ├── pypi.go                  # PyPI JSON API + warehouse HTML parser
│       └── report.go                # Human-readable and compact report formatters
├── deploy/cronjob.yaml              # K8s CronJob for daily queue scanning (factory namespace)
├── Dockerfile                       # Multi-stage alpine build with CGO/SQLite support
└── Makefile                         # build, test, lint, docker, docker-arm64, deploy targets
```

Single Go binary. No frontend. SQLite for persistence. Three CLI commands.

## Install

```bash
go install github.com/timholm/market-scanner@latest
```

Or build from source:

```bash
git clone https://github.com/timholm/market-scanner.git
cd market-scanner
make build
# Binary at ./bin/market-scanner
```

Requires Go 1.22+ and GCC (CGO is required for the SQLite driver).

## CLI Commands

### `scan` -- Single Product Scan

Run a one-off market scan for a product idea.

```bash
# Basic scan with human-readable report
market-scanner scan --name "code-review-bot" --problem "automated code review for pull requests"

# JSON output for piping into other tools
market-scanner scan --name "log-aggregator" --problem "centralized logging for microservices" --json

# CI gating: exit code 1 if novelty is below threshold
market-scanner scan --name "kubectl" --problem "Kubernetes CLI" || echo "SKIP: too much competition"
```

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--name` | `-n` | Yes | Product name to scan for |
| `--problem` | `-p` | No | Problem description the product solves |
| `--json` | | No | Output as JSON instead of human-readable report |

Exit code 1 when novelty score is below the configured threshold, enabling CI/pipeline gating.

### `scan-queue` -- Batch Queue Processing

Process all `pending` items in the `build_queue` SQLite table. Each item gets scanned and marked as `scanned_proceed` or `scanned_skip` based on the novelty threshold. Designed to run as a K8s CronJob in the factory pipeline.

```bash
market-scanner scan-queue
```

Output is a compact one-line summary per item:

```
[0.85] tool-mesh — PROCEED: Highly novel. No significant competition found. (gh:2 npm:5 pypi:1)
[0.42] log-parser — SKIP: Significant competition exists. Consider a different angle. (gh:15 npm:30 pypi:8)
```

### `serve` -- HTTP API Server

Start the HTTP API server on the configured listen address.

```bash
GITHUB_TOKEN=ghp_xxx market-scanner serve
# market-scanner API listening on :8090
```

## HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check. Returns `{"status": "ok", "service": "market-scanner"}` |
| `GET` | `/scan/:name?problem=...` | Run a scan by name with optional problem query param |
| `POST` | `/scan` | Run a scan with JSON body: `{"name": "...", "problem": "..."}` |
| `GET` | `/reports` | List the 100 most recent scan results |
| `GET` | `/reports/:name` | Get the latest scan result for a specific product name |

### Examples

```bash
# Trigger a scan via API
curl -X POST http://localhost:8090/scan \
  -H "Content-Type: application/json" \
  -d '{"name": "task-queue", "problem": "distributed background job processing"}'

# GET-based scan
curl "http://localhost:8090/scan/task-queue?problem=distributed+background+job+processing"

# Retrieve previous results
curl http://localhost:8090/reports
curl http://localhost:8090/reports/task-queue
```

### Response Format

```json
{
  "name": "task-queue",
  "problem": "distributed background job processing",
  "github": [
    {
      "full_name": "hibiken/asynq",
      "description": "Simple, reliable, and efficient distributed task queue in Go",
      "stars": 9200,
      "forks": 680,
      "language": "Go",
      "url": "https://github.com/hibiken/asynq",
      "updated_at": "2026-03-20T10:00:00Z",
      "topics": ["task-queue", "redis", "go"]
    }
  ],
  "npm": [
    {
      "name": "bull",
      "description": "Premium Queue package for handling distributed jobs",
      "version": "4.12.0",
      "keywords": ["queue", "jobs", "redis"],
      "url": "https://www.npmjs.com/package/bull",
      "score": 0.92
    }
  ],
  "pypi": [
    {
      "name": "celery",
      "description": "Distributed Task Queue",
      "version": "5.4.0",
      "url": "https://pypi.org/project/celery/"
    }
  ],
  "novelty_score": 0.35,
  "recommendation": "SKIP: Significant competition exists. Consider a different angle or skip.",
  "scan_duration": 2400000000
}
```

## Configuration

All configuration is via environment variables with sensible defaults.

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_TOKEN` | _(none)_ | GitHub personal access token. Without it, you hit unauthenticated rate limits (10 req/min vs 30 req/min). |
| `FACTORY_DATA_DIR` | `/tmp/factory-data` | Base directory for data storage |
| `DB_PATH` | `$FACTORY_DATA_DIR/market-scanner.db` | SQLite database file path |
| `NOVELTY_THRESHOLD` | `0.6` | Minimum novelty score (0.0-1.0) to recommend proceeding |
| `LISTEN_ADDR` | `:8090` | HTTP API listen address |

## Novelty Scoring Algorithm

The novelty score starts at 1.0 (fully novel) with penalties applied from each registry:

### GitHub (max penalty: 0.60)

Each repository penalizes based on:
- **Star weight**: `min(stars / 10000, 1.0)` -- a 10k+ star repo is maximum signal
- **Name similarity**: substring match (0.8 weight) + word overlap (0.4 weight), capped at 1.0
- **Per-repo penalty**: `(0.05 + 0.15 * starWeight) * nameSimilarity`

### npm (max penalty: 0.25)

Each package penalizes: `0.03 * npmQualityScore * nameSimilarity`

### PyPI (max penalty: 0.15)

Each package penalizes: `0.02 * nameSimilarity`

### Recommendation Thresholds

| Score Range | Recommendation | Action |
|-------------|---------------|--------|
| >= 0.8 | `PROCEED` | Highly novel. Build it. |
| >= threshold (0.6) | `PROCEED_WITH_CAUTION` | Competitors exist but differentiation possible. |
| >= 0.3 | `SKIP` | Significant competition. Consider a different angle. |
| < 0.3 | `SKIP` | Market saturated. Problem is well-solved. |

## Factory Pipeline Integration

market-scanner runs as a K8s CronJob in the `factory` namespace at 5 AM daily, positioned before the other factory stages:

| Time | Stage | Description |
|------|-------|-------------|
| Hourly | **gather** | Scrape ideas from GitHub trending, HN, Reddit, arXiv, YC, Product Hunt |
| 5:00 AM | **market-scanner** | Scan pending ideas for competition |
| 6:00 AM | **analyze** | Generate specs for ideas that passed the scan |
| 6:30 AM | **build** | Build products from specs using Claude Code headless |

### Deploy

```bash
# Apply the CronJob
make deploy
# or
kubectl apply -f deploy/cronjob.yaml
```

Requires:
- `factory-secrets` Secret with `github-token` key
- `factory-data` PersistentVolumeClaim (shared NFS volume with other factory components)

Resource allocation: 64-256Mi memory, 100-500m CPU. 30-minute deadline per run.

## Database Schema

SQLite with WAL journaling and 10-second busy timeout for concurrent access.

### `scan_results`

Persisted scan output for historical tracking and API retrieval.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER | Auto-incrementing primary key |
| `name` | TEXT | Product name that was scanned |
| `problem` | TEXT | Problem description |
| `novelty_score` | REAL | Computed novelty score (0.0-1.0) |
| `recommendation` | TEXT | Human-readable recommendation string |
| `report_json` | TEXT | Full scan result serialized as JSON |
| `scanned_at` | DATETIME | Timestamp of the scan |

Indexed on `name` for fast lookups.

### `build_queue`

Items waiting to be scanned, populated by the factory's analyze stage.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER | Auto-incrementing primary key |
| `name` | TEXT | Product name |
| `problem` | TEXT | Problem description |
| `status` | TEXT | `pending`, `scanned_proceed`, or `scanned_skip` |
| `created_at` | DATETIME | When the item was queued |
| `scanned_at` | DATETIME | When the scan completed (NULL until scanned) |

Indexed on `status` for efficient pending item retrieval.

## Development

```bash
make build        # Build binary to bin/market-scanner
make test         # Run all tests with -race -count=1
make lint         # Run golangci-lint
make run          # Build and run a sample scan
make serve        # Build and start the HTTP API
make clean        # Remove build artifacts
make docker       # Build Docker image (linux/amd64)
make docker-arm64 # Build and push arm64 image for K8s cluster
make push         # Build and push Docker image
make deploy       # kubectl apply the CronJob
```

## License

MIT
