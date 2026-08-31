package runtime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestLoadReconcileConfig(t *testing.T) {
	t.Setenv("CELLP_FLEET_RECONCILE_INTERVAL", "0")
	cfg := LoadReconcileConfig()
	if cfg.Background {
		t.Fatal("expected boot only")
	}
	t.Setenv("CELLP_FLEET_RECONCILE_INTERVAL", "5m")
	cfg = LoadReconcileConfig()
	if cfg.Interval != 5*time.Minute {
		t.Fatalf("interval %v", cfg.Interval)
	}
	t.Setenv("CELLP_FLEET_RECONCILE_INTERVAL", "not-a-duration")
	cfg = LoadReconcileConfig()
	if cfg.Interval != defaultReconcileInterval {
		t.Fatalf("bad duration interval %v", cfg.Interval)
	}
}

func TestReconcileFleetHealthyRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "fleet.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: port,
	})

	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	m := New(8800, "", "us-east-1", "s3://cellp-celld", "k", "s")
	started, skipped, err := ReconcileFleet(ctx, store, m)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 || started != 0 {
		t.Fatalf("started=%d skipped=%d", started, skipped)
	}
}

func TestStartReconcilerCancel(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "rec.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := ReconcileConfig{Interval: 5 * time.Millisecond, Background: true}
	StartReconciler(ctx, store, New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"), cfg)
	time.Sleep(15 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)
}
