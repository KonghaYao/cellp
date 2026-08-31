package orch_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestPromoteSuccessNoCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	artifactsDir := t.TempDir()
	store, err := registry.Open(t.TempDir() + "/promote.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.Config{GatewayURL: "http://127.0.0.1:8787", ArtifactsDir: artifactsDir}
	o := orch.New(store, job.NewSQLiteQueue(store), branch.New(t.TempDir(), store),
		runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"), &artifact.Store{LocalDir: artifactsDir}, cfg)

	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-old", ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-new", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-old", registry.StatusReady, nil)
	_ = store.UpdateVersionStatus(ctx, "demo", "v-new", registry.StatusReady, nil)
	_ = store.SetProdVersionCAS(ctx, "demo", "", "v-old")
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-old", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-new", Active: false,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8793,
	})
	artDir := filepath.Join(artifactsDir, "demo", "v-new")
	_ = os.MkdirAll(artDir, 0o755)
	_ = os.WriteFile(filepath.Join(artDir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644)

	if err := o.Promote(ctx, "demo", "v-new"); err != nil {
		t.Fatal(err)
	}
	p, _ := store.GetProject(ctx, "demo")
	if p == nil || p.ProdVersionID == nil || *p.ProdVersionID != "v-new" {
		t.Fatalf("prod=%v", p)
	}
}
