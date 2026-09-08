package registry

import (
	"errors"
	"testing"
	"time"
)

func TestRecoverLegacyDeployQualification_UniqueCandidate(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	_ = store.FailJob(ctx, j.ID)
	failMsg := "boom"
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusFailed, &failMsg); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonLegacyExact,
	}); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.RecoverLegacyDeployQualificationIfUnique(ctx, "demo", "v1")
	if err != nil || !recovered {
		t.Fatalf("recover: recovered=%v err=%v", recovered, err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.Reason != deployQualificationReasonPrefix+j.ID {
		t.Fatalf("desire: %+v", d)
	}
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "compensating" {
		t.Fatalf("job: %+v", rec)
	}
}

func TestRecoverLegacyDeployQualification_MultipleCandidatesNoOp(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j1, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j2, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	_ = store.FailJob(ctx, j1.ID)
	_ = store.FailJob(ctx, j2.ID)
	failMsg := "boom"
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", StatusFailed, &failMsg)
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonLegacyExact,
	})
	recovered, err := store.RecoverLegacyDeployQualificationIfUnique(ctx, "demo", "v1")
	if err != nil || recovered {
		t.Fatalf("want ambiguous no-op: recovered=%v err=%v", recovered, err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d.Reason != deployQualificationReasonLegacyExact {
		t.Fatalf("desire unchanged: %+v", d)
	}
}

func TestRecoverLegacyDeployQualification_ActiveDeployOwnerNoOp(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	jComp, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	_ = store.FailJob(ctx, jComp.ID)
	jActive, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	active := claimPendingJob(t, store, ctx, "w-active")
	att := claimDeployOp(t, store, ctx, active, time.Minute)
	if att.JobID != jActive.ID {
		t.Fatalf("active job=%s", att.JobID)
	}
	failMsg := "boom"
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", StatusFailed, &failMsg)
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonLegacyExact,
	})
	recovered, err := store.RecoverLegacyDeployQualificationIfUnique(ctx, "demo", "v1")
	if err != nil || recovered {
		t.Fatalf("active deploy op must block recovery: recovered=%v err=%v", recovered, err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d.Reason != deployQualificationReasonLegacyExact {
		t.Fatalf("desire unchanged: %+v", d)
	}
}

func TestMarkJobCompensating_RejectsActiveClaim(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.ClaimCompensatingJob(ctx, "w-new", time.Minute)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if err := store.MarkJobCompensating(ctx, j.ID); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("global mark must not clear active claim: %v", err)
	}
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "claimed" || rec.ClaimedWorkerID != "w-new" {
		t.Fatalf("new worker claim must remain: %+v", rec)
	}
}

func TestMarkJobCompensatingForAttempt_StaleWorkerNoOp(t *testing.T) {
	store, ctx := openDeployOperationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	j1 := claimPendingJob(t, store, ctx, "w1")
	stale := JobDeployAttempt(j1)
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := store.(*SQLiteStore).db.ExecContext(ctx, `UPDATE jobs SET lease_until = ? WHERE id = ?`, past, j1.ID); err != nil {
		t.Fatal(err)
	}
	j2, err := store.ClaimJob(ctx, "w2", time.Minute)
	if err != nil || j2 == nil || j2.ID != j1.ID {
		t.Fatalf("reclaim: %+v err=%v", j2, err)
	}
	if err := store.MarkJobCompensatingForAttempt(ctx, "w1", stale); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("stale worker exact mark must fail: %v", err)
	}
	if err := store.MarkJobCompensating(ctx, j1.ID); !errors.Is(err, ErrJobLeaseConflict) {
		t.Fatalf("global mark must fail on active new claim: %v", err)
	}
	rec, _ := store.GetJob(ctx, j1.ID)
	if rec == nil || rec.ClaimedWorkerID != "w2" || rec.ClaimEpoch <= stale.ClaimEpoch {
		t.Fatalf("new epoch claim preserved: %+v", rec)
	}
}
