package health

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// CelldHealthResponseOK reports whether status and body match celld
// GET /.well-known/celld/health when the node is ready ({"ok":true}).
func CelldHealthResponseOK(status int, body []byte) bool {
	if status != http.StatusOK {
		return false
	}
	var v struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return false
	}
	return v.OK
}

// ProbeCelldHTTP GETs a celld health URL and requires {"ok":true}.
func ProbeCelldHTTP(ctx context.Context, url string) CheckResult {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return CheckResult{Name: "celld", Status: "down", Detail: err.Error()}
	}
	resp, err := probeClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return CheckResult{Name: "celld", Status: "down", LatencyMs: latency, Detail: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if CelldHealthResponseOK(resp.StatusCode, body) {
		return CheckResult{Name: "celld", Status: "ok", LatencyMs: latency}
	}
	return CheckResult{
		Name: "celld", Status: "down", LatencyMs: latency,
		Detail: "celld health response not ok",
	}
}
