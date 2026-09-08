package orch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

type errGetVersionStore struct {
	registry.Store
}

func (e *errGetVersionStore) GetVersion(context.Context, string, string) (*registry.Version, error) {
	return nil, errors.New("version read failed")
}

type failMarkCompensatingStore struct {
	registry.Store
}

func (f *failMarkCompensatingStore) MarkJobCompensatingForAttempt(context.Context, string, registry.DeployAttempt) error {
	return registry.ErrJobLeaseConflict
}

func (f *failMarkCompensatingStore) MarkJobCompensating(context.Context, string) error {
	return errors.New("mark compensating failed")
}

func TestDeployFailureNeedsCompensation_RegistryUncertainty(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	wrapped := &errGetVersionStore{Store: store}
	o := &Orchestrator{store: wrapped}
	j := &registry.Job{ID: "j1", ProjectID: "demo", VersionID: "v1"}
	needs, err := o.deployFailureNeedsCompensation(ctx, j)
	if needs || !errors.Is(err, ErrDeployCompensationIncomplete) {
		t.Fatalf("want incomplete uncertainty, needs=%v err=%v", needs, err)
	}
}

func TestAbortWithoutOp_MarkCompensatingFailureKeepsClaimed(t *testing.T) {
	base, ctx := openQualificationTestStore(t)
	store := &failMarkCompensatingStore{Store: base}
	j := prepareQualificationJob(t, store, ctx, "", 2)
	_ = store.ClaimVersionDeployOperation(ctx, "demo", "v1", registry.JobDeployAttempt(j), time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	o := &Orchestrator{store: store}
	o.abortDeployJobWithoutOp(ctx, "wrong-worker", j, errors.New("boom"))
	rec, _ := store.GetJob(ctx, j.ID)
	if rec == nil || rec.Status != "claimed" {
		t.Fatalf("must stay claimed until lease expiry for discovery: %+v", rec)
	}
}
