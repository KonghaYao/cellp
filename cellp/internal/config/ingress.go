package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Ingress settings (AD-12 / INGRESS-ROUTING §6). Tier B defaults to host-only routing.
type IngressConfig struct {
	TierB               string
	BaseDomain          string
	PublicSchemePreview string
	PublicSchemeProd    string
	PreviewURLTemplate  string
	IngressPortMin      int
	IngressPortMax      int
}

func loadIngressConfig() IngressConfig {
	return IngressConfig{
		TierB:               envOr("CELLP_INGRESS_TIER_B", "host"),
		BaseDomain:          envOr("CELLP_INGRESS_BASE_DOMAIN", "ingress.local"),
		PublicSchemePreview: envOr("CELLP_PUBLIC_SCHEME_PREVIEW", "http"),
		PublicSchemeProd:    envOr("CELLP_PUBLIC_SCHEME_PROD", "https"),
		PreviewURLTemplate:  os.Getenv("CELLP_PREVIEW_URL_TEMPLATE"),
		IngressPortMin:      envInt("INGRESS_PORT_MIN", 19080),
		IngressPortMax:      envInt("INGRESS_PORT_MAX", 19999),
	}
}

// PreviewHost returns the external Host FQDN for a preview version (Tier A).
func (c Config) PreviewHost(projectID, versionID string) string {
	base := strings.ToLower(strings.TrimSpace(c.Ingress.BaseDomain))
	return dnsLabel(versionID) + "." + dnsLabel(projectID) + "." + base
}

// PreviewSyntheticHost is the Host header sent to celld upstream (R-UPSTREAM-HOST).
func (c Config) PreviewSyntheticHost(projectID, versionID string) string {
	return "synthetic." + c.PreviewHost(projectID, versionID)
}

// ProdHost returns the stable prod Host FQDN for a project.
func (c Config) ProdHost(projectID string) string {
	base := strings.ToLower(strings.TrimSpace(c.Ingress.BaseDomain))
	return dnsLabel(projectID) + "." + base
}

// ProdSyntheticHost is the upstream Host for prod traffic.
func (c Config) ProdSyntheticHost(projectID string) string {
	return "synthetic." + c.ProdHost(projectID)
}

// FormatPreviewURL builds the outward preview URL (INGRESS-ROUTING §3.4).
func (c Config) FormatPreviewURL(host string, listenPort *int) string {
	if tpl := strings.TrimSpace(c.Ingress.PreviewURLTemplate); tpl != "" {
		port := derefPort(listenPort)
		if port <= 0 {
			port = c.outwardGatewayPort()
		}
		return strings.ReplaceAll(strings.ReplaceAll(tpl, "{host}", host), "{port}", fmt.Sprintf("%d", port))
	}
	if host != "" {
		scheme := strings.TrimSpace(c.Ingress.PublicSchemePreview)
		if scheme == "" {
			scheme = "http"
		}
		return c.formatIngressHostURL(scheme, host)
	}
	if listenPort != nil && *listenPort > 0 {
		return fmt.Sprintf("http://127.0.0.1:%d/", *listenPort)
	}
	return ""
}

// ProdURL returns the outward prod URL for API responses (P0: derived from prod host).
func (c Config) ProdURL(projectID string) string {
	host := c.ProdHost(projectID)
	scheme := strings.TrimSpace(c.Ingress.PublicSchemeProd)
	if scheme == "" {
		scheme = "https"
	}
	return c.formatIngressHostURL(scheme, host)
}

// OutwardGatewayPort is the TCP port clients use to reach Gateway (from GATEWAY_URL or GATEWAY_PORT).
func (c Config) OutwardGatewayPort() int {
	return c.outwardGatewayPort()
}

func (c Config) outwardGatewayPort() int {
	if raw := strings.TrimSpace(c.GatewayURL); raw != "" {
		if u, err := url.Parse(raw); err == nil {
			if u.Port() != "" {
				if p, err := strconv.Atoi(u.Port()); err == nil && p > 0 {
					return p
				}
			}
			if u.Scheme == "https" {
				return 443
			}
			if u.Scheme == "http" {
				return 80
			}
		}
	}
	if c.GatewayPort > 0 {
		return c.GatewayPort
	}
	return 8787
}

func (c Config) formatIngressHostURL(scheme, host string) string {
	scheme = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(scheme)), ":")
	if scheme == "" {
		scheme = "http"
	}
	port := c.outwardGatewayPort()
	authority := host
	if shouldAppendGatewayPort(scheme, port) {
		authority = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return scheme + "://" + authority + "/"
}

func shouldAppendGatewayPort(scheme string, port int) bool {
	if port <= 0 {
		return false
	}
	if scheme == "http" && port == 80 {
		return false
	}
	if scheme == "https" && port == 443 {
		return false
	}
	return true
}

func derefPort(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func dnsLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "x"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "x"
	}
	return out
}
