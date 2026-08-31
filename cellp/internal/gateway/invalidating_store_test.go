package gateway

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestWrapStoreInvalidatesOnSetRoute(t *testing.T) {
	dir := t.TempDir()
	base, err := registry.Open(filepath.Join(dir, "inv.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })

	gw := New(base)
	store := WrapStore(base, gw)
	ctx := context.Background()

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	if err := store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	}); err != nil {
		t.Fatal(err)
	}
	prod := "v1"
	if err := store.SetProdVersionCAS(ctx, "demo", "", "v1"); err != nil {
		t.Fatal(err)
	}
	_ = prod
	if err := store.SetRouteActive(ctx, "demo", "v1", false); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRoute(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestWrapStoreNilGateway(t *testing.T) {
	dir := t.TempDir()
	base, err := registry.Open(filepath.Join(dir, "inv2.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })
	if WrapStore(base, nil) != base {
		t.Fatal("expected base store")
	}
}
