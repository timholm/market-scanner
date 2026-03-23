# AGENTS.md -- market-scanner

How AI agents should integrate with and use market-scanner.

## What This Tool Does

market-scanner answers a single question: **"Does this product idea already exist?"**

Given a product name and problem description, it searches GitHub, npm, and PyPI, computes a novelty score (0.0 = saturated, 1.0 = novel), and returns a proceed/skip recommendation. It is the first quality gate in the Claude Code Factory's autonomous build pipeline.

## Agent Integration Methods

### Method 1: CLI with Exit Code (Recommended for Pipelines)

```bash
# Exit code 0 = proceed, exit code 1 = below novelty threshold (skip)
market-scanner scan --name "my-tool" --problem "Solves X problem"

# JSON output for structured parsing
market-scanner scan --name "my-tool" --problem "Solves X problem" --json
```

The exit code makes this directly usable in shell pipelines and CI:

```bash
if market-scanner scan --name "$PRODUCT_NAME" --problem "$PROBLEM" --json > /tmp/scan.json 2>&1; then
  echo "Novelty check passed. Proceeding to build."
  # ... continue pipeline
else
  echo "Too much competition. Skipping."
  exit 0
fi
```

### Method 2: HTTP API (Recommended for Services)

```bash
# POST a scan request
curl -s -X POST http://market-scanner:8090/scan \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-tool","problem":"Solves X problem"}'

# GET a scan (name in path, problem in query)
curl -s "http://market-scanner:8090/scan/my-tool?problem=Solves+X+problem"

# Retrieve historical results
curl -s http://market-scanner:8090/reports/my-tool
curl -s http://market-scanner:8090/reports
```

### Method 3: Build Queue (Recommended for Factory Pipeline)

Insert items into the `build_queue` SQLite table with status `pending`, then run:

```bash
market-scanner scan-queue
```

Each item is scanned and marked `scanned_proceed` or `scanned_skip`. The factory's analyze stage should only pick up `scanned_proceed` items.

## Response Schema

```json
{
  "name": "string",
  "problem": "string",
  "github": [
    {
      "full_name": "owner/repo",
      "description": "string",
      "stars": 0,
      "forks": 0,
      "language": "string",
      "url": "https://github.com/owner/repo",
      "updated_at": "2026-01-01T00:00:00Z",
      "topics": ["string"]
    }
  ],
  "npm": [
    {
      "name": "string",
      "description": "string",
      "version": "string",
      "keywords": ["string"],
      "url": "https://www.npmjs.com/package/name",
      "score": 0.0
    }
  ],
  "pypi": [
    {
      "name": "string",
      "description": "string",
      "version": "string",
      "url": "https://pypi.org/project/name/"
    }
  ],
  "novelty_score": 0.0,
  "recommendation": "string",
  "scan_duration": 0
}
```

## Decision Logic

| Score | Recommendation | Agent Action |
|-------|---------------|-------------|
| >= 0.8 | `PROCEED` | Build the product. No significant competition. |
| >= 0.6 | `PROCEED_WITH_CAUTION` | Build it, but ensure clear differentiation from competitors listed in the report. |
| 0.3 - 0.6 | `SKIP` | Do not build. Significant competition exists. Consider pivoting the idea. |
| < 0.3 | `SKIP` | Do not build. Market is saturated. The problem is well-solved by existing tools. |

## Pipeline Position

market-scanner is the second stage in the factory pipeline, running after data gathering and before spec generation:

```
gather (hourly) --> market-scanner (5 AM) --> analyze (6 AM) --> build (6:30 AM) --> mirror (11 PM)
  Scrape ideas       Check for competition    Generate specs      Build with Claude     Push to GitHub
```

Items that fail the novelty check are never analyzed or built, saving substantial compute time.

## Tips for Agent Callers

1. **Always provide a problem description.** Name-only scans work but produce less accurate novelty scores because keyword-based queries cannot run.
2. **Use JSON output for programmatic access.** The human-readable report is designed for terminal display, not parsing.
3. **Check the `github` array for direct competitors.** The novelty score is a summary; the raw results contain URLs and star counts for deeper analysis.
4. **The score is conservative.** A 0.6 does not mean 60% of the market is taken -- it means enough signal was found across registries to warrant caution.
5. **Scan duration is typically 1-5 seconds.** All three registries are searched concurrently.
6. **GitHub rate limits matter.** Set `GITHUB_TOKEN` environment variable for 30 req/min instead of 10 req/min unauthenticated.

## In-Cluster Access

When running inside the factory K8s cluster:
- Service name: `market-scanner` in namespace `factory`
- Port: `8090`
- Health check: `GET /health`
- The scan-queue CronJob shares the `factory-data` PVC at `/data`
