#!/usr/bin/env bash
# benchmark.sh — Measures proxy request latency and throughput using Go's built-in benchmarks.
#
# Usage:
#   ./scripts/benchmark.sh
#
# Prerequisites:
#   - Go installed (see go.mod for the minimum version)
#   - Run from the project root directory
#
# What it measures (tests/e2e/bench_test.go):
#   - BenchmarkProxyToolsList: tools/list requests, one at a time, through the full
#     pipeline (auth, rate limit, RBAC, audit) to a local mock upstream.
#     Prints ns/op plus the p50-ns and p95-ns latency of the run.
#   - BenchmarkProxyToolsListParallel: the same request from many clients at once.
#     Prints ns/op (lower is higher throughput).
#
# These are loopback round trips on your machine, so they include the HTTP client and
# the mock upstream, not only the proxy. Your numbers will differ from other machines.

set -euo pipefail

cd "$(dirname "$0")/.."

OUT="${TMPDIR:-/tmp}/mcpproxy-bench.txt"

echo "=== MCP Zero-Trust Proxy — Latency Benchmark ==="
echo ""
echo "Running Go benchmarks (3 runs each)..."
echo ""

go test ./tests/e2e/ -run='^$' -bench=. -benchmem -count=3 -timeout=10m 2>&1 | tee "$OUT"

if ! grep -q '^Benchmark' "$OUT"; then
  echo "ERROR: no benchmark results were produced" >&2
  exit 1
fi

echo ""
echo "=== Benchmark complete ==="
echo "Raw results saved to $OUT"
echo ""
echo "To compare against a baseline:"
echo "  go install golang.org/x/perf/cmd/benchstat@latest"
echo "  benchstat $OUT"
