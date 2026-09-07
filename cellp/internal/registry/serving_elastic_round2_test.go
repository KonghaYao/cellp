package registry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestClaimWithoutDesireFails(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/no-desire.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = seedElasticClaimBase(ctx, store, "n1")
	_, _ = store.db.Exec(`DELETE FROM serving_desires WHERE project_id = ? AND version_id = ?`, "demo", "v1")
	valid := time.Now().UTC().Add(time.Hour)
	err = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	if !errors.Is(err, ErrAssignmentCASConflict) {
		t.Fatalf("want desire conflict, got %v", err)
	}
}

func TestClaimWrongDesireGeneration(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/bad-desire-gen.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = seedElasticClaimBase(ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	err = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 99, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	if !errors.Is(err, ErrAssignmentCASConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestClaimMissingNode(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/no-node.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = seedElasticClaimBase(ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	err = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "missing",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("want lease expired for missing node, got %v", err)
	}
}

func TestClaimCordonedNode(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/cordoned.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = seedElasticClaimBase(ctx, store, "n1")
	_ = store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1, Cordoned: true,
		LeaseExpiry: time.Now().UTC().Add(time.Hour),
	})
	valid := time.Now().UTC().Add(time.Hour)
	err = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("want cordoned lease fail, got %v", err)
	}
}

func TestClaimSameGenerationMutationRejected(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/claim-mut.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = seedElasticClaimBase(ctx, store, "n1")
	valid := time.Now().UTC().Add(30 * time.Minute)
	claim := AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}
	if err := store.ClaimAssignment(ctx, claim); err != nil {
		t.Fatal(err)
	}
	renewed := valid.Add(time.Minute)
	err = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: renewed,
	})
	if !errors.Is(err, ErrAssignmentCASConflict) {
		t.Fatalf("want conflict on lease renew via claim, got %v", err)
	}
	if err := store.ClaimAssignment(ctx, claim); err != nil {
		t.Fatalf("idempotent claim: %v", err)
	}
}

func TestClaimConcurrentDifferentReplicas(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/claim-race.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = seedElasticClaimBase(ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			errs <- store.ClaimAssignment(ctx, AssignmentClaim{
				ReplicaID: id, ProjectID: "demo", VersionID: "v1", NodeID: "n1",
				Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
			})
		}([]string{"ra", "rb"}[i])
	}
	wg.Wait()
	close(errs)
	var ok int
	for err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 2 {
		t.Fatalf("both claims should succeed for different replica IDs, ok=%d", ok)
	}
}

func TestObservationReplayNoRevisionBump(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/obs-replay.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	mustElasticEnrolledReady(t, ctx, store)
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	epUntil := time.Now().UTC().Add(2 * time.Hour)
	ready := ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9910,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	}
	if err := store.RecordObservation(ctx, ready); err != nil {
		t.Fatal(err)
	}
	rev1, _ := store.GetRouteRevision(ctx)
	if err := store.RecordObservation(ctx, ready); err != nil {
		t.Fatal(err)
	}
	rev2, _ := store.GetRouteRevision(ctx)
	if rev2 != rev1 {
		t.Fatalf("replay should not bump revision: %d -> %d", rev1, rev2)
	}
}

func TestObservationScopeMismatch(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/obs-scope.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_ = seedElasticClaimBase(ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	_ = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	err = store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "other", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	if !errors.Is(err, ErrObservationStale) {
		t.Fatalf("want stale, got %v", err)
	}
}

func TestSnapshotExcludesDrainingAndExpiredReady(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/snap-filter.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	mustElasticEnrolledReady(t, ctx, store)
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r-drain", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	epUntil := time.Now().UTC().Add(time.Hour)
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r-drain", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r-drain", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9919,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	}); err != nil {
		t.Fatalf("ready: %v", err)
	}
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r-drain", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaDraining,
		ListenHost: "127.0.0.1", ListenPort: 9920, EndpointState: contract.EndpointDraining,
		EndpointValidUntil: &epUntil,
	}); err != nil {
		t.Fatalf("draining: %v", err)
	}
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 0 {
		t.Fatalf("draining must not appear in public snapshot: %+v", snap.EndpointSets)
	}
}

func TestQualificationViewDeployReady(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/qual.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, ElasticEnrolled: true,
		BackgroundMode: contract.BackgroundModeNone,
	}); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	epUntil := time.Now().UTC().Add(time.Hour)
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9930,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	pub, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.EndpointSets) != 0 {
		t.Fatalf("deploy_ready must stay off public snapshot")
	}
	rev, _ := store.GetRouteRevision(ctx)
	q, ok, err := store.BuildQualificationViewAfter(ctx, rev-1)
	if err != nil || !ok || len(q.EndpointSets) != 1 {
		t.Fatalf("qualification view: ok=%v err=%v sets=%+v", ok, err, q.EndpointSets)
	}
}

func TestIngressUpsertNoOpSkipsRouteRevision(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/ingress-noop.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	host := "v1.demo.ingress.local"
	b := IngressBinding{
		BindingID: "b1", ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, Host: &host, SyntheticHost: "synth.ingress.local", Active: true,
	}
	if err := store.UpsertIngressBinding(ctx, b); err != nil {
		t.Fatal(err)
	}
	rev1, _ := store.GetRouteRevision(ctx)
	if err := store.UpsertIngressBinding(ctx, b); err != nil {
		t.Fatal(err)
	}
	rev2, _ := store.GetRouteRevision(ctx)
	if rev2 != rev1 {
		t.Fatalf("no-op ingress upsert should not bump route revision")
	}
}
