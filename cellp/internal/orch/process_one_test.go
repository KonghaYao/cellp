package orch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/registry"
)

func TestProcessOneSuccess(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	uri := artifact.ServerArtifactURI("cellp-artifacts", "demo", "v1")
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v1", ProjectID: "demo", ArtifactURI: uri, GitRef: "main", GitSHA: "abc",
	})
	destDir := filepath.Join(o.cfg.ArtifactsDir, "demo", "v1")
	_ = os.MkdirAll(destDir, 0o755)
	_ = os.WriteFile(filepath.Join(destDir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)

	o.processOne(ctx, "test-worker")
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("version %+v", v)
	}
	pending, _ := store.CountPendingJobs(ctx)
	if pending != 0 {
		t.Fatalf("pending jobs = %d", pending)
	}
}

func TestProcessOneFailMarksVersionFailed(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v1", ProjectID: "demo", GitSHA: "fail",
		ArtifactURI: artifact.ServerArtifactURI("cellp-artifacts", "demo", "v1"),
	})
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	o.processOne(ctx, "test-worker")
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusFailed {
		t.Fatalf("version %+v", v)
	}
}

func TestBranchStepLenient(t *testing.T) {
	t.Setenv("CELLP_LENIENT_DEPLOY", "1")
	o, _, ctx := newTestOrch(t)
	if err := o.branchStep(ctx, "fork", func() error { return os.ErrInvalid }); err != nil {
		t.Fatalf("lenient: %v", err)
	}
}

func TestPromoteAlreadyProdNoop(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	_ = store.SetProdVersion(ctx, "demo", "v1")
	if err := o.Promote(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}
