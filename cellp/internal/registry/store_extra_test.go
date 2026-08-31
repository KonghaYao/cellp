package registry

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSetProdVersionCASConflict(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "prod.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", "v1")

	err = store.SetProdVersionCAS(ctx, "demo", "v-wrong", "v2")
	if err == nil {
		t.Fatal("expected CAS conflict")
	}
}

func TestListAllReadyVersions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ready.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil)
	_ = store.UpdateVersionStatus(ctx, "demo", "v2", StatusArchived, nil)

	all, err := store.ListAllReadyVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "v1" {
		t.Fatalf("ready = %+v", all)
	}
}

func TestCountChildVersions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "child.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	parent := "v-parent"
	pid := parent
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: parent, ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{
		ID: "v-child", ProjectID: "demo", ParentVersionID: &pid,
	})
	_ = store.UpdateVersionStatus(ctx, "demo", parent, StatusReady, nil)
	_ = store.UpdateVersionStatus(ctx, "demo", "v-child", StatusArchived, nil)

	n, err := store.CountChildVersions(ctx, "demo", parent)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("children = %d", n)
	}
}
