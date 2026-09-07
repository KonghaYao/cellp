package metrics

import (
	"fmt"
	"sync/atomic"
)

var (
	gatewayRequests       atomic.Uint64
	gatewayErrors5xx      atomic.Uint64
	gatewayUpstream4xx    atomic.Uint64
	gatewayUpstream5xx    atomic.Uint64
	activatorAllowed      atomic.Uint64
	activatorQueueFull    atomic.Uint64
	activatorTimeout      atomic.Uint64
	activatorCapacity     atomic.Uint64
	activatorRetry        atomic.Uint64
	activatorArchived     atomic.Uint64
	activatorControlError atomic.Uint64
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

// RecordActivatorResult records one bounded, low-cardinality activation outcome.
func RecordActivatorResult(reason string, allowed bool) {
	if allowed {
		activatorAllowed.Add(1)
		return
	}
	switch reason {
	case "wake_queue_full":
		activatorQueueFull.Add(1)
	case "wake_timeout":
		activatorTimeout.Add(1)
	case "capacity_exhausted":
		activatorCapacity.Add(1)
	case "wake_retry":
		activatorRetry.Add(1)
	case "version_archived":
		activatorArchived.Add(1)
	default:
		activatorControlError.Add(1)
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
	_, _ = w.Write([]byte("# HELP cellp_activator_results_total Cold activation outcomes by bounded reason.\n"))
	_, _ = w.Write([]byte("# TYPE cellp_activator_results_total counter\n"))
	for reason, value := range map[string]uint64{
		"ready": activatorAllowed.Load(), "wake_queue_full": activatorQueueFull.Load(),
		"wake_timeout": activatorTimeout.Load(), "capacity_exhausted": activatorCapacity.Load(),
		"wake_retry": activatorRetry.Load(), "version_archived": activatorArchived.Load(),
		"control_unavailable": activatorControlError.Load(),
	} {
		_, _ = fmt.Fprintf(w, "cellp_activator_results_total{reason=%q} %d\n", reason, value)
	}
}
