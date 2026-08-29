package orch_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

type blockingStore struct {
	registry.Store
	activeClaims atomic.Int32
	peakClaims   atomic.Int32
	release      chan struct{}
}

func (b *blockingStore) ClaimJob(ctx context.Context, workerID string, lease time.Duration) (*registry.Job, error) {
	j, err := b.Store.ClaimJob(ctx, workerID, lease)
	if err != nil || j == nil {
		return j, err
	}
	n := b.activeClaims.Add(1)
	for {
		peak := b.peakClaims.Load()
		if n <= peak || b.peakClaims.CompareAndSwap(peak, n) {
			break
		}
	}
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	b.activeClaims.Add(-1)
	return j, nil
}

func TestOrchestratorWorkerPool(t *testing.T) {
	t.Setenv("CELLP_ORCH_WORKERS", "3")
	t.Setenv("CELLP_E2E_INJECT_DEPLOY_FAIL", "1")

	dir := t.TempDir()
	base, err := registry.Open(filepath.Join(dir, "orch.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()

	store := &blockingStore{
		Store:   base,
		release: make(chan struct{}),
	}

	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("v%d", i)
		_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: id, ProjectID: "demo"})
		_, _ = store.EnqueueJob(ctx, "demo", id, registry.StatusFetching)
	}

	cfg := config.Config{ArtifactsDir: dir, ArtifactsBucket: "cellp-artifacts"}
	q := job.NewSQLiteQueue(store)
	bm := branch.New(dir+"/offshoot", store)
	rm := runtime.New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	as := &artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}
	o := orch.New(store, q, bm, rm, as, cfg)

	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.Run(runCtx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if store.peakClaims.Load() >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if store.peakClaims.Load() < 3 {
		t.Fatalf("expected 3 concurrent claims, peak was %d", store.peakClaims.Load())
	}

	close(store.release)
	cancel()
	wg.Wait()
}
