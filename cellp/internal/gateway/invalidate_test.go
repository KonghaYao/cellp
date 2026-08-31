package gateway_test

import (
	"context"
	"testing"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/registry"
)

func TestInvalidateProdCache(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/inv.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", "v1")

	gw := gateway.New(store)
	if _, ok := gw.LookupProdVersion(ctx, "demo"); !ok {
		t.Fatal("expected prod")
	}
	gw.InvalidateProd("demo")
	gw.InvalidateRoute("demo", "v1")
}
