package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestReconcileFleetUnhealthyRoute(t *testing.T) {
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
		UpstreamHost: "127.0.0.1", UpstreamPort: 1,
	})

	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	m := New(8800, "", "us-east-1", "s3://cellp-celld", "k", "s")
	started, skipped, err := ReconcileFleet(ctx, store, m)
	if err != nil {
		t.Fatal(err)
	}
	if started != 0 || skipped != 0 {
		t.Fatalf("started=%d skipped=%d", started, skipped)
	}
}
