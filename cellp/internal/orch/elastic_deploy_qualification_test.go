package orch

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

func openQualificationTestStore(t *testing.T) (registry.Store, context.Context) {
	t.Helper()
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "qual.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	return store, ctx
}

func testJob(id, projectID, versionID string) *registry.Job {
	return &registry.Job{ID: id, ProjectID: projectID, VersionID: versionID}
}

func prepareQualificationJob(t *testing.T, store registry.Store, ctx context.Context, _ string, maxReplicas int) *registry.Job {
	t.Helper()
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: maxReplicas, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	j, err := store.ClaimJob(ctx, "qual-worker", jobLease)
	if err != nil || j == nil {
		t.Fatal(err)
	}
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", registry.JobDeployAttempt(j), jobLease); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestEnsureDeployQualificationDesire_IdempotentSameJob(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	o := &Orchestrator{store: store}
	j := prepareQualificationJob(t, store, ctx, "job-a", 2)

	if err := o.ensureDeployQualificationDesire(ctx, j); err != nil {
		t.Fatal(err)
	}
	d1, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d1 == nil || d1.DesiredReplicas != 1 || d1.Reason != deployQualificationReasonForJob(j.ID) {
		t.Fatalf("first ensure: %+v", d1)
	}

	if err := o.ensureDeployQualificationDesire(ctx, j); err != nil {
		t.Fatal(err)
	}
	d2, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d2.Generation != d1.Generation {
		t.Fatalf("retry should be idempotent: gen %d -> %d", d1.Generation, d2.Generation)
	}
}

func TestEnsureDeployQualificationDesire_TakeoverActivatorIntent(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 2, Generation: 1, Reason: "activator_ensure",
	}); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	j := prepareQualificationJob(t, store, ctx, "job-b", 3)
	if err := o.ensureDeployQualificationDesire(ctx, j); err != nil {
		t.Fatal(err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.DesiredReplicas != 2 || d.Reason != deployQualificationReasonForJob(j.ID) || d.Generation != 2 {
		t.Fatalf("takeover: %+v", d)
	}
}

func TestEnsureDeployQualificationDesire_TakeoverStaleQualificationJob(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob("old-job"),
	}); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	j := prepareQualificationJob(t, store, ctx, "new-job", 2)
	if err := o.ensureDeployQualificationDesire(ctx, j); err != nil {
		t.Fatal(err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d.Reason != deployQualificationReasonForJob(j.ID) || d.Generation != 2 {
		t.Fatalf("new deploy job should fence stale qualification: %+v", d)
	}
}

func TestEnsureDeployQualificationDesire_LegacyReasonNotSilentReuse(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "deploy_qualification",
	}); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	j := prepareQualificationJob(t, store, ctx, "job-c", 2)
	if err := o.ensureDeployQualificationDesire(ctx, j); !errors.Is(err, ErrDeployCompensationIncomplete) {
		t.Fatalf("legacy reason must not be guessed by arbitrary job: %v", err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d.Reason != "deploy_qualification" {
		t.Fatalf("legacy reason must remain until safe recovery: %+v", d)
	}
}

func TestRollbackElasticQualificationDesire_Owned(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	j := prepareQualificationJob(t, store, ctx, "", 2)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(j.ID),
	}); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	if err := o.rollbackElasticQualificationDesire(ctx, "demo", "v1", registry.JobDeployAttempt(j)); err != nil {
		t.Fatal(err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.DesiredReplicas != 0 || d.Reason != desireReasonDeployFailed || d.Generation != 2 {
		t.Fatalf("rollback: %+v", d)
	}
}

func TestRollbackElasticQualificationDesire_ConcurrentOwnerChangeNoOverwrite(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	jobA := "job-a"
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(jobA),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 1, registry.ServingDesireRow{
		DesiredReplicas: 3, Generation: 2, Reason: "scale-up",
	}); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	if err := o.rollbackElasticQualificationDesire(ctx, "demo", "v1", registry.DeployAttempt{JobID: jobA, ClaimEpoch: 0}); err != nil {
		t.Fatal(err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d.DesiredReplicas != 3 || d.Reason != "scale-up" {
		t.Fatalf("stale compensation must not clobber newer intent: %+v", d)
	}
}

type casConflictDesireStore struct {
	row *registry.ServingDesireRow
}

func (s *casConflictDesireStore) GetServingDesire(context.Context, string, string) (*registry.ServingDesireRow, error) {
	return s.row, nil
}

func (s *casConflictDesireStore) CompareAndSetDesired(context.Context, string, string, int64, registry.ServingDesireRow) error {
	return registry.ErrDesiredCASConflict
}

func TestRollbackElasticQualificationDesire_CASConflictReturnsError(t *testing.T) {
	jobID := "job-cas"
	fake := &casConflictDesireStore{row: &registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 4, Reason: deployQualificationReasonForJob(jobID),
	}}
	err := rollbackElasticQualificationDesireOnStore(context.Background(), fake, "demo", "v1", jobID)
	if !errors.Is(err, ErrQualificationDesireRollbackIncomplete) {
		t.Fatalf("want rollback incomplete, got %v", err)
	}
}

func TestEnsureDeployQualificationDesire_ClampsAboveMax(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 9, Generation: 1, Reason: "activator_ensure",
	}); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store}
	j := prepareQualificationJob(t, store, ctx, "job-max", 1)
	if err := o.ensureDeployQualificationDesire(ctx, j); err != nil {
		t.Fatal(err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.DesiredReplicas != 1 {
		t.Fatalf("want clamped desired=1, got %+v", d)
	}
}

func TestRollbackElasticQualificationDesire_EmptyJobIDFailClosed(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	o := &Orchestrator{store: store}
	if err := o.rollbackElasticQualificationDesire(ctx, "demo", "v1", registry.DeployAttempt{}); err == nil {
		t.Fatal("expected error for empty job id")
	}
}

func TestWaitQualificationHealth_RespectsContextCancel(t *testing.T) {
	o, _, ctx := newTestOrch(t)
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	if err := o.waitQualificationHealth(ctx, "127.0.0.1", 1); err == nil {
		t.Fatal("expected context error")
	}
}

func TestRollbackElasticQualificationDesire_NotOwnerIncomplete(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	j := prepareQualificationJob(t, store, ctx, "", 2)
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(j.ID),
	}); err != nil {
		t.Fatal(err)
	}
	att := registry.JobDeployAttempt(j)
	_ = store.ReleaseVersionDeployOperation(ctx, "demo", "v1", att)
	o := &Orchestrator{store: store}
	if err := o.rollbackElasticQualificationDesire(ctx, "demo", "v1", att); !errors.Is(err, ErrQualificationDesireRollbackIncomplete) {
		t.Fatalf("want rollback incomplete, got %v", err)
	}
}
