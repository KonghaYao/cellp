package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestUpsertServingPolicyRollsBackOnHookFailure(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/policy-tx.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})

	injected := errors.New("inject step 3")
	upsertServingPolicyTestHook = func(step int) error {
		if step == 3 {
			return injected
		}
		return nil
	}
	defer func() { upsertServingPolicyTestHook = nil }()

	err = store.UpsertServingPolicy(ctx, ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	})
	if !errors.Is(err, injected) {
		t.Fatalf("want injected err, got %v", err)
	}
	pol, err := store.GetServingPolicy(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if pol != nil {
		t.Fatalf("policy should rollback, got %+v", pol)
	}
	snap0, _ := store.BuildLegacyRouteSnapshot(ctx)
	snap1, _ := store.BuildLegacyRouteSnapshot(ctx)
	if snap1.PolicyRevision != snap0.PolicyRevision {
		t.Fatal("policy_revision should not advance on rollback")
	}
}
