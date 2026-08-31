package orch_test

import (
	"context"
	"errors"
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

func setupPromoteOrch(t *testing.T) (registry.Store, *orch.Orchestrator, context.Context) {
	t.Helper()
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "promote.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"v-old", "v-new"} {
		if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: id, ProjectID: "demo"}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateVersionStatus(ctx, "demo", id, registry.StatusReady, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetProdVersionCAS(ctx, "demo", "", "v-old"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-old", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-new", Active: false,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8793,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{ArtifactsDir: dir, ArtifactsBucket: "cellp-artifacts"}
	q := job.NewSQLiteQueue(store)
	bm := branch.New(dir+"/offshoot", store)
	rm := runtime.New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	as := &artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}
	o := orch.New(store, q, bm, rm, as, cfg)
	return store, o, ctx
}

func TestPromote_OffshootFail_NoCAS(t *testing.T) {
	store, o, ctx := setupPromoteOrch(t)
	t.Setenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL", "1")

	err := o.Promote(ctx, "demo", "v-new")
	if err == nil {
		t.Fatal("expected promote error")
	}
	if !errors.Is(err, orch.ErrOffshootPromote) {
		t.Fatalf("expected ErrOffshootPromote, got %v", err)
	}

	proj, err := store.GetProject(ctx, "demo")
	if err != nil || proj == nil || proj.ProdVersionID == nil || *proj.ProdVersionID != "v-old" {
		t.Fatalf("prod should remain v-old: %+v err=%v", proj, err)
	}
}

func TestPromote_OffshootFail_CompensatesOldRoute(t *testing.T) {
	store, o, ctx := setupPromoteOrch(t)
	t.Setenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL", "1")

	_ = o.Promote(ctx, "demo", "v-new")

	route, err := store.GetRoute(ctx, "demo", "v-old")
	if err != nil || route == nil || !route.Active {
		t.Fatalf("old prod route should be active after compensation: %+v err=%v", route, err)
	}
	newRoute, err := store.GetRoute(ctx, "demo", "v-new")
	if err != nil {
		t.Fatalf("get v-new route: %v", err)
	}
	if newRoute != nil && newRoute.Active {
		t.Fatalf("new version route should not be active: %+v", newRoute)
	}
}
