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
)

func TestVersionRouteProxy(t *testing.T) {
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

	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	host := parsed.Hostname()
	port := 0
	if parsed.Port() != "" {
		fmt.Sscanf(parsed.Port(), "%d", &port)
	}
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	})

	gw := gateway.New(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/demo/v1/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "preview-body" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
}
