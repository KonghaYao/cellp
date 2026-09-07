package registry

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func openDeployOperationTestStore(t *testing.T) (Store, context.Context) {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "deploy-op.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	return store, ctx
}

func claimPendingJob(t *testing.T, store Store, ctx context.Context, worker string) *Job {
	t.Helper()
	j, err := store.ClaimJob(ctx, worker, time.Minute)
	if err != nil || j == nil {
		t.Fatalf("claim job: %v", err)
	}
	return j
}

func claimDeployOp(t *testing.T, store Store, ctx context.Context, j *Job, lease time.Duration) DeployAttempt {
	t.Helper()
	att := JobDeployAttempt(j)
	if err := store.ClaimVersionDeployOperation(ctx, j.ProjectID, j.VersionID, att, lease); err != nil {
		t.Fatalf("claim deploy op: %v", err)
	}
	return att
}

func execJobFixtureSQL(t *testing.T, s *SQLiteStore, ctx context.Context, query string, args ...any) {
	t.Helper()
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func setJobCompensatingStepLease(t *testing.T, store Store, ctx context.Context, jobID, lease string) {
	t.Helper()
	s, ok := store.(*SQLiteStore)
	if !ok {
		t.Fatal("sqlite store required")
	}
	jobID = strings.TrimSpace(jobID)
	step := compensatingStepPrefix + "route"
	if lease == "" {
		execJobFixtureSQL(t, s, ctx, `UPDATE jobs SET step = ?, lease_until = NULL WHERE id = ?`, step, jobID)
		return
	}
	execJobFixtureSQL(t, s, ctx, `UPDATE jobs SET step = ?, lease_until = ? WHERE id = ?`, step, lease, jobID)
}

func setJobUpdatedAtForTest(t *testing.T, store Store, ctx context.Context, jobID string, updatedAt time.Time) {
	t.Helper()
	s, ok := store.(*SQLiteStore)
	if !ok {
		t.Fatal("sqlite store required")
	}
	jobID = strings.TrimSpace(jobID)
	execJobFixtureSQL(t, s, ctx, `UPDATE jobs SET updated_at = ? WHERE id = ?`,
		updatedAt.UTC().Format(time.RFC3339Nano), jobID)
}

func compensatingTakeoverScanOffsetForTest(t *testing.T, store Store) int {
	t.Helper()
	s, ok := store.(*SQLiteStore)
	if !ok {
		t.Fatal("sqlite store required")
	}
	return loadCompensatingTakeoverScanOffset(&s.compensatingTakeoverScanOffset)
}

func setCompensatingTakeoverScanOffsetForTest(t *testing.T, store Store, offset int) {
	t.Helper()
	s, ok := store.(*SQLiteStore)
	if !ok {
		t.Fatal("sqlite store required")
	}
	resetCompensatingTakeoverScanOffsetAtomicForTest(&s.compensatingTakeoverScanOffset, offset)
}

func seedBlockedCompensatingTakeoverRows(t *testing.T, store Store, ctx context.Context, n int, stamp time.Time) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
		block := claimPendingJob(t, store, ctx, "w-block")
		setJobCompensatingStepLease(t, store, ctx, block.ID, "")
		setJobUpdatedAtForTest(t, store, ctx, block.ID, stamp)
	}
}

// seedCompensatingDirectScanFillerRows creates status=compensating jobs ordered before later takeover targets.
func seedCompensatingDirectScanFillerRows(t *testing.T, store Store, ctx context.Context, n int, stamp time.Time) []string {
	t.Helper()
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		j, err := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
			t.Fatal(err)
		}
		setJobUpdatedAtForTest(t, store, ctx, j.ID, stamp)
		ids[i] = j.ID
	}
	return ids
}

func TestClaimVersionDeployOperation_SecondActiveJobConflict(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	lease := time.Minute
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jA := claimPendingJob(t, store, ctx, "w-a")
	claimDeployOp(t, store, ctx, jA, lease)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jB := claimPendingJob(t, store, ctx, "w-b")
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", JobDeployAttempt(jB), lease); !errors.Is(err, ErrDeployOperationConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestClaimVersionDeployOperation_NewJobAfterFailedOwner(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	lease := time.Minute
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	claimDeployOp(t, store, ctx, j, lease)
	_ = store.FailJob(ctx, j.ID)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jNew := claimPendingJob(t, store, ctx, "w2")
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", JobDeployAttempt(jNew), lease); err != nil {
		t.Fatalf("new job should claim after failed owner: %v", err)
	}
}

func TestClaimVersionDeployOperation_ConcurrentClaims(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	lease := time.Minute
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	claim := func(worker string) {
		defer wg.Done()
		j, err := store.ClaimJob(ctx, worker, lease)
		if err != nil || j == nil {
			errs <- err
			return
		}
		errs <- store.ClaimVersionDeployOperation(ctx, "demo", "v1", JobDeployAttempt(j), lease)
	}
	wg.Add(2)
	go claim("w-a")
	go claim("w-b")
	wg.Wait()
	close(errs)
	var ok, conflict int
	for err := range errs {
		if err == nil {
			ok++
		}
		if errors.Is(err, ErrDeployOperationConflict) {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("want one success and one conflict, got ok=%d conflict=%d", ok, conflict)
	}
}
