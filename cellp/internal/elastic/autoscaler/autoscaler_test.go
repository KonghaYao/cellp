package autoscaler

import (
	"context"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

func TestTick_disabledByDefault(t *testing.T) {
	t.Setenv(contract.EnvElasticRuntime, "")
	store, err := registry.Open(t.TempDir() + "/as.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	loop := &Loop{Store: RegistryStore{ServingStore: store}}
	rep, err := loop.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Skipped {
		t.Fatalf("want skipped when flag off, got %+v", rep)
	}
}

func TestTick_comparesDesiredVsReady(t *testing.T) {
	t.Setenv(contract.EnvElasticRuntime, "1")
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/gap.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 3, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 2, Generation: 1, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRuntimeReplica(ctx, contract.RuntimeReplica{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
	}); err != nil {
		t.Fatal(err)
	}
	loop := &Loop{Store: RegistryStore{ServingStore: store}}
	rep, err := loop.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Skipped || len(rep.Gaps) != 1 {
		t.Fatalf("gaps: %+v skipped=%v", rep.Gaps, rep.Skipped)
	}
	if rep.Gaps[0].Gap != 1 || rep.Gaps[0].DesiredReplicas != 2 || rep.Gaps[0].ReadyReplicas != 1 {
		t.Fatalf("gap: %+v", rep.Gaps[0])
	}
}

func TestCountReadyReplicas(t *testing.T) {
	reps := []contract.RuntimeReplica{
		{State: contract.ReplicaReady},
		{State: contract.ReplicaStarting},
		{State: contract.ReplicaReady},
	}
	if n := CountReadyReplicas(reps); n != 2 {
		t.Fatalf("ready=%d", n)
	}
}

func TestValidateServingPolicyBackground_resident(t *testing.T) {
	p := contract.ServingPolicy{
		MinReplicas: 0, MaxReplicas: 2, BackgroundMode: contract.BackgroundModeResidentRequired,
	}
	if err := contract.ValidateServingPolicyBackground(p, contract.BackgroundGuardOptions{}); err == nil {
		t.Fatal("expected min>=1 for resident")
	}
	p.MinReplicas = 1
	p.MaxReplicas = 2
	if err := contract.ValidateServingPolicyBackground(p, contract.BackgroundGuardOptions{}); err == nil {
		t.Fatal("expected max=1 without SP-E3")
	}
	p.MaxReplicas = 1
	if err := contract.ValidateServingPolicyBackground(p, contract.BackgroundGuardOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertServingPolicy_backgroundGuard(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/bg.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	err = store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, BackgroundMode: contract.BackgroundModeResidentRequired,
		ElasticEnrolled: true,
	})
	if err == nil {
		t.Fatal("expected resident guard on upsert")
	}
}
