package registry

import (
	"errors"
	"testing"
	"time"
)

func TestFencedDeployWrite_RejectsNonOwner(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	lease := time.Minute
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jA := claimPendingJob(t, store, ctx, "w-a")
	attemptA := claimDeployOp(t, store, ctx, jA, lease)
	_ = store.FailJob(ctx, jA.ID)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jB := claimPendingJob(t, store, ctx, "w-b")
	claimDeployOp(t, store, ctx, jB, lease)
	msg := "boom"
	if err := store.UpdateVersionStatusForDeployOperation(ctx, "demo", "v1", attemptA, StatusFailed, &msg); !errors.Is(err, ErrDeployOperationNotOwner) {
		t.Fatalf("old job fenced status: %v", err)
	}
	if err := store.SetRouteActiveForDeployOperation(ctx, "demo", "v1", attemptA, false); !errors.Is(err, ErrDeployOperationNotOwner) {
		t.Fatalf("old job fenced route: %v", err)
	}
}

func TestFencedDeployWrite_ExpiredLeaseRejectsOwner(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	lease := 20 * time.Millisecond
	attempt := claimDeployOp(t, store, ctx, j, lease)
	time.Sleep(30 * time.Millisecond)
	if err := store.UpdateVersionStatusForDeployOperation(ctx, "demo", "v1", attempt, StatusFailed, nil); !errors.Is(err, ErrDeployOperationNotOwner) {
		t.Fatalf("expired lease must reject fenced write: %v", err)
	}
}

func TestClaimCompensatingJob_StaleLeaseTakeover(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	short := 30 * time.Millisecond
	got, err := store.ClaimCompensatingJob(ctx, "w1", short)
	if err != nil || got == nil {
		t.Fatalf("first claim: got=%v err=%v", got, err)
	}
	time.Sleep(50 * time.Millisecond)
	got2, err := store.ClaimCompensatingJob(ctx, "w2", time.Minute)
	if err != nil || got2 == nil {
		t.Fatalf("takeover claim: got=%v err=%v", got2, err)
	}
	if got2.ID != j.ID {
		t.Fatalf("job id=%s", got2.ID)
	}
	if got2.ClaimEpoch <= got.ClaimEpoch {
		t.Fatalf("takeover must bump epoch: old=%d new=%d", got.ClaimEpoch, got2.ClaimEpoch)
	}
}

func TestClaimJob_ReclaimBumpsEpoch_FencesStaleDeployAttempt(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j1 := claimPendingJob(t, store, ctx, "w1")
	staleAttempt := claimDeployOp(t, store, ctx, j1, time.Minute)
	time.Sleep(2 * time.Millisecond)
	// Expire worker lease so another worker reclaims same job id.
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx, `UPDATE jobs SET lease_until = ? WHERE id = ?`, past, j1.ID); err != nil {
		t.Fatal(err)
	}
	j2, err := store.ClaimJob(ctx, "w2", time.Minute)
	if err != nil || j2 == nil || j2.ID != j1.ID {
		t.Fatalf("reclaim job: j2=%v err=%v", j2, err)
	}
	if j2.ClaimEpoch <= staleAttempt.ClaimEpoch {
		t.Fatalf("reclaim must bump epoch")
	}
	if err := store.UpdateVersionStatusForDeployOperation(ctx, "demo", "v1", staleAttempt, StatusFailed, nil); !errors.Is(err, ErrDeployOperationNotOwner) {
		t.Fatalf("stale attempt must not write: %v", err)
	}
	newAttempt := JobDeployAttempt(j2)
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", newAttempt, time.Minute); err != nil {
		t.Fatalf("new worker deploy op: %v", err)
	}
	if err := store.UpdateVersionStatusForDeployOperation(ctx, "demo", "v1", newAttempt, StatusFailed, nil); err != nil {
		t.Fatalf("new attempt write: %v", err)
	}
}

func TestRenewClaimedJobLease_StaleEpochRejected(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w1")
	stale := JobDeployAttempt(j)
	if err := store.RenewClaimedJobLease(ctx, "w1", stale, time.Minute); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx, `UPDATE jobs SET lease_until = ? WHERE id = ?`, past, j.ID); err != nil {
		t.Fatal(err)
	}
	j2, err := store.ClaimJob(ctx, "w2", time.Minute)
	if err != nil || j2 == nil || j2.ID != j.ID {
		t.Fatalf("reclaim: %+v err=%v", j2, err)
	}
	if err := store.RenewClaimedJobLease(ctx, "w1", stale, time.Minute); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("stale epoch renew: %v", err)
	}
}

func TestPrepareDiscoveredCompensationJob_SkipsActiveClaim(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.ClaimCompensatingJob(ctx, "w1", time.Minute)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	prepared, err := store.PrepareDiscoveredCompensationJob(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared {
		t.Fatal("must not prepare while compensating claim is active")
	}
}

func TestDiscoveredPrepareThenClaimCompensating_NoDoubleFinalize(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	_ = store.FailJob(ctx, j.ID)
	prepared, err := store.PrepareDiscoveredCompensationJob(ctx, j.ID)
	if err != nil || !prepared {
		t.Fatalf("prepare: prepared=%v err=%v", prepared, err)
	}
	got, err := store.ClaimCompensatingJob(ctx, "w1", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("claim compensating: %v", err)
	}
	prepared2, _ := store.PrepareDiscoveredCompensationJob(ctx, j.ID)
	if prepared2 {
		t.Fatal("second prepare must not succeed while claimed")
	}
}

func TestListDeployQualificationCompensationWork_CompensatingWithoutFailedVersion(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonPrefix + j.ID,
	}); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v != nil && v.Status == StatusFailed {
		t.Fatal("version must not be failed for this orphan scenario")
	}
	work, err := store.ListDeployQualificationCompensationWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range work {
		if item.JobID == j.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected compensating job in discovery work: %+v", work)
	}
}

func TestListDeployQualificationCompensationWork_ExcludesCrossVersionJobReason(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	jV2, _ := store.EnqueueJob(ctx, "demo", "v2", StatusFetching)
	failMsg := "failed"
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusFailed, &failMsg); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonPrefix + jV2.ID,
	}); err != nil {
		t.Fatal(err)
	}
	work, err := store.ListDeployQualificationCompensationWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range work {
		if item.VersionID == "v1" && item.JobID == jV2.ID {
			t.Fatalf("cross-version job reason must not associate to v1 desire: %+v", work)
		}
	}
}

func TestListDeployQualificationCompensationWork_IncludesInvalidExcludesFutureValid(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}

	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	jInvalid := claimPendingJob(t, store, ctx, "w-invalid")
	setJobCompensatingStepLease(t, store, ctx, jInvalid.ID, "not-rfc3339")
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonPrefix + jInvalid.ID,
	}); err != nil {
		t.Fatal(err)
	}

	_, _ = store.EnqueueJob(ctx, "demo", "v2", StatusFetching)
	jFuture := claimPendingJob(t, store, ctx, "w-future")
	future := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	setJobCompensatingStepLease(t, store, ctx, jFuture.ID, future)
	if err := store.CompareAndSetDesired(ctx, "demo", "v2", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonPrefix + jFuture.ID,
	}); err != nil {
		t.Fatal(err)
	}

	work, err := store.ListDeployQualificationCompensationWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var hasInvalid, hasFuture bool
	for _, item := range work {
		if item.JobID == jInvalid.ID {
			hasInvalid = true
		}
		if item.JobID == jFuture.ID {
			hasFuture = true
		}
	}
	if !hasInvalid {
		t.Fatalf("invalid-lease claimed compensating job must be listed: %+v", work)
	}
	if hasFuture {
		t.Fatalf("future-valid claimed job must be excluded: %+v", work)
	}
}

func TestListDeployQualificationCompensationWork_ExcludesNormalDeployStep(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j := claimPendingJob(t, store, ctx, "w-deploy")
	if err := store.UpdateJobStep(ctx, j.ID, StatusDeploying); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonPrefix + j.ID,
	}); err != nil {
		t.Fatal(err)
	}

	work, err := store.ListDeployQualificationCompensationWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range work {
		if item.JobID == j.ID {
			t.Fatalf("normal deploy step job must not be listed: %+v", work)
		}
	}
}
