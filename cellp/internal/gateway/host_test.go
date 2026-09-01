package gateway

import (
	"net"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestNormalizeHost(t *testing.T) {
	if got := normalizeHost("V1.Demo.Ingress.Local:8787"); got != "v1.demo.ingress.local" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeHost("  "); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestEffectiveHostTrustForwarded(t *testing.T) {
	cfg := GatewayConfig{
		TrustForwardedHeaders: true,
		TrustedProxyCIDRs:     mustParseCIDR(t, "127.0.0.0/8"),
	}
	gw := NewWithConfig(nil, cfg)

	r := httptest.NewRequest("GET", "http://gateway/", nil)
	r.Host = "public.example"
	r.Header.Set("X-Forwarded-Host", "preview.ingress.local")
	r.RemoteAddr = "127.0.0.1:12345"

	if got := gw.effectiveHost(r); got != "preview.ingress.local" {
		t.Fatalf("got %q", got)
	}
}

func TestEffectiveHostTrustOff(t *testing.T) {
	cfg := GatewayConfig{TrustForwardedHeaders: false}
	gw := NewWithConfig(nil, cfg)

	r := httptest.NewRequest("GET", "http://gateway/", nil)
	r.Host = "public.example"
	r.Header.Set("X-Forwarded-Host", "spoof.ingress.local")

	if got := gw.effectiveHost(r); got != "public.example" {
		t.Fatalf("got %q", got)
	}
}

func TestPublicSchemeForRole(t *testing.T) {
	gw := NewWithConfig(nil, GatewayConfig{
		PublicSchemePreview: "http",
		PublicSchemeProd:    "https",
	})
	if gw.publicSchemeForRole(registry.IngressRolePreview) != "http" {
		t.Fatal("preview scheme")
	}
	if gw.publicSchemeForRole(registry.IngressRoleProd) != "https" {
		t.Fatal("prod scheme")
	}
}

func mustParseCIDR(t *testing.T, s string) []*net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return []*net.IPNet{n}
}
