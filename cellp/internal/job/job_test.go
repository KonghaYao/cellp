package job

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestSQLiteQueueEnqueueNotify(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "job.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	q := NewSQLiteQueue(store)
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	select {
	case <-q.Notify():
		t.Fatal("unexpected notify before enqueue")
	default:
	}

	if err := q.Enqueue(ctx, &DeployJob{ProjectID: "demo", VersionID: "v1", Step: registry.StatusFetching}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-q.Notify():
	default:
		t.Fatal("expected notify after enqueue")
	}

	// notify channel is buffered; second enqueue when full should not block
	for i := 0; i < 70; i++ {
		_ = q.Enqueue(ctx, &DeployJob{ProjectID: "demo", VersionID: "v1", Step: registry.StatusFetching})
	}
}
