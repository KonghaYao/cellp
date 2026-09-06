package orch_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestReconcileCronAfterProdChangeRequiresConfiguredRoute(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	artifactsDir := t.TempDir()
	store, err := registry.Open(t.TempDir() + "/cron-route.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-new", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-new", registry.StatusReady, nil)
	bundle := filepath.Join(artifactsDir, "demo", "v-new")
	_ = os.MkdirAll(bundle, 0o755)
	_ = os.WriteFile(filepath.Join(bundle, "wrangler.json"), []byte(`{"name":"x"}`), 0o644)

	cfg := config.Config{ArtifactsDir: artifactsDir}
	o := orch.New(store, job.NewSQLiteQueue(store), branch.New(t.TempDir(), store),
		runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"), &artifact.Store{LocalDir: artifactsDir}, cfg)

	err = o.ReconcileCronAfterProdChange(ctx, "demo", "", "v-new")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected missing route error, got %v", err)
	}
}
