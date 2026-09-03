package runtime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestSeedPortCollision(t *testing.T) {
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")

	if err := m.SeedPort("demo", "v1", 8803); err != nil {
		t.Fatalf("SeedPort v1: %v", err)
	}
	if err := m.SeedPort("demo", "v1", 8803); err != nil {
		t.Fatalf("SeedPort v1 again: %v", err)
	}
	if err := m.SeedPort("demo", "v2", 8803); err == nil {
		t.Fatal("expected collision when seeding same port for different version")
	}
	if err := m.SeedPort("demo", "v1", 8804); err == nil {
		t.Fatal("expected error when changing seeded port for same version")
	}
}

func TestReconcileFleetSkipsHealthyRoutes(t *testing.T) {
	bin := t.TempDir()
	stub := filepath.Join(bin, "celld")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "reconcile.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	}); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	started, skipped, err := ReconcileFleet(ctx, store, m)
	if err != nil {
		t.Fatalf("ReconcileFleet: %v", err)
	}
	if started != 0 {
		t.Fatalf("started = %d, want 0", started)
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
}

func TestLoadReconcileConfigDefaults(t *testing.T) {
	t.Setenv("CELLP_FLEET_RECONCILE_INTERVAL", "")
	cfg := LoadReconcileConfig()
	if !cfg.Background {
		t.Fatal("expected background enabled by default")
	}
	if cfg.Interval != defaultReconcileInterval {
		t.Fatalf("interval = %v, want %v", cfg.Interval, defaultReconcileInterval)
	}
}

func TestLoadReconcileConfigBootOnly(t *testing.T) {
	t.Setenv("CELLP_FLEET_RECONCILE_INTERVAL", "0")
	cfg := LoadReconcileConfig()
	if cfg.Background {
		t.Fatal("expected background disabled when interval is 0")
	}
}
