package config

import "testing"

func TestFormatIngressHostURLIncludesGatewayPort(t *testing.T) {
	t.Setenv("CELLP_INGRESS_BASE_DOMAIN", "ingress.local")
	t.Setenv("CELLP_PUBLIC_SCHEME_PREVIEW", "http")
	t.Setenv("CELLP_PUBLIC_SCHEME_PROD", "http")
	t.Setenv("GATEWAY_URL", "http://127.0.0.1:8787")
	cfg := Load()

	got := cfg.FormatPreviewURL(cfg.PreviewHost("demo-app", "v1"), nil)
	want := "http://v1.demo-app.ingress.local:8787/"
	if got != want {
		t.Fatalf("preview got %q want %q", got, want)
	}

	gotProd := cfg.ProdURL("demo-app", nil)
	wantProd := "http://demo-app.ingress.local:8787/"
	if gotProd != wantProd {
		t.Fatalf("prod got %q want %q", gotProd, wantProd)
	}
}

func TestFormatIngressHostURLHTTPSDevPort(t *testing.T) {
	t.Setenv("CELLP_INGRESS_BASE_DOMAIN", "ingress.local")
	t.Setenv("CELLP_PUBLIC_SCHEME_PREVIEW", "https")
	t.Setenv("CELLP_PUBLIC_SCHEME_PROD", "https")
	t.Setenv("GATEWAY_URL", "https://127.0.0.1:8788")
	t.Setenv("GATEWAY_TLS_PORT", "8788")
	cfg := Load()

	got := cfg.FormatPreviewURL(cfg.PreviewHost("demo-app", "v1"), nil)
	want := "https://v1.demo-app.ingress.local:8788/"
	if got != want {
		t.Fatalf("preview got %q want %q", got, want)
	}
}

func TestFormatIngressHostURLOmitsDefaultHTTPPort(t *testing.T) {
	t.Setenv("GATEWAY_URL", "http://127.0.0.1:80")
	t.Setenv("GATEWAY_PORT", "80")
	t.Setenv("CELLP_PUBLIC_SCHEME_PROD", "http")
	cfg := Load()
	got := cfg.ProdURL("demo-app", nil)
	want := "http://demo-app.ingress.local/"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
