package gateway

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// GatewayConfig holds AD-12 ingress / proxy settings (P0: Tier B = host).
type GatewayConfig struct {
	IngressTierB              string
	HostOnly                  bool
	GatewayPort               int
	TrustForwardedHeaders     bool
	TrustedProxyCIDRs         []*net.IPNet
	PublicSchemePreview       string
	PublicSchemeProd          string
	GatewayID                 string
}

// ConfigFromEnv loads gateway settings from the environment.
// AD-12 P3: Host-only ingress is the default; path routing was removed.
func ConfigFromEnv() GatewayConfig {
	cfg := GatewayConfig{
		IngressTierB:        envOr("CELLP_INGRESS_TIER_B", "host"),
		HostOnly:            true,
		PublicSchemePreview: envOr("CELLP_PUBLIC_SCHEME_PREVIEW", "http"),
		PublicSchemeProd:    envOr("CELLP_PUBLIC_SCHEME_PROD", "https"),
		GatewayPort:         envIntOr("GATEWAY_PORT", 8787),
		GatewayID:           envOr("CELLPD_INSTANCE_ID", ""),
	}
	if os.Getenv("GATEWAY_TRUST_FORWARDED_HEADERS") == "1" {
		cfg.TrustForwardedHeaders = true
	}
	for _, cidr := range strings.Split(os.Getenv("GATEWAY_TRUSTED_PROXY_CIDRS"), ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			cfg.TrustedProxyCIDRs = append(cfg.TrustedProxyCIDRs, n)
		}
	}
	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
