package orch_test

import (
	"context"
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

func TestApplyWorkerEnvNotReady(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "env.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusDeploying, nil)

	cfg := config.Config{ArtifactsDir: dir}
	o := orch.New(store, job.NewSQLiteQueue(store), branch.New(dir+"/off", store),
		runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"), &artifact.Store{LocalDir: dir}, cfg)

	if err := o.ApplyWorkerEnv(ctx, "demo", "v1", map[string]string{"GREETING": "hi"}); err != nil {
		t.Fatal(err)
	}
	env, err := store.GetVersionEnv(ctx, "demo", "v1")
	if err != nil || env["GREETING"] != "hi" {
		t.Fatalf("env=%v err=%v", env, err)
	}

	if err := o.ApplyWorkerEnv(ctx, "demo", "missing", map[string]string{}); err == nil {
		t.Fatal("expected not found")
	}
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusDestroyed, nil)
	if err := o.ApplyWorkerEnv(ctx, "demo", "v1", map[string]string{}); err == nil {
		t.Fatal("expected destroyed")
	}
}
