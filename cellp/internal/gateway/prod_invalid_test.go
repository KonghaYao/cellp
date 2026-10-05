package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestProdPointsToDestroyedVersion(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/prod-destroyed.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-old", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-old", registry.StatusDestroyed, nil)
	_ = store.SetProdVersion(ctx, "demo", "v-old")
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-old", Active: false,
		UpstreamHost: "127.0.0.1", UpstreamPort: 9999,
	})
	upsertProdBinding(t, store, "demo", "demo.ingress.local", "syn.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()
	resp := doHostGet(t, srv.URL, "demo.ingress.local", "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "prod_version_destroyed") {
		t.Fatalf("body = %q", body)
	}
}
