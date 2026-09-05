package activator

import (
	"net/http"
	"strings"
)

// WaitClass describes how a cold request may be handled.
type WaitClass int

const (
	// WaitClassNone means no activator wait path.
	WaitClassNone WaitClass = 0
	// WaitClassBounded may block until wake deadline polling for an endpoint.
	WaitClassBounded WaitClass = 1
	// WaitClassFastFail triggers ensure-capacity then immediate 503 + Retry-After.
	WaitClassFastFail WaitClass = 2
)

// ClassifyRequest maps HTTP properties to activator wait behavior (CF-ACTIVATOR).
func ClassifyRequest(r *http.Request, maxBufferedBody int64) WaitClass {
	if r == nil {
		return WaitClassFastFail
	}
	if isUpgrade(r) {
		return WaitClassFastFail
	}
	method := strings.ToUpper(r.Method)
	switch method {
	case http.MethodGet, http.MethodHead:
		return WaitClassBounded
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		if r.ContentLength < 0 {
			// Chunked or unknown length — do not buffer.
			return WaitClassFastFail
		}
		if r.ContentLength > maxBufferedBody {
			return WaitClassFastFail
		}
		return WaitClassBounded
	default:
		return WaitClassFastFail
	}
}

func isUpgrade(r *http.Request) bool {
	conn := strings.ToLower(strings.TrimSpace(r.Header.Get("Connection")))
	upgrade := strings.ToLower(strings.TrimSpace(r.Header.Get("Upgrade")))
	return strings.Contains(conn, "upgrade") && upgrade != ""
}
