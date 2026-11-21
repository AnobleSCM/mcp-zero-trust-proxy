package e2e

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/AnobleSCM/mcp-zero-trust-proxy/internal/config"
	"github.com/AnobleSCM/mcp-zero-trust-proxy/internal/proxy"
	"github.com/AnobleSCM/mcp-zero-trust-proxy/internal/ratelimit"
	"github.com/AnobleSCM/mcp-zero-trust-proxy/internal/rbac"
)

// newBenchProxy starts the full proxy pipeline (auth, rate limit, RBAC, audit)
// in front of the local mock upstream. The rate limit is set high enough that
// a benchmark never gets a 429.
func newBenchProxy(b *testing.B) *httptest.Server {
	b.Helper()
	mock := mockMCPServer(0)
	b.Cleanup(mock.Close)

	handler, err := proxy.NewHandler(&config.Config{Server: config.ServerConfig{UpstreamURL: mock.URL}})
	if err != nil {
		b.Fatalf("create handler: %v", err)
	}
	// The default transport keeps only 2 idle connections to the upstream, so a
	// many-client benchmark would open a new connection per request and run out of
	// local ports. Keep enough idle connections to measure the request path itself.
	handler.SetTransport(&http.Transport{MaxIdleConnsPerHost: 64})
	roles := []config.RoleConfig{{Name: "admin", AllowedTools: []string{}, DenyTools: []string{}}}
	limiter := ratelimit.NewLimiter(&config.RateLimitConfig{RequestsPerMinute: 600_000_000, BurstSize: 1 << 30})
	pipeline := proxy.NewPipelineForTest(handler, &multiTokenAuth{}, limiter, rbac.NewEngine(roles), &discardAudit{})
	server := httptest.NewServer(pipeline)
	b.Cleanup(server.Close)
	return server
}

// discardAudit drops entries so the benchmark does not grow memory.
type discardAudit struct{}

func (discardAudit) Log(proxy.AuditEntry) {}

// benchToolsList sends one tools/list request and fails the benchmark unless it returns 200.
func benchToolsList(b *testing.B, client *http.Client, url string, body []byte) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		b.Error(err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenKai)
	resp, err := client.Do(req)
	if err != nil {
		b.Error(err)
		return
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b.Errorf("status %d, want 200", resp.StatusCode)
	}
}

// BenchmarkProxyToolsList measures one client sending tools/list requests one at a
// time through the proxy to a local mock upstream, and reports the p50 and p95 latency.
func BenchmarkProxyToolsList(b *testing.B) {
	server := newBenchProxy(b)
	body := jsonRPCBody("tools/list", nil)
	client := &http.Client{Timeout: 10 * time.Second}
	lat := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		benchToolsList(b, client, server.URL, body)
		lat = append(lat, time.Since(start))
	}
	b.StopTimer()

	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	b.ReportMetric(float64(lat[len(lat)*50/100].Nanoseconds()), "p50-ns")
	b.ReportMetric(float64(lat[len(lat)*95/100].Nanoseconds()), "p95-ns")
}

// BenchmarkProxyToolsListParallel measures throughput with many clients at once.
func BenchmarkProxyToolsListParallel(b *testing.B) {
	server := newBenchProxy(b)
	body := jsonRPCBody("tools/list", nil)
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 64}}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchToolsList(b, client, server.URL, body)
		}
	})
}
