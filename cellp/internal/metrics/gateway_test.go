package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/metrics"
)

func TestGatewayMetricsRecorded(t *testing.T) {
	metrics.RecordGatewayRequest(http.StatusOK)
	metrics.RecordGatewayUpstream(http.StatusBadGateway)

	w := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := w.Body.String()
	for _, want := range []string{
		"cellp_gateway_requests_total",
		"cellp_gateway_upstream_5xx",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in metrics", want)
		}
	}
}
