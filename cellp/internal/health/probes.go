package health

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// CheckResult is a single dependency probe outcome.
type CheckResult struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // ok | down | skipped
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

var probeClient = &http.Client{Timeout: 2 * time.Second}

// ProbeHTTP GETs url and returns ok when status is 2xx.
func ProbeHTTP(ctx context.Context, url string) CheckResult {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return CheckResult{Name: "http", Status: "down", Detail: err.Error()}
	}
	resp, err := probeClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return CheckResult{Name: "http", Status: "down", LatencyMs: latency, Detail: err.Error()}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return CheckResult{Name: "http", Status: "ok", LatencyMs: latency}
	}
	return CheckResult{
		Name: "http", Status: "down", LatencyMs: latency,
		Detail: resp.Status,
	}
}

// ProbeRustFS checks RustFS liveness via GET {endpoint}/health.
func ProbeRustFS(ctx context.Context, endpoint string) CheckResult {
	base := strings.TrimRight(endpoint, "/")
	r := ProbeHTTP(ctx, base+"/health")
	r.Name = "rustfs"
	return r
}
