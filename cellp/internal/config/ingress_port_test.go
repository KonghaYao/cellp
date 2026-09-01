package config

import "testing"

func TestProdURLLoopbackPort(t *testing.T) {
	cfg := Config{
		Ingress: IngressConfig{PublicSchemeProd: "https"},
	}
	port := 19081
	got := cfg.ProdURL("demo", &port)
	want := "http://127.0.0.1:19081/"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatPreviewURLLoopback(t *testing.T) {
	cfg := Config{}
	port := 19082
	got := cfg.FormatPreviewURL("", &port)
	want := "http://127.0.0.1:19082/"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
