package gateway

import (
	"net"
	"net/http"
	"strings"

	"github.com/cellp/cellp/internal/registry"
)

func normalizeHost(host string) string {
	return registry.NormalizeIngressHost(host)
}

func clientAuthority(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return host
}

func (g *Gateway) effectiveHost(r *http.Request) string {
	h := r.Host
	if g.cfg.TrustForwardedHeaders && g.trustedProxy(r.RemoteAddr) {
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
			parts := strings.Split(xfh, ",")
			h = strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return normalizeHost(h)
}

func (g *Gateway) trustedProxy(remoteAddr string) bool {
	if len(g.cfg.TrustedProxyCIDRs) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range g.cfg.TrustedProxyCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (g *Gateway) publicSchemeForRole(role string) string {
	if role == registry.IngressRoleProd {
		return g.cfg.PublicSchemeProd
	}
	return g.cfg.PublicSchemePreview
}
