package registry

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRepairClaimedJobLeaseInvariant_NullLeaseNoDeployOp(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("expected repair")
	}
	got, _ := store.GetJob(ctx, j.ID)
	if got == nil || got.Status != "compensating" || got.ClaimedWorkerID != "" {
		t.Fatalf("after repair: %+v", got)
	}
}

func TestRepairClaimedJobLeaseInvariant_ActiveDeployOpBlocks(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "")

	_, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
	if !errors.Is(err, ErrJobClaimedLeaseInvariant) {
		t.Fatalf("want invariant error, got %v", err)
	}
	got, _ := store.GetJob(ctx, j.ID)
	if got == nil || got.Status != "claimed" {
		t.Fatalf("job must not move: %+v", got)
	}
	_ = att
}

func TestClaimCompensatingJob_TakeoverNullLeaseCompensatingStep(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
	if err != nil || !repaired {
		t.Fatalf("repair before claim: repaired=%v err=%v", repaired, err)
	}
	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("claim: %+v err=%v", got, err)
	}
	if got.ClaimedWorkerID != "w2" || got.Step != compensatingStepPrefix+"route" {
		t.Fatalf("takeover: %+v", got)
	}
	if got.ClaimEpoch < 1 {
		t.Fatalf("epoch bumped: %d", got.ClaimEpoch)
	}
}

func TestClaimCompensatingJob_DoesNotGrabFutureInvalidLeaseString(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	futureGarbage := "9999-99-99T99:99:99.999999999Z"
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx,
		`UPDATE jobs SET step = ?, lease_until = ? WHERE id = ?`, compensatingStepPrefix+"desire", futureGarbage, j.ID); err != nil {
		t.Fatal(err)
	}

	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("invalid lease must not be taken over: %+v", got)
	}
}

func TestUpdateJobStepForAttempt_Fenced(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := JobDeployAttempt(j)
	if err := store.UpdateJobStepForAttempt(ctx, "w1", att, compensatingStepPrefix+"desire"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateJobStepForAttempt(ctx, "w2", att, compensatingStepPrefix+"route"); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("wrong worker: %v", err)
	}
	if err := store.UpdateJobStepForAttempt(ctx, "w1", DeployAttempt{JobID: j.ID, ClaimEpoch: att.ClaimEpoch - 1}, compensatingStepPrefix+"route"); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("stale epoch: %v", err)
	}
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx, `UPDATE jobs SET lease_until = ? WHERE id = ?`, past, j.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateJobStepForAttempt(ctx, "w1", att, compensatingStepPrefix+"route"); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("expired lease: %v", err)
	}
}

func TestRepairAndClaimCompensating_ConcurrentOneWinner(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	epochBefore := j.ClaimEpoch
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	var repairedWins, claimWins atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
		if err == nil && repaired {
			repairedWins.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
		if err == nil && got != nil && got.ID == j.ID {
			claimWins.Add(1)
		}
	}()
	wg.Wait()
	got, _ := store.GetJob(ctx, j.ID)
	if got == nil {
		t.Fatal("missing job")
	}
	if repairedWins.Load() > 1 {
		t.Fatalf("at most one repair winner, got %d", repairedWins.Load())
	}
	if repairedWins.Load()+claimWins.Load() < 1 {
		t.Fatalf("expected repair or claim progress: repaired=%d claim=%d state=%+v", repairedWins.Load(), claimWins.Load(), got)
	}
	switch got.Status {
	case "compensating":
		if got.ClaimedWorkerID != "" {
			t.Fatalf("compensating must have no active worker owner: %+v", got)
		}
	case "claimed":
		if got.ClaimedWorkerID != "w2" {
			t.Fatalf("single active owner expected w2, got %q (%+v)", got.ClaimedWorkerID, got)
		}
		if got.ClaimEpoch <= epochBefore {
			t.Fatalf("claim_epoch must increase when claimed: before=%d after=%d", epochBefore, got.ClaimEpoch)
		}
	default:
		t.Fatalf("unexpected terminal state: %+v", got)
	}
	if got.Status == "claimed" && claimWins.Load() != 1 {
		t.Fatalf("claimed terminal requires exactly one winning claim, claimWins=%d", claimWins.Load())
	}
}

func TestClaimJob_SkipsExpiredCompensatingStep(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx,
		`UPDATE jobs SET step = ?, lease_until = ? WHERE id = ?`, compensatingStepPrefix+"desire", past, j.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)

	got, err := store.ClaimJob(ctx, "w2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID == j.ID {
		t.Fatalf("ClaimJob must not steal compensating-step job, got %+v", got)
	}
}

func TestRenewClaimedJobLease_RejectsNullInvalidExpired(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := JobDeployAttempt(j)

	if err := store.RenewClaimedJobLease(ctx, "w1", att, time.Minute); err != nil {
		t.Fatalf("renew future lease: %v", err)
	}

	if _, err := store.(*SQLiteStore).db.ExecContext(ctx, `UPDATE jobs SET lease_until = NULL WHERE id = ?`, j.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RenewClaimedJobLease(ctx, "w1", att, time.Minute); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("null lease renew: %v", err)
	}

	garbage := "not-a-timestamp"
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx, `UPDATE jobs SET lease_until = ? WHERE id = ?`, garbage, j.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RenewClaimedJobLease(ctx, "w1", att, time.Minute); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("invalid lease renew: %v", err)
	}

	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx, `UPDATE jobs SET lease_until = ? WHERE id = ?`, past, j.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RenewClaimedJobLease(ctx, "w1", att, time.Minute); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("expired lease renew: %v", err)
	}
}

func TestRepairClaimedJobLeaseInvariant_InvalidAndExpiredLease(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	for _, lease := range []string{"garbage-ts", time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)} {
		setJobCompensatingStepLease(t, store, ctx, j.ID, lease)
		repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
		if err != nil || !repaired {
			t.Fatalf("lease=%q repaired=%v err=%v", lease, repaired, err)
		}
		got, _ := store.GetJob(ctx, j.ID)
		if got == nil || got.Status != "compensating" {
			t.Fatalf("after repair lease=%q: %+v", lease, got)
		}
		if _, err := store.(*SQLiteStore).db.ExecContext(ctx,
			`UPDATE jobs SET status = 'claimed', claimed_worker_id = 'w1', claim_epoch = claim_epoch + 1 WHERE id = ?`, j.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepairThenClaimCompensating_InvalidLeaseNewEpoch(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	epochBefore := j.ClaimEpoch
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "not-rfc3339")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
	if err != nil || !repaired {
		t.Fatalf("repair: %v repaired=%v", err, repaired)
	}
	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("claim: %v %+v", err, got)
	}
	if got.ClaimEpoch <= epochBefore {
		t.Fatalf("expected epoch bump: before=%d after=%d", epochBefore, got.ClaimEpoch)
	}
}

func TestClaimCompensatingJob_DoesNotGrabClaimedInvalidWithoutRepair(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)
	_ = att

	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("must not claim claimed+invalid lease without repair: %+v", got)
	}
	row, _ := store.GetJob(ctx, j.ID)
	if row == nil || row.Status != "claimed" {
		t.Fatalf("job unchanged: %+v", row)
	}
}

func TestPrepareDiscoveredCompensationJob_SkipsClaimedInvalidLease(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	setJobCompensatingStepLease(t, store, ctx, j.ID, "bad-lease")
	prepared, err := store.PrepareDiscoveredCompensationJob(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared {
		t.Fatal("prepare must not mutate claimed compensating job with invalid lease")
	}
	got, _ := store.GetJob(ctx, j.ID)
	if got == nil || got.Status != "claimed" {
		t.Fatalf("status: %+v", got)
	}
}

func TestRepairClaimedJobLeaseInvariant_RejectsFutureValidLease(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	future := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, j.ID, future)
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired {
		t.Fatal("future-valid lease must not be repaired")
	}
}

func TestClaimCompensatingJob_SkipsUnclaimableHeadClaimsReadyJob(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jHead := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, jHead, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, jHead.ID, "")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	jReady, _ := store.EnqueueJob(ctx, "demo", "v2", StatusFetching)
	if err := store.MarkJobCompensating(ctx, jReady.ID); err != nil {
		t.Fatal(err)
	}
	oldTS := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx,
		`UPDATE jobs SET updated_at = ? WHERE id = ?`, oldTS, jHead.ID); err != nil {
		t.Fatal(err)
	}

	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != jReady.ID {
		t.Fatalf("expected ready job behind blocked head, got %+v", got)
	}
}

func TestClaimCompensatingJob_SkipsHeadBlockedByActiveDeployOp(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jHead := claimPendingJob(t, store, ctx, "w1")
	_ = claimDeployOp(t, store, ctx, jHead, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, jHead.ID, "bad-lease")

	jReady, _ := store.EnqueueJob(ctx, "demo", "v2", StatusFetching)
	if err := store.MarkJobCompensating(ctx, jReady.ID); err != nil {
		t.Fatal(err)
	}
	oldTS := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx,
		`UPDATE jobs SET updated_at = ? WHERE id = ?`, oldTS, jHead.ID); err != nil {
		t.Fatal(err)
	}

	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != jReady.ID {
		t.Fatalf("expected job behind deploy-op-blocked head, got %+v", got)
	}
}

func TestClaimCompensatingJob_ConcurrentTwoWorkersOneWinner(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var winners []string
	var mu sync.Mutex
	wg.Add(2)
	for _, wid := range []string{"w-a", "w-b"} {
		go func(worker string) {
			defer wg.Done()
			got, err := store.ClaimCompensatingJob(ctx, worker, time.Minute)
			if err != nil {
				t.Errorf("%s: %v", worker, err)
				return
			}
			if got != nil {
				mu.Lock()
				winners = append(winners, worker)
				mu.Unlock()
			}
		}(wid)
	}
	wg.Wait()
	if len(winners) != 1 {
		t.Fatalf("exactly one winner, got %v", winners)
	}
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "claimed" || rec.ClaimedWorkerID != winners[0] {
		t.Fatalf("single active owner: %+v winners=%v", rec, winners)
	}
}

func TestClaimCompensatingJob_ExpiredClaimedCompensatingDirectTakeover(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, j.ID, past)
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("expired lease takeover: err=%v got=%+v", err, got)
	}
	if got.ClaimedWorkerID != "w2" || got.Status != "claimed" {
		t.Fatalf("takeover: %+v", got)
	}
}

func TestClaimCompensatingJob_ReachesReadyPast64BlockedTakeoverRows(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)
	for i := 0; i < 64; i++ {
		_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
		block := claimPendingJob(t, store, ctx, "w-block")
		setJobCompensatingStepLease(t, store, ctx, block.ID, "")
		setJobUpdatedAtForTest(t, store, ctx, block.ID, stamp)
	}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	ready := claimPendingJob(t, store, ctx, "w-ready")
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, ready.ID, past)
	setJobUpdatedAtForTest(t, store, ctx, ready.ID, stamp.Add(time.Minute))

	got, err := store.ClaimCompensatingJob(ctx, "w-winner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != ready.ID {
		t.Fatalf("expected ready job past blocked window, got %+v", got)
	}
}

func TestClaimCompensatingJob_ReachesCompensatingPast64BlockedClaimedHeads(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v-ready", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC().Add(-2 * time.Hour)
	for i := 0; i < 64; i++ {
		_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
		block := claimPendingJob(t, store, ctx, "w-block")
		if err := store.UpdateJobStepForAttempt(ctx, "w-block", JobDeployAttempt(block), compensatingStepPrefix+"desire"); err != nil {
			t.Fatal(err)
		}
		setJobUpdatedAtForTest(t, store, ctx, block.ID, stamp)
	}
	jReady, _ := store.EnqueueJob(ctx, "demo", "v-ready", StatusFetching)
	if err := store.MarkJobCompensating(ctx, jReady.ID); err != nil {
		t.Fatal(err)
	}

	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != jReady.ID {
		t.Fatalf("expected compensating job behind 64 live-claimed heads, got %+v", got)
	}
}

func TestRepairClaimedJobLeaseInvariant_ConcurrentRepairOneTrue(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	var repairedCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
			if err == nil && repaired {
				repairedCount.Add(1)
			}
		}()
	}
	wg.Wait()
	if repairedCount.Load() != 1 {
		t.Fatalf("exactly one repair winner, got %d", repairedCount.Load())
	}
}

func TestRepairAndClaimCompensating_RoundsInvariant(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	epochStart := j.ClaimEpoch
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "")
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)

	const rounds = 12
	for i := 0; i < rounds; i++ {
		var roundRepairedWins atomic.Int32
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
			if err == nil && repaired {
				roundRepairedWins.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			_, _ = store.ClaimCompensatingJob(ctx, "w2", time.Minute)
		}()
		wg.Wait()
		if roundRepairedWins.Load() > 1 {
			t.Fatalf("round %d: at most one repair winner, got %d", i, roundRepairedWins.Load())
		}

		got, _ := store.GetJob(ctx, j.ID)
		if got == nil {
			t.Fatal("missing job")
		}
		if got.Status == "claimed" {
			if got.ClaimedWorkerID != "w2" {
				t.Fatalf("round %d: unexpected worker %q", i, got.ClaimedWorkerID)
			}
			if got.ClaimEpoch <= epochStart {
				t.Fatalf("round %d: epoch must be strictly above start (%d got %d)", i, epochStart, got.ClaimEpoch)
			}
			break
		}
		if got.Status != "compensating" {
			t.Fatalf("round %d: unexpected status %q", i, got.Status)
		}
	}
	final, _ := store.GetJob(ctx, j.ID)
	if final == nil || final.Status != "claimed" || final.ClaimedWorkerID != "w2" {
		t.Fatalf("expected single claimed owner w2, got %+v", final)
	}
}

func TestClaimCompensatingJob_TakeoverScanOffsetAdvancesWithoutClaimable(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)
	seedBlockedCompensatingTakeoverRows(t, store, ctx, 64, stamp)

	setCompensatingTakeoverScanOffsetForTest(t, store, 0)
	got, err := store.ClaimCompensatingJob(ctx, "w-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("blocked heads should not be claimed, got %+v", got)
	}
	offsetAfter := compensatingTakeoverScanOffsetForTest(t, store)
	if offsetAfter <= 0 {
		t.Fatalf("cursor should advance after blocked scan, got %d", offsetAfter)
	}
}

func TestClaimCompensatingJob_TakeoverScanOffsetWrapsPastEnd(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)
	seedBlockedCompensatingTakeoverRows(t, store, ctx, 4, stamp)

	setCompensatingTakeoverScanOffsetForTest(t, store, 128)
	got, err := store.ClaimCompensatingJob(ctx, "w-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("no claimable rows, got %+v", got)
	}
	offsetAfter := compensatingTakeoverScanOffsetForTest(t, store)
	if offsetAfter >= 128 {
		t.Fatalf("cursor should wrap/advance past stale offset, got %d", offsetAfter)
	}
}

func TestClaimCompensatingJob_ConcurrentManyWorkersPast64Blocked(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)
	for i := 0; i < 64; i++ {
		_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
		block := claimPendingJob(t, store, ctx, "w-block")
		setJobCompensatingStepLease(t, store, ctx, block.ID, "")
		setJobUpdatedAtForTest(t, store, ctx, block.ID, stamp)
	}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	ready := claimPendingJob(t, store, ctx, "w-ready")
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, ready.ID, past)
	setJobUpdatedAtForTest(t, store, ctx, ready.ID, stamp.Add(time.Minute))

	const workers = 16
	var wg sync.WaitGroup
	var winners []string
	var mu sync.Mutex
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		wid := fmt.Sprintf("w-%d", i)
		go func(worker string) {
			defer wg.Done()
			got, err := store.ClaimCompensatingJob(ctx, worker, time.Minute)
			if err != nil {
				t.Errorf("%s: %v", worker, err)
				return
			}
			if got != nil && got.ID == ready.ID {
				mu.Lock()
				winners = append(winners, worker)
				mu.Unlock()
			}
		}(wid)
	}
	wg.Wait()
	if len(winners) != 1 {
		t.Fatalf("exactly one winner for claimable job, got %v", winners)
	}
	rec, _ := store.GetJob(ctx, ready.ID)
	if rec == nil || rec.Status != "claimed" || rec.ClaimedWorkerID != winners[0] {
		t.Fatalf("single owner: %+v winners=%v", rec, winners)
	}
}

func TestRepairThenClaimCompensating_InvalidLeaseEpochBump(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	epochBefore := j.ClaimEpoch
	att := claimDeployOp(t, store, ctx, j, time.Minute)
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)
	setJobCompensatingStepLease(t, store, ctx, j.ID, "bad-lease")

	repaired, err := store.RepairClaimedJobLeaseInvariantForCompensation(ctx, j.ID)
	if err != nil || !repaired {
		t.Fatalf("repair invalid lease: repaired=%v err=%v", repaired, err)
	}
	mid, _ := store.GetJob(ctx, j.ID)
	if mid == nil || mid.Status != "compensating" {
		t.Fatalf("after repair: %+v", mid)
	}

	got, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("claim after repair: %+v err=%v", got, err)
	}
	if got.ClaimEpoch <= epochBefore {
		t.Fatalf("epoch must increase: before=%d after=%d", epochBefore, got.ClaimEpoch)
	}
	if got.ClaimedWorkerID != "w2" {
		t.Fatalf("single worker owner: %+v", got)
	}
}

func TestCommitCompensatingClaimAndStoreTakeoverCursor_CommitFailurePreservesCursor(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	sqlite, ok := store.(*SQLiteStore)
	if !ok {
		t.Fatal("sqlite store required")
	}
	var counter atomic.Int64
	resetCompensatingTakeoverScanOffsetAtomicForTest(&counter, 42)

	tx, err := sqlite.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := commitCompensatingClaimAndStoreTakeoverCursor(tx, &counter, 99); err == nil {
		t.Fatal("expected commit error on finished transaction")
	}
	if loadCompensatingTakeoverScanOffset(&counter) != 42 {
		t.Fatalf("cursor must not advance on commit failure, got %d", loadCompensatingTakeoverScanOffset(&counter))
	}
}

func TestClaimCompensatingJob_ReachesReadyPast256BlockedAcrossCalls(t *testing.T) {
	// Phase-2 per-phase scan budget is 256; 256 blocked takeover rows plus one ready row needs a second call.
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)
	seedBlockedCompensatingTakeoverRows(t, store, ctx, 256, stamp)

	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	ready := claimPendingJob(t, store, ctx, "w-ready")
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, ready.ID, past)
	setJobUpdatedAtForTest(t, store, ctx, ready.ID, stamp.Add(time.Minute))

	setCompensatingTakeoverScanOffsetForTest(t, store, 0)
	got1, err := store.ClaimCompensatingJob(ctx, "w-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got1 != nil {
		t.Fatalf("first call should exhaust phase-2 per-phase scan cap before ready row, got %+v", got1)
	}
	if compensatingTakeoverScanOffsetForTest(t, store) != compensatingClaimMaxScanPerPhase {
		t.Fatalf("cursor should advance to %d after first pass, got %d", compensatingClaimMaxScanPerPhase, compensatingTakeoverScanOffsetForTest(t, store))
	}

	got2, err := store.ClaimCompensatingJob(ctx, "w-winner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got2 == nil || got2.ID != ready.ID {
		t.Fatalf("second call should claim ready job past blocked window, got %+v", got2)
	}
}

func TestClaimCompensatingJob_ConcurrentManyWorkersPast256BlockedAcrossCalls(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)
	seedBlockedCompensatingTakeoverRows(t, store, ctx, 256, stamp)

	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	ready := claimPendingJob(t, store, ctx, "w-ready")
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, ready.ID, past)
	setJobUpdatedAtForTest(t, store, ctx, ready.ID, stamp.Add(time.Minute))

	setCompensatingTakeoverScanOffsetForTest(t, store, 0)

	const workers = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	var winners []string
	var mu sync.Mutex
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		wid := fmt.Sprintf("w-%d", i)
		go func(worker string) {
			defer wg.Done()
			<-start
			for attempt := 0; attempt < 12; attempt++ {
				got, err := store.ClaimCompensatingJob(ctx, worker, time.Minute)
				if err != nil {
					t.Errorf("%s attempt %d: %v", worker, attempt, err)
					return
				}
				if got != nil && got.ID == ready.ID {
					mu.Lock()
					winners = append(winners, worker)
					mu.Unlock()
					return
				}
			}
		}(wid)
	}
	close(start)
	wg.Wait()
	if len(winners) != 1 {
		t.Fatalf("exactly one winner for claimable job across calls, got %v", winners)
	}
	rec, _ := store.GetJob(ctx, ready.ID)
	if rec == nil || rec.Status != "claimed" || rec.ClaimedWorkerID != winners[0] {
		t.Fatalf("single owner: %+v winners=%v", rec, winners)
	}
}

func TestCompensatingClaimScanBudgetPerPhase(t *testing.T) {
	if compensatingClaimMaxScanPerPhase != 256 {
		t.Fatalf("unexpected per-phase cap: %d", compensatingClaimMaxScanPerPhase)
	}
	if compensatingClaimMaxScanPerCall != 512 {
		t.Fatalf("unexpected per-call cap: %d", compensatingClaimMaxScanPerCall)
	}
}

func TestClaimCompensatingJob_Phase2TakeoverSameCallWhenPhase1UsesPerPhaseScanBudget(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)

	const phase1Fillers = 200
	const blockedTakeover = 64
	seedCompensatingDirectScanFillerRows(t, store, ctx, phase1Fillers, stamp)
	seedBlockedCompensatingTakeoverRows(t, store, ctx, blockedTakeover, stamp)

	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	ready := claimPendingJob(t, store, ctx, "w-ready")
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, ready.ID, past)
	setJobUpdatedAtForTest(t, store, ctx, ready.ID, stamp.Add(time.Minute))

	compensatingDirectClaimAttemptHook = func(*Job) bool { return true }
	defer func() { compensatingDirectClaimAttemptHook = nil }()

	got, err := store.ClaimCompensatingJob(ctx, "w-winner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != ready.ID {
		t.Fatalf("phase-2 ready takeover should succeed in same call after phase-1 scan budget, got %+v", got)
	}
}

func TestClaimCompensatingJob_Phase2TakeoverSameCallWhenPhase1PerPhaseBudgetExhausted(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	stamp := time.Now().UTC().Add(-2 * time.Hour)

	seedCompensatingDirectScanFillerRows(t, store, ctx, compensatingClaimMaxScanPerPhase, stamp)

	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	ready := claimPendingJob(t, store, ctx, "w-ready")
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, ready.ID, past)
	setJobUpdatedAtForTest(t, store, ctx, ready.ID, stamp.Add(time.Minute))

	compensatingDirectClaimAttemptHook = func(*Job) bool { return true }
	defer func() { compensatingDirectClaimAttemptHook = nil }()

	got, err := store.ClaimCompensatingJob(ctx, "w-winner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != ready.ID {
		t.Fatalf("phase-2 takeover expected after full phase-1 per-phase budget, got %+v", got)
	}
}

// resetCompensatingTakeoverScanOffsetAtomicForTest sets the cursor for test fixtures (not production merge semantics).
func resetCompensatingTakeoverScanOffsetAtomicForTest(counter *atomic.Int64, offset int) {
	counter.Store(compensatingTakeoverScanOffsetToInt64(offset))
}

func TestStoreCompensatingTakeoverScanOffset_MonotonicMaxConcurrent(t *testing.T) {
	var counter atomic.Int64
	resetCompensatingTakeoverScanOffsetAtomicForTest(&counter, 0)

	firstDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		storeCompensatingTakeoverScanOffset(&counter, 500)
		close(firstDone)
	}()
	go func() {
		defer wg.Done()
		<-firstDone
		storeCompensatingTakeoverScanOffset(&counter, 50)
	}()
	wg.Wait()
	if got := loadCompensatingTakeoverScanOffset(&counter); got != 500 {
		t.Fatalf("late smaller offset must not regress cursor: got %d want 500", got)
	}
}

func TestStoreCompensatingTakeoverScanOffset_NegativeClampedToZero(t *testing.T) {
	var counter atomic.Int64
	resetCompensatingTakeoverScanOffsetAtomicForTest(&counter, 10)
	storeCompensatingTakeoverScanOffset(&counter, -3)
	if got := loadCompensatingTakeoverScanOffset(&counter); got != 10 {
		t.Fatalf("negative store must not regress via clamped max: got %d want 10", got)
	}
}

func TestTryWrapCompensatingTakeoverScanOffset_SucceedsWhenUnchanged(t *testing.T) {
	var counter atomic.Int64
	resetCompensatingTakeoverScanOffsetAtomicForTest(&counter, 128)
	if !tryWrapCompensatingTakeoverScanOffset(&counter, 128) {
		t.Fatal("expected wrap CAS success when cursor still at scan start")
	}
	if got := loadCompensatingTakeoverScanOffset(&counter); got != 0 {
		t.Fatalf("wrap should reset to 0, got %d", got)
	}
}

func TestTryWrapCompensatingTakeoverScanOffset_FailsWhenAdvancedByOther(t *testing.T) {
	var counter atomic.Int64
	resetCompensatingTakeoverScanOffsetAtomicForTest(&counter, 128)
	storeCompensatingTakeoverScanOffset(&counter, 200)
	if tryWrapCompensatingTakeoverScanOffset(&counter, 128) {
		t.Fatal("stale wrap must not succeed after another goroutine advanced cursor")
	}
	if got := loadCompensatingTakeoverScanOffset(&counter); got != 200 {
		t.Fatalf("cursor must remain advanced, got %d", got)
	}
}

func TestStoreCompensatingTakeoverScanOffset_ConcurrentMaxMerge(t *testing.T) {
	var counter atomic.Int64
	resetCompensatingTakeoverScanOffsetAtomicForTest(&counter, 0)
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		v := (i + 1) * 10
		go func(off int) {
			defer wg.Done()
			<-start
			storeCompensatingTakeoverScanOffset(&counter, off)
		}(v)
	}
	close(start)
	wg.Wait()
	if got := loadCompensatingTakeoverScanOffset(&counter); got != goroutines*10 {
		t.Fatalf("monotonic max should reach highest offset %d, got %d", goroutines*10, got)
	}
}
