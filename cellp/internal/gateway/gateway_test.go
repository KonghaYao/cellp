package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/registry"
)

func setupGateway(t *testing.T) (*gateway.Gateway, *registry.SQLiteStore, func()) {
	t.Helper()
	store, err := registry.Open(t.TempDir() + "/gw.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 9999,
	})
	return hostOnlyGW(store), store, func() { store.Close() }
}

func TestHealth(t *testing.T) {
	gw, store, cleanup := setupGateway(t)
	defer cleanup()
	_ = store
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestArchivedVersion503(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/gw-archived.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusArchived, nil)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: false,
		UpstreamHost: "127.0.0.1", UpstreamPort: 9999,
	})
	host := "v1.demo.ingress.local"
	upsertPreviewBinding(t, store, "demo", "v1", host, "syn.v1.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()
	resp := doHostGet(t, srv.URL, host, "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "version_archived") {
		t.Fatalf("body = %q", body)
	}
}

func TestInactiveRoute503(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	store, err := registry.Open(t.TempDir() + "/gw2.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: false,
		UpstreamHost: "127.0.0.1", UpstreamPort: 9999,
	})
	host := "v1.demo.ingress.local"
	upsertPreviewBinding(t, store, "demo", "v1", host, "syn.v1.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp := doHostGet(t, srv.URL, host, "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
}

func TestProdRoute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("prod-body"))
	}))
	defer upstream.Close()

	store, err := registry.Open(t.TempDir() + "/gw3.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", "v1")

	host, port := upstreamHostPort(t, upstream.URL)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	})
	prodHost := "demo.ingress.local"
	upsertProdBinding(t, store, "demo", prodHost, "syn.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp := doHostGet(t, srv.URL, prodHost, "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("prod route status = %d", resp.StatusCode)
	}
}

func TestPathRoutingRemoved404(t *testing.T) {
	gw, _, cleanup := setupGateway(t)
	defer cleanup()
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/demo/v1/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("path route should be gone, status=%d", resp.StatusCode)
	}
}
