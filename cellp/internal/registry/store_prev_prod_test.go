package registry

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPreviousProdFields(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "prev.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", "v1")
	if err := store.SetProdVersionCAS(ctx, "demo", "v1", "v2"); err != nil {
		t.Fatal(err)
	}
	p, _ := store.GetProject(ctx, "demo")
	if p == nil || p.ProdVersionID == nil || *p.ProdVersionID != "v2" {
		t.Fatalf("prod=%v", p)
	}
	if p.PreviousProdVersionID == nil || *p.PreviousProdVersionID != "v1" {
		t.Fatalf("prev prod=%v", p.PreviousProdVersionID)
	}
}

func TestListVersionsSinceFilter(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "since.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	since := time.Now().UTC().Add(-time.Hour)
	page, err := store.ListVersions(ctx, "demo", ListVersionsOpts{Limit: 10, Since: &since})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Versions) != 1 {
		t.Fatalf("versions=%d", len(page.Versions))
	}
}
