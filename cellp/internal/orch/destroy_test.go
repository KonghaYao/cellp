package orch

import (
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestDestroyReadyVersion(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8793,
	})

	if err := o.Destroy(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusDestroyed {
		t.Fatalf("status = %v", v)
	}
}

func TestDestroyAlreadyDestroyedNoop(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusDestroyed, nil)
	if err := o.Destroy(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}
