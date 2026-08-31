package gateway_test

import (
	"context"
	"testing"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/registry"
)

func TestLookupRouteCachesMiss(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/miss.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	gw := gateway.New(store)
	if _, ok := gw.LookupRoute(ctx, "demo", "v-missing"); ok {
		t.Fatal("expected miss")
	}
	if _, ok := gw.LookupRoute(ctx, "demo", "v-missing"); ok {
		t.Fatal("expected cached miss")
	}
}
