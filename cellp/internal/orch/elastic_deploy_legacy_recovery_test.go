package orch

import (
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestReconcileLegacyDeployQualificationRecovery_Unique(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	j, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	_ = store.FailJob(ctx, j.ID)
	failMsg := "boom"
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusFailed, &failMsg)
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: desireReasonDeployQualificationLegacy,
	})
	o := &Orchestrator{store: store}
	o.reconcileDiscoveredDeployCompensation(ctx)
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.Reason != deployQualificationReasonForJob(j.ID) {
		t.Fatalf("normalized desire: %+v", d)
	}
}
