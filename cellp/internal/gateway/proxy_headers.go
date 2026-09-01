package gateway

import (
	"fmt"
	"net"
	"net/http"

	"github.com/cellp/cellp/internal/registry"
)

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
