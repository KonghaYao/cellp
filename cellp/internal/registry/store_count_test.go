package registry

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCountVersionsAndJobs(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "count.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"})

	n, err := store.CountVersions(ctx, "demo")
	if err != nil || n != 2 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	pending, err := store.CountPendingJobs(ctx)
	if err != nil || pending != 1 {
		t.Fatalf("pending=%d err=%v", pending, err)
	}
}
