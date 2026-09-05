package orch

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestMaybeEnterDeployReady(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "lifecycle.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := config.Config{ArtifactsDir: dir}
	o := New(store, job.NewSQLiteQueue(store), branch.New(dir+"/off", store),
		runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"),
		&artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}, cfg)

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusDeploying)

	t.Setenv(contract.EnvElasticRuntime, "")
	if err := o.maybeEnterDeployReady(ctx, j); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v != nil && v.Status == registry.StatusDeployReady {
		t.Fatal("flag off should not set deploy_ready")
	}

	t.Setenv(contract.EnvElasticRuntime, "1")
	_ = store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	})
	if err := o.maybeEnterDeployReady(ctx, j); err != nil {
		t.Fatal(err)
	}
	v, _ = store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusDeployReady {
		t.Fatalf("want deploy_ready, got %+v", v)
	}
}
