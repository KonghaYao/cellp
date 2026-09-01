package gateway_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/registry"
	"github.com/google/uuid"
)

func TestHostIngressPathPreserved(t *testing.T) {
	var gotPath, gotHost, gotXFH, gotXFProto string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHost = r.Host
		gotXFH = r.Header.Get("X-Forwarded-Host")
		gotXFProto = r.Header.Get("X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	store, err := registry.Open(t.TempDir() + "/gw-host.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	parsed, _ := url.Parse(upstream.URL)
	port := 0
	fmt.Sscanf(parsed.Port(), "%d", &port)

	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: parsed.Hostname(), UpstreamPort: port,
	})

	host := "v1.demo.ingress.local"
	synthetic := "synthetic.v1.demo.ingress.local"
	_ = store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: uuid.NewString(), ProjectID: "demo", VersionID: strPtr("v1"),
		Role: registry.IngressRolePreview, Host: &host, SyntheticHost: synthetic, Active: true,
	})

	cfg := gateway.ConfigFromEnv()
	cfg.HostOnly = true
	gw := gateway.NewWithConfig(store, cfg)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/widgets?q=1", nil)
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
	if gotPath != "/api/widgets" {
		t.Fatalf("path = %q want /api/widgets", gotPath)
	}
	if gotHost != synthetic {
		t.Fatalf("upstream Host = %q want %q", gotHost, synthetic)
	}
	if gotXFH != host {
		t.Fatalf("X-Forwarded-Host = %q want %q", gotXFH, host)
	}
	if gotXFProto != "http" {
		t.Fatalf("X-Forwarded-Proto = %q want http", gotXFProto)
	}
}

func TestHostIngressUnknown404(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/gw-unknown.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := gateway.ConfigFromEnv()
	cfg.HostOnly = true
	gw := gateway.NewWithConfig(store, cfg)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	req.Host = "missing.ingress.local"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "" {
		t.Fatal("expected body")
	}
}

func strPtr(s string) *string { return &s }
