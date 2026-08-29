package gateway_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/registry"
)

type countingStore struct {
	registry.Store
	getRouteCalls   atomic.Int32
	getProjectCalls atomic.Int32
}

func (c *countingStore) GetRoute(ctx context.Context, projectID, versionID string) (*registry.Route, error) {
	c.getRouteCalls.Add(1)
	return c.Store.GetRoute(ctx, projectID, versionID)
}

func (c *countingStore) GetProject(ctx context.Context, id string) (*registry.Project, error) {
	c.getProjectCalls.Add(1)
	return c.Store.GetProject(ctx, id)
}

func setupCountingGateway(t *testing.T) (*gateway.Gateway, *countingStore, func()) {
	t.Helper()
	base, err := registry.Open(t.TempDir() + "/cache.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingStore{Store: base}
	gw := gateway.New(counting)
	return gw, counting, func() { base.Close() }
}

func TestRouteCacheHitMiss(t *testing.T) {
	gw, counting, cleanup := setupCountingGateway(t)
	defer cleanup()

	ctx := context.Background()
	store := counting.Store
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})

	route1, ok := gw.LookupRoute(ctx, "demo", "v1")
	if !ok || route1 == nil {
		t.Fatal("expected route on first lookup")
	}
	if counting.getRouteCalls.Load() != 1 {
		t.Fatalf("first lookup getRouteCalls = %d, want 1", counting.getRouteCalls.Load())
	}

	route2, ok := gw.LookupRoute(ctx, "demo", "v1")
	if !ok || route2 == nil {
		t.Fatal("expected route on cached lookup")
	}
	if counting.getRouteCalls.Load() != 1 {
		t.Fatalf("cached lookup getRouteCalls = %d, want 1", counting.getRouteCalls.Load())
	}
	if route1.UpstreamPort != route2.UpstreamPort {
		t.Fatalf("cached route mismatch: %+v vs %+v", route1, route2)
	}
}

func TestRouteCacheInvalidation(t *testing.T) {
	gw, counting, cleanup := setupCountingGateway(t)
	defer cleanup()

	ctx := context.Background()
	store := counting.Store
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})

	if _, ok := gw.LookupRoute(ctx, "demo", "v1"); !ok {
		t.Fatal("expected route")
	}
	if _, ok := gw.LookupRoute(ctx, "demo", "v1"); !ok {
		t.Fatal("expected cached route")
	}
	if counting.getRouteCalls.Load() != 1 {
		t.Fatalf("before invalidation getRouteCalls = %d, want 1", counting.getRouteCalls.Load())
	}

	gw.InvalidateRoute("demo", "v1")

	if _, ok := gw.LookupRoute(ctx, "demo", "v1"); !ok {
		t.Fatal("expected route after invalidation")
	}
	if counting.getRouteCalls.Load() != 2 {
		t.Fatalf("after invalidation getRouteCalls = %d, want 2", counting.getRouteCalls.Load())
	}
}

func TestProdCacheHitMissAndInvalidation(t *testing.T) {
	gw, counting, cleanup := setupCountingGateway(t)
	defer cleanup()

	ctx := context.Background()
	store := counting.Store
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", "v1")

	version1, ok := gw.LookupProdVersion(ctx, "demo")
	if !ok || version1 != "v1" {
		t.Fatalf("first prod lookup = %q ok=%v", version1, ok)
	}
	if counting.getProjectCalls.Load() != 1 {
		t.Fatalf("first prod lookup getProjectCalls = %d, want 1", counting.getProjectCalls.Load())
	}

	version2, ok := gw.LookupProdVersion(ctx, "demo")
	if !ok || version2 != "v1" {
		t.Fatalf("cached prod lookup = %q ok=%v", version2, ok)
	}
	if counting.getProjectCalls.Load() != 1 {
		t.Fatalf("cached prod lookup getProjectCalls = %d, want 1", counting.getProjectCalls.Load())
	}

	gw.InvalidateProd("demo")

	version3, ok := gw.LookupProdVersion(ctx, "demo")
	if !ok || version3 != "v1" {
		t.Fatalf("prod lookup after invalidation = %q ok=%v", version3, ok)
	}
	if counting.getProjectCalls.Load() != 2 {
		t.Fatalf("after invalidation getProjectCalls = %d, want 2", counting.getProjectCalls.Load())
	}
}

func TestWrapStoreInvalidatesOnSetRoute(t *testing.T) {
	base, err := registry.Open(t.TempDir() + "/wrap.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()

	counting := &countingStore{Store: base}
	gw := gateway.New(counting)
	store := gateway.WrapStore(counting, gw)

	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})

	if _, ok := gw.LookupRoute(ctx, "demo", "v1"); !ok {
		t.Fatal("expected route")
	}
	if _, ok := gw.LookupRoute(ctx, "demo", "v1"); !ok {
		t.Fatal("expected cached route")
	}
	if counting.getRouteCalls.Load() != 1 {
		t.Fatalf("before update getRouteCalls = %d, want 1", counting.getRouteCalls.Load())
	}

	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8793,
	})

	route, ok := gw.LookupRoute(ctx, "demo", "v1")
	if !ok || route == nil {
		t.Fatal("expected route after write invalidation")
	}
	if route.UpstreamPort != 8793 {
		t.Fatalf("route port = %d, want 8793", route.UpstreamPort)
	}
	if counting.getRouteCalls.Load() != 2 {
		t.Fatalf("after update getRouteCalls = %d, want 2", counting.getRouteCalls.Load())
	}
}

func TestRouteCacheTTLExpiry(t *testing.T) {
	base, err := registry.Open(t.TempDir() + "/ttl.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()

	counting := &countingStore{Store: base}
	gw := gateway.New(counting)
	cache := gw.RouteCacheForTest()
	cache.SetTTLForTest(20 * time.Millisecond)

	ctx := context.Background()
	_, _ = base.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = base.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = base.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})

	if _, ok := gw.LookupRoute(ctx, "demo", "v1"); !ok {
		t.Fatal("expected route")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := gw.LookupRoute(ctx, "demo", "v1"); !ok {
		t.Fatal("expected route after ttl expiry")
	}
	if counting.getRouteCalls.Load() != 2 {
		t.Fatalf("after ttl expiry getRouteCalls = %d, want 2", counting.getRouteCalls.Load())
	}
}
