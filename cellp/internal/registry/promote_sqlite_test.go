package registry

import (
	"context"
	"testing"
)

func TestCommitProdPromoteAtomic(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/promote.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v-old", ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v-new", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-old", StatusReady, nil)
	_ = store.UpdateVersionStatus(ctx, "demo", "v-new", StatusReady, nil)
	_ = store.SetProdVersionCAS(ctx, "demo", "", "v-old")
	_ = store.SetRoute(ctx, Route{
		ProjectID: "demo", VersionID: "v-old", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})
	_ = store.SetRoute(ctx, Route{
		ProjectID: "demo", VersionID: "v-new", Active: false,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8793,
	})

	revBefore, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}

	revAfter, err := store.CommitProdPromote(ctx, "demo", "v-old", "v-new")
	if err != nil {
		t.Fatal(err)
	}
	if revAfter != revBefore+1 {
		t.Fatalf("revision: before=%d after=%d", revBefore, revAfter)
	}

	p, _ := store.GetProject(ctx, "demo")
	if p == nil || p.ProdVersionID == nil || *p.ProdVersionID != "v-new" {
		t.Fatalf("prod=%v", p)
	}
	r, _ := store.GetRoute(ctx, "demo", "v-new")
	if r == nil || !r.Active {
		t.Fatalf("new route: %+v", r)
	}
}

func TestCommitProdPromoteMissingRoute(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/promote-fail.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v-new", ProjectID: "demo"})
	_ = store.SetProdVersionCAS(ctx, "demo", "", "v-old")

	_, err = store.CommitProdPromote(ctx, "demo", "v-old", "v-new")
	if err == nil {
		t.Fatal("expected error for missing route")
	}
}
