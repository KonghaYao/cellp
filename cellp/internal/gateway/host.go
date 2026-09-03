package gateway

import (
	"net"
	"net/http"
	"strconv"
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

// clientAuthorityForIngress is the authority celld should treat as the public Host
// (X-Forwarded-Host), including the outward gateway port when non-default.
func clientAuthorityForIngress(r *http.Request, publicProto string, gatewayPort int) string {
	auth := clientAuthority(r)
	if auth == "" {
		return auth
	}
	if h, p, err := net.SplitHostPort(auth); err == nil && p != "" {
		return auth
	} else if err == nil {
		auth = h
	}
	return appendGatewayPortIfNeeded(auth, publicProto, gatewayPort)
}

func appendGatewayPortIfNeeded(host, scheme string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 {
		return host
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "http" && port == 80 {
		return host
	}
	if scheme == "https" && port == 443 {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// Dev Gateway listens on :8787 without TLS; never advertise https on that port.
func schemeForDevGatewayPort(scheme string, gatewayPort int) string {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "https" && gatewayPort == 8787 {
		return "http"
	}
	return scheme
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

// publicSchemeForRequest prefers the actual client TLS when Gateway serves HTTPS directly.
func (g *Gateway) publicSchemeForRequest(r *http.Request, role string) string {
	if r != nil && r.TLS != nil {
		return "https"
	}
	scheme := g.publicSchemeForRole(role)
	return schemeForDevGatewayPort(scheme, g.cfg.GatewayPort)
}
