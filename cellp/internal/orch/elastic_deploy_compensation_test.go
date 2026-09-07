package orch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

func openCompensationTestStore(t *testing.T) (registry.Store, context.Context) {
	return openQualificationTestStore(t)
}

func setClaimedCompensatingStepExpiredLease(t *testing.T, store registry.Store, ctx context.Context, j *registry.Job, workerID string) {
	t.Helper()
	att := registry.JobDeployAttempt(j)
	if err := store.UpdateJobStepForAttempt(ctx, workerID, att, "compensating:desire"); err != nil {
		t.Fatal(err)
	}
	if err := store.RenewClaimedJobLease(ctx, workerID, att, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
}

func TestDeployOperationRenew_KeepsSameJobOwner(t *testing.T) {
	store, ctx := openCompensationTestStore(t)
	lease := 2 * time.Second
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	j, err := store.ClaimJob(ctx, "w1", lease)
	if err != nil || j == nil {
		t.Fatal(err)
	}
	att := registry.JobDeployAttempt(j)
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", att, lease); err != nil {
		t.Fatal(err)
	}
	if err := store.RenewVersionDeployOperation(ctx, "demo", "v1", att, lease); err != nil {
		t.Fatal(err)
	}
	if err := store.AssertVersionDeployOperation(ctx, "demo", "v1", att); err != nil {
		t.Fatal(err)
	}
}

func TestClaimVersionDeployOperation_BlockedByCompensatingJob(t *testing.T) {
	store, ctx := openCompensationTestStore(t)
	jOld, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := store.MarkJobCompensating(ctx, jOld.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", registry.DeployAttempt{JobID: jOld.ID, ClaimEpoch: 0}, time.Minute); err != nil {
		t.Fatal(err)
	}
	jNew, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	jNewClaim, _ := store.ClaimJob(ctx, "w-new", time.Minute)
	if jNewClaim == nil {
		t.Fatal("claim new job")
	}
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", registry.JobDeployAttempt(jNewClaim), time.Minute); !errors.Is(err, registry.ErrDeployOperationConflict) {
		t.Fatalf("new deploy must wait for compensating job, got %v", err)
	}
	_ = jNew
}

func TestClaimCompensatingJob_DoubleClaimOneWinner(t *testing.T) {
	store, ctx := openCompensationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var wins int32
	wg.Add(2)
	claim := func(wid string) {
		defer wg.Done()
		got, err := store.ClaimCompensatingJob(ctx, wid, time.Minute)
		if err != nil {
			t.Errorf("claim err: %v", err)
			return
		}
		if got != nil {
			atomic.AddInt32(&wins, 1)
		}
	}
	go claim("w-a")
	go claim("w-b")
	wg.Wait()
	if wins != 1 {
		t.Fatalf("want exactly one compensating claim, got %d", wins)
	}
}

func TestPurgeCompletedJobs_SkipsCompensating(t *testing.T) {
	store, ctx := openCompensationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	n, err := store.PurgeCompletedJobs(ctx, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("compensating job purged: %d", n)
	}
	got, _ := store.GetJob(ctx, j.ID)
	if got == nil || got.Status != "compensating" {
		t.Fatalf("job=%+v", got)
	}
}

func TestCompensationSupersededSkipsBranchDestroy(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	jobA := claimOrchJob(t, store, ctx, "w-a")
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	jobB := claimOrchJob(t, store, ctx, "w-b")
	attA := registry.JobDeployAttempt(jobA)
	_ = store.ClaimVersionDeployOperation(ctx, "demo", "v1", attA, time.Minute)
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(jobB.ID),
	})
	attB := registry.JobDeployAttempt(jobB)
	_ = store.ClaimVersionDeployOperation(ctx, "demo", "v1", attB, time.Minute)
	o := &Orchestrator{store: store}
	jobARec, _ := store.GetJob(ctx, jobA.ID)
	if jobARec != nil {
		jobA.ClaimEpoch = jobARec.ClaimEpoch
	}
	done, err := o.runDeployCompensationStateMachine(ctx, "w-a", jobA, compStepBranch, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected superseded terminal")
	}
	rec, _ := store.GetJob(ctx, jobA.ID)
	if rec == nil || rec.Status != "failed" {
		t.Fatalf("old job should terminal safely: %+v", rec)
	}
}

func TestCompensationResourceSuperseded_RecordingBranchDestroyNotCalled(t *testing.T) {
	var destroyCalls int
	old := branchDestroyForCompensation
	branchDestroyForCompensation = func(o *Orchestrator, ctx context.Context, projectID, versionID string) error {
		destroyCalls++
		return old(o, ctx, projectID, versionID)
	}
	t.Cleanup(func() { branchDestroyForCompensation = old })

	store, ctx := openQualificationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	jobA := claimOrchJob(t, store, ctx, "w-a")
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	jobB := claimOrchJob(t, store, ctx, "w-b")
	attA := registry.JobDeployAttempt(jobA)
	_ = store.ClaimVersionDeployOperation(ctx, "demo", "v1", attA, time.Minute)
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(jobB.ID),
	})
	attB := registry.JobDeployAttempt(jobB)
	_ = store.ClaimVersionDeployOperation(ctx, "demo", "v1", attB, time.Minute)
	o := &Orchestrator{store: store}
	jobARec, _ := store.GetJob(ctx, jobA.ID)
	if jobARec != nil {
		jobA.ClaimEpoch = jobARec.ClaimEpoch
	}
	done, err := o.runDeployCompensationStateMachine(ctx, "w-a", jobA, compStepBranch, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("expected superseded terminal")
	}
	if destroyCalls != 0 {
		t.Fatalf("Destroy must not run when resource superseded: calls=%d", destroyCalls)
	}
}

func claimOrchJob(t *testing.T, store registry.Store, ctx context.Context, worker string) *registry.Job {
	t.Helper()
	j, err := store.ClaimJob(ctx, worker, time.Minute)
	if err != nil || j == nil {
		t.Fatalf("claim: %v", err)
	}
	return j
}

func TestDeployWorkerLeaseKeeper_RenewLossStopsRenewals(t *testing.T) {
	oldTick := deployOperationLeaseTick
	deployOperationLeaseTick = 15 * time.Millisecond
	t.Cleanup(func() { deployOperationLeaseTick = oldTick })

	base, ctx := openCompensationTestStore(t)
	store := &failSecondJobRenewStore{Store: base}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	j := claimOrchJob(t, store, ctx, "w1")
	att := registry.JobDeployAttempt(j)
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", att, time.Minute); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	lctx, stop := o.startDeployWorkerLeaseKeeper(ctx, j, "w1", time.Minute)
	defer stop()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if lctx.Err() != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if lctx.Err() == nil {
		t.Fatal("keeper should cancel after renew loss")
	}
}

type failSecondJobRenewStore struct {
	registry.Store
	calls atomic.Int32
}

func (f *failSecondJobRenewStore) RenewClaimedJobLease(ctx context.Context, workerID string, attempt registry.DeployAttempt, lease time.Duration) error {
	if f.calls.Add(1) > 1 {
		return registry.ErrJobLeaseConflict
	}
	return f.Store.RenewClaimedJobLease(ctx, workerID, attempt, lease)
}

func TestCompensation_OpLossSameJobDesireIncomplete(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	j := prepareQualificationJob(t, store, ctx, "", 2)
	att := registry.JobDeployAttempt(j)
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", att, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := store.MarkJobCompensatingForAttempt(ctx, "qual-worker", att); err != nil {
		t.Fatal(err)
	}
	j, _ = store.GetJob(ctx, j.ID)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(j.ID),
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimCompensatingJob(ctx, "qual-worker", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim compensating: %v", err)
	}
	o := &Orchestrator{store: store}
	done, err := o.runDeployCompensationStateMachine(ctx, "qual-worker", claimed, compStepDesire, 2)
	if done || !errors.Is(err, ErrDeployCompensationIncomplete) {
		t.Fatalf("want durable incomplete, done=%v err=%v", done, err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.DesiredReplicas != 1 || d.Reason != deployQualificationReasonForJob(j.ID) {
		t.Fatalf("desire must be unchanged: %+v", d)
	}
	rec, _ := store.GetJob(ctx, j.ID)
	if rec != nil && rec.Status == "failed" {
		t.Fatalf("job must not fail without op: %+v", rec)
	}
}

type errHolderStore struct {
	registry.Store
}

func (e *errHolderStore) GetVersionDeployOperationHolder(context.Context, string, string) (registry.VersionDeployOperationHolder, error) {
	return registry.VersionDeployOperationHolder{}, errors.New("registry read failed")
}

func TestCompensationSupersededCheck_RegistryErrorIncomplete(t *testing.T) {
	base, ctx := openQualificationTestStore(t)
	store := &errHolderStore{Store: base}
	j := &registry.Job{ID: "job-x", ProjectID: "demo", VersionID: "v1"}
	o := &Orchestrator{store: store}
	_, err := o.compensationSuperseded(ctx, j)
	if !errors.Is(err, ErrDeployCompensationIncomplete) {
		t.Fatalf("want incomplete, got %v", err)
	}
}

func TestProcessOne_VersionBusyRequeuesPending(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	jOld, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	_ = store.MarkJobCompensating(ctx, jOld.ID)
	_ = store.ClaimVersionDeployOperation(ctx, "demo", "v1", registry.DeployAttempt{JobID: jOld.ID, ClaimEpoch: 0}, time.Minute)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	o := &Orchestrator{store: store}
	o.processOne(ctx, "busy-worker")
	pending, _ := store.CountPendingJobs(ctx)
	if pending != 1 {
		t.Fatalf("want one pending job after version busy, got %d", pending)
	}
}

func TestAbortWithoutOp_DiscoveredRecoveryCompletesCompensation(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "on")
	store, ctx := openQualificationTestStore(t)
	j := prepareQualificationJob(t, store, ctx, "", 2)
	att := registry.JobDeployAttempt(j)
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", att, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(j.ID),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", att, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	o := &Orchestrator{store: store}
	o.abortDeployJobWithoutOp(ctx, "qual-worker", j, errors.New("deploy lost op"))
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "compensating" {
		t.Fatalf("job should be durably compensating: %+v", rec)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v != nil && v.Status == registry.StatusFailed {
		t.Fatalf("version may remain non-failed until fenced recovery: %+v", v)
	}
	work, err := store.ListDeployQualificationCompensationWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var listed bool
	for _, item := range work {
		if item.JobID == j.ID {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("discovery must find orphan compensating job: %+v", work)
	}
	o.processCompensationOne(ctx, "recovery-worker")
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.DesiredReplicas != 0 || d.Reason != desireReasonDeployFailed {
		t.Fatalf("desire rollback: %+v", d)
	}
	rec, _ = store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "failed" {
		t.Fatalf("job should terminal after recovery: %+v", rec)
	}
}

func TestProcessCompensationOne_DiscoveredPrepareThenRuns(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "on")
	store, ctx := openQualificationTestStore(t)
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(j.ID),
	})
	o := &Orchestrator{store: store}
	o.processCompensationOne(ctx, "w-discover")
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "failed" {
		t.Fatalf("discovered compensation should finish job: %+v", rec)
	}
}

func TestCompensation_ThirdPartyDesireSameOpContinuesFencedCleanup(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "on")
	store, ctx := openQualificationTestStore(t)
	j := prepareQualificationJob(t, store, ctx, "", 2)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 2, Generation: 1, Reason: "activator",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8080,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkJobCompensatingForAttempt(ctx, "qual-worker", registry.JobDeployAttempt(j)); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimCompensatingJob(ctx, "worker-after-restart", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim compensating: %v", err)
	}
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", registry.JobDeployAttempt(claimed), time.Minute); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	done, err := o.runDeployCompensationStateMachine(ctx, "worker-after-restart", claimed, compStepDesire, 12)
	if err != nil || !done {
		t.Fatalf("compensation: done=%v err=%v", done, err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.DesiredReplicas != 2 || d.Reason != "activator" {
		t.Fatalf("third-party desire must remain: %+v", d)
	}
	route, _ := store.GetRoute(ctx, "demo", "v1")
	if route == nil {
		t.Fatal("route row expected")
	}
	if route.Active {
		t.Fatalf("route must be inactive: %+v", route)
	}
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "failed" {
		t.Fatalf("job terminal: %+v", rec)
	}
	holder, _ := store.GetVersionDeployOperationHolder(ctx, "demo", "v1")
	if holder.JobID != "" {
		t.Fatalf("deploy op released: %+v", holder)
	}
}

func TestCompensation_NewOrchestratorWorkerDiscoversWork(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "on")
	store, ctx := openQualificationTestStore(t)
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := store.MarkJobCompensating(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(j.ID),
	})
	oRestart := &Orchestrator{store: store}
	oRestart.processCompensationOne(ctx, "worker-after-restart")
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "failed" {
		t.Fatalf("new worker should complete discovered compensation: %+v", rec)
	}
}

func TestProcessCompensationOne_ReconcileRepairsClaimedInvalidLease(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "on")
	store, ctx := openQualificationTestStore(t)
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	j := prepareQualificationJob(t, store, ctx, "", 2)
	epochBefore := j.ClaimEpoch
	att := registry.JobDeployAttempt(j)
	if err := store.UpdateJobStepForAttempt(ctx, "qual-worker", att, "compensating:desire"); err != nil {
		t.Fatal(err)
	}
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)
	setClaimedCompensatingStepExpiredLease(t, store, ctx, j, "qual-worker")
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(j.ID),
	}); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{store: store}
	o.processCompensationOne(ctx, "recover-worker")
	afterClaim, _ := store.GetJob(ctx, j.ID)
	if afterClaim == nil || afterClaim.ClaimEpoch <= epochBefore {
		t.Fatalf("claim epoch must increase after claim: before=%d after=%+v", epochBefore, afterClaim)
	}
	if afterClaim.Status != "failed" {
		t.Fatalf("compensation should finish: %+v", afterClaim)
	}
}

func TestProcessCompensationOne_StuckInvalidHeadDoesNotBlockSecondJob(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "on")
	store, ctx := openQualificationTestStore(t)
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v2", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v2", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}

	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	jHead, _ := store.ClaimJob(ctx, "w-head", time.Minute)
	if jHead == nil {
		t.Fatal("claim head")
	}
	_ = store.ClaimVersionDeployOperation(ctx, "demo", "v1", registry.JobDeployAttempt(jHead), time.Minute)
	setClaimedCompensatingStepExpiredLease(t, store, ctx, jHead, "w-head")

	jReady, _ := store.EnqueueJob(ctx, "demo", "v2", registry.StatusFetching)
	if err := store.MarkJobCompensating(ctx, jReady.ID); err != nil {
		t.Fatal(err)
	}
	_ = store.CompareAndSetDesired(ctx, "demo", "v2", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(jReady.ID),
	})

	o := &Orchestrator{store: store}
	o.processCompensationOne(ctx, "worker-b")
	rec, _ := store.GetJob(ctx, jReady.ID)
	if rec == nil || rec.Status != "failed" {
		t.Fatalf("second job must complete while head remains blocked: %+v", rec)
	}
	head, _ := store.GetJob(ctx, jHead.ID)
	if head == nil || head.Status != "claimed" {
		t.Fatalf("stuck head unchanged: %+v", head)
	}
}
