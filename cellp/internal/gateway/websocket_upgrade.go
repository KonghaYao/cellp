package gateway

import (
	"net"
	"net/http"
	"strings"
)

// isUpgradeRequest detects RFC6455 WebSocket upgrade per WEBSOCKET-INGRESS-DESIGN §4.2.3.
func isUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	up := strings.ToLower(strings.TrimSpace(r.Header.Get("Upgrade")))
	if !strings.Contains(up, "websocket") {
		return false
	}
	conn := r.Header.Get("Connection")
	for _, part := range strings.Split(conn, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
			return true
		}
	}
	return false
}

// classifyProxyError buckets ReverseProxy failures for Upgrade structured logs (P-4).
func classifyProxyError(err error) string {
	if err == nil {
		return "other"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "hijack") {
		return "hijack"
	}
	if _, ok := err.(*net.OpError); ok {
		return "dial"
	}
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "dial") ||
		strings.Contains(msg, "i/o timeout") {
		return "dial"
	}
	return "other"
}
