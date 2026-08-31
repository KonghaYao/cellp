package orch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func newTestOrch(t *testing.T) (*Orchestrator, registry.Store, context.Context) {
	t.Helper()
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "deploy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Config{ArtifactsDir: dir, ArtifactsBucket: "cellp-artifacts"}
	o := New(store, job.NewSQLiteQueue(store), branch.New(dir+"/off", store),
		runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"),
		&artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}, cfg)
	return o, store, context.Background()
}

func TestRunDeployVersionMissing(t *testing.T) {
	o, _, ctx := newTestOrch(t)
	err := o.runDeploy(ctx, &registry.Job{ProjectID: "demo", VersionID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunDeployBadArtifactURI(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v1", ProjectID: "demo",
		ArtifactURI: "https://evil.example/bundle",
	})
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := o.runDeploy(ctx, j); err == nil {
		t.Fatal("expected fetch error")
	}
}

func TestRunDeployInjectedFailure(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v1", ProjectID: "demo", GitSHA: "fail",
		ArtifactURI: artifact.ServerArtifactURI("cellp-artifacts", "demo", "v1"),
	})
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := o.runDeploy(ctx, j); err == nil {
		t.Fatal("expected inject error")
	}
}

func TestRunBindingBranchesNoBindings(t *testing.T) {
	o, _, ctx := newTestOrch(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(`{"name":"counter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.runBindingBranches(ctx, "demo", "child", "parent", dir); err != nil {
		t.Fatal(err)
	}
}

func TestBranchStepFailClosed(t *testing.T) {
	o, _, ctx := newTestOrch(t)
	t.Setenv("CELLP_LENIENT_DEPLOY", "")
	err := o.branchStep(ctx, "fork", func() error { return os.ErrInvalid })
	if err == nil {
		t.Fatal("expected branch step error")
	}
}

func TestRunDeployReadyWithoutCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	uri := artifact.ServerArtifactURI("cellp-artifacts", "demo", "v1")
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v1", ProjectID: "demo", ArtifactURI: uri, GitRef: "main", GitSHA: "abc",
	})
	destDir := filepath.Join(o.cfg.ArtifactsDir, "demo", "v1")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := o.runDeploy(ctx, j); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("version %+v", v)
	}
}

func TestCompensateDeploy(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})
	o.compensateDeploy(ctx, "demo", "v1")
	r, _ := store.GetRoute(ctx, "demo", "v1")
	if r != nil && r.Active {
		t.Fatal("route should be inactive")
	}
}
