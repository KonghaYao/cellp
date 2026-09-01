package config

import (
	"fmt"
	"os"
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
		return strings.ReplaceAll(strings.ReplaceAll(tpl, "{host}", host), "{port}", fmt.Sprintf("%d", derefPort(listenPort)))
	}
	if host != "" {
		scheme := strings.TrimSpace(c.Ingress.PublicSchemePreview)
		if scheme == "" {
			scheme = "http"
		}
		return scheme + "://" + host + "/"
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
	return scheme + "://" + host + "/"
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
