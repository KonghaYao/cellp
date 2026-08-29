package metrics

import (
	"fmt"
	"sync/atomic"
)

var (
	gatewayRequests   atomic.Uint64
	gatewayErrors5xx  atomic.Uint64
	gatewayUpstream4xx atomic.Uint64
	gatewayUpstream5xx atomic.Uint64
)

// RecordGatewayRequest increments gateway request counters by HTTP status class.
func RecordGatewayRequest(status int) {
	gatewayRequests.Add(1)
	if status >= 500 {
		gatewayErrors5xx.Add(1)
	}
}

// RecordGatewayUpstream records upstream response status from reverse proxy.
func RecordGatewayUpstream(status int) {
	if status >= 400 && status < 500 {
		gatewayUpstream4xx.Add(1)
	}
	if status >= 500 {
		gatewayUpstream5xx.Add(1)
	}
}

func writeGatewayMetrics(w interface{ Write([]byte) (int, error) }) {
	_, _ = w.Write([]byte("# HELP cellp_gateway_requests_total Gateway requests served.\n"))
	_, _ = w.Write([]byte("# TYPE cellp_gateway_requests_total counter\n"))
	_, _ = fmt.Fprintf(w, "cellp_gateway_requests_total %d\n", gatewayRequests.Load())
	_, _ = w.Write([]byte("# HELP cellp_gateway_errors_5xx Gateway responses with status >= 500.\n"))
	_, _ = w.Write([]byte("# TYPE cellp_gateway_errors_5xx counter\n"))
	_, _ = fmt.Fprintf(w, "cellp_gateway_errors_5xx %d\n", gatewayErrors5xx.Load())
	_, _ = w.Write([]byte("# HELP cellp_gateway_upstream_4xx Upstream 4xx responses proxied.\n"))
	_, _ = w.Write([]byte("# TYPE cellp_gateway_upstream_4xx counter\n"))
	_, _ = fmt.Fprintf(w, "cellp_gateway_upstream_4xx %d\n", gatewayUpstream4xx.Load())
	_, _ = w.Write([]byte("# HELP cellp_gateway_upstream_5xx Upstream 5xx responses proxied.\n"))
	_, _ = w.Write([]byte("# TYPE cellp_gateway_upstream_5xx counter\n"))
	_, _ = fmt.Fprintf(w, "cellp_gateway_upstream_5xx %d\n", gatewayUpstream5xx.Load())
}
