package gateway

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/cellp/cellp/internal/registry"
)

// applyUpstreamHeaders sets synthetic Host and forwarded client metadata.
// WebSocket upgrade headers are hop-by-hop; do not Del Sec-WebSocket-* (WEBSOCKET-INGRESS-DESIGN §4.2).
func applyUpstreamHeaders(r *http.Request, binding *registry.IngressBinding, clientAuth, publicProto string) {
	if binding == nil || r == nil {
		return
	}
	r.Host = binding.SyntheticHost
	r.Header.Set("Host", binding.SyntheticHost)

	r.Header.Del("X-Forwarded-Host")
	r.Header.Del("X-Forwarded-Proto")
	r.Header.Del("X-Forwarded-For")
	r.Header.Del("Forwarded")

	if clientAuth != "" {
		r.Header.Set("X-Forwarded-Host", clientAuth)
		if port := forwardedPort(clientAuth); port != "" {
			r.Header.Set("X-Forwarded-Port", port)
		}
	}
	if publicProto != "" {
		r.Header.Set("X-Forwarded-Proto", publicProto)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		r.Header.Set("X-Forwarded-For", host)
	}

	if clientAuth != "" && publicProto != "" {
		fwdHost := quoteForwardedHost(clientAuth)
		r.Header.Set("Forwarded", fmt.Sprintf("host=%s;proto=%q", fwdHost, publicProto))
	}
}

func forwardedPort(authority string) string {
	authority = strings.TrimSpace(authority)
	if authority == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(authority); err == nil && port != "" {
		return port
	}
	return ""
}

func quoteForwardedHost(authority string) string {
	if authority == "" {
		return `""`
	}
	if h, p, err := net.SplitHostPort(authority); err == nil {
		if p != "" {
			return fmt.Sprintf("%q:%s", h, p)
		}
		return fmt.Sprintf("%q", h)
	}
	return fmt.Sprintf("%q", authority)
}
