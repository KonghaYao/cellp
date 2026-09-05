package activator

import (
	"net/http"
	"strconv"
)

// AdmitResult is the gateway decision for a cold deploy_ready request.
type AdmitResult struct {
	AllowProxy    bool
	Reason        string
	RetryAfterSec int
}

// WriteRetryResponse emits 503 with Retry-After and X-Cellp-Reason.
func WriteRetryResponse(w http.ResponseWriter, res AdmitResult) {
	if res.RetryAfterSec <= 0 {
		res.RetryAfterSec = 1
	}
	if res.Reason == "" {
		res.Reason = ReasonWakeRetry
	}
	w.Header().Set("Retry-After", strconv.Itoa(res.RetryAfterSec))
	w.Header().Set(HeaderCellpReason, res.Reason)
	http.Error(w, res.Reason, http.StatusServiceUnavailable)
}
