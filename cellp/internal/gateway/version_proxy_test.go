package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestVersionRouteProxyHost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("preview-body"))
	}))
	defer upstream.Close()

	store, err := registry.Open(t.TempDir() + "/gw-ver.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	host, port := upstreamHostPort(t, upstream.URL)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	})
	previewHost := "v1.demo.ingress.local"
	upsertPreviewBinding(t, store, "demo", "v1", previewHost, "syn.v1.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp := doHostGet(t, srv.URL, previewHost, "/")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "preview-body" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
}
