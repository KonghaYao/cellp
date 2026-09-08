package registry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestIPv6ElasticSnapshotAndQualificationUseBracketedAddress(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/ipv6-snapshot.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, ServingPolicyRow{ProjectID: "demo", VersionID: "v1", Revision: 1, MaxReplicas: 1, ElasticEnrolled: true, BackgroundMode: contract.BackgroundModeNone}); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{ReplicaID: "r6", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid})
	mustRecordObservation(t, ctx, store, ReplicaObservation{ReplicaID: "r6", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, State: contract.ReplicaStarting})
	mustRecordObservation(t, ctx, store, ReplicaObservation{ReplicaID: "r6", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, State: contract.ReplicaReady, ListenHost: "::1", ListenPort: 9986, EndpointState: contract.EndpointReady, EndpointValidUntil: &valid})

	view, ok, err := store.BuildQualificationViewAfter(ctx, -1)
	if err != nil || !ok || len(view.EndpointSets) != 1 || view.EndpointSets[0].Endpoints[0].Address != "[::1]:9986" {
		t.Fatalf("qualification: %+v ok=%v err=%v", view.EndpointSets, ok, err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil || len(snap.EndpointSets) != 1 || snap.EndpointSets[0].Endpoints[0].Address != "[::1]:9986" {
		t.Fatalf("snapshot: %+v err=%v", snap.EndpointSets, err)
	}
}

func TestSnapshotLKGValidUntilIsEarliestLease(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/lkg-min.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	mustElasticEnrolledReady(t, ctx, store)

	now := time.Now().UTC()
	nodeExp := now.Add(72 * time.Hour)
	assignExp := now.Add(12 * time.Hour)
	epExp := now.Add(24 * time.Hour)
	mustUpsertRuntimeNode(t, ctx, store, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, LeaseExpiry: nodeExp,
	})
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r-lkg", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: assignExp,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r-lkg", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r-lkg", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9981,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epExp,
	})

	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 1 || len(snap.EndpointSets[0].Endpoints) != 1 {
		t.Fatalf("want one endpoint, got %+v", snap.EndpointSets)
	}
	got := snap.EndpointSets[0].Endpoints[0].ValidUntil
	if got == nil {
		t.Fatal("ValidUntil required")
	}
	want := assignExp.UTC().Format(time.RFC3339Nano)
	if got.UTC().Format(time.RFC3339Nano) != want {
		t.Fatalf("LKG ValidUntil want %s got %s", want, got.Format(time.RFC3339Nano))
	}
}

func TestSnapshotExcludesWhenEarliestLeaseExpires(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/lkg-expire.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	mustElasticEnrolledReady(t, ctx, store)

	now := time.Now().UTC()
	nodeExp := now.Add(48 * time.Hour)
	assignExp := now.Add(36 * time.Hour)
	epExp := now.Add(30 * time.Hour)
	mustUpsertRuntimeNode(t, ctx, store, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, LeaseExpiry: nodeExp,
	})
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r-exp", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: assignExp,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r-exp", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r-exp", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9982,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epExp,
	})
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil || len(snap.EndpointSets) != 1 {
		t.Fatalf("pre-expire snapshot: err=%v sets=%+v", err, snap.EndpointSets)
	}

	past := now.Add(-time.Hour)
	if _, err := store.db.Exec(`UPDATE runtime_replicas SET valid_until = ? WHERE replica_id = ?`,
		past.Format(time.RFC3339Nano), "r-exp"); err != nil {
		t.Fatal(err)
	}
	snap, err = store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 0 {
		t.Fatalf("expired assignment (LKG) must exclude route: %+v", snap.EndpointSets)
	}
}

func TestSnapshotUnknownEndpointStateFailsClosed(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/unknown-ep-state.sqlite")
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
	epUntil := time.Now().UTC().Add(time.Hour)
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9983,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	if _, err := store.db.Exec(`UPDATE runtime_replica_endpoints SET endpoint_state = ? WHERE replica_id = ?`,
		"bogus", "r1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.BuildLegacyRouteSnapshot(ctx)
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("want corrupt endpoint state error, got %v", err)
	}
}

func TestCordonedNodeAllowsDrainingThenRemovesPublicRoute(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/cordon-drain.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	mustElasticEnrolledReady(t, ctx, store)
	valid := time.Now().UTC().Add(2 * time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	epUntil := valid
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9984,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil || len(snap.EndpointSets) != 1 {
		t.Fatalf("ready route: err=%v sets=%+v", err, snap.EndpointSets)
	}
	revBefore, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mustUpsertRuntimeNode(t, ctx, store, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, Cordoned: true, LeaseExpiry: valid,
	})
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaDraining,
		ListenHost: "127.0.0.1", ListenPort: 9984, EndpointState: contract.EndpointDraining,
		EndpointValidUntil: &epUntil,
	}); err != nil {
		t.Fatalf("draining on cordoned node: %v", err)
	}
	revAfter, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if revAfter <= revBefore {
		t.Fatalf("draining must bump revision: %d -> %d", revBefore, revAfter)
	}
	snap, err = store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 0 {
		t.Fatalf("draining endpoint must not be public: %+v", snap.EndpointSets)
	}
}

func TestCordonedNodeRejectsReadyObservation(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/cordon-ready.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	valid := time.Now().UTC().Add(2 * time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	epUntil := valid
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9985,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	mustUpsertRuntimeNode(t, ctx, store, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, Cordoned: true, LeaseExpiry: valid,
	})
	ready := ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9985,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	}
	err = store.RecordObservation(ctx, ready)
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("cordon blocks ready replay (fail-closed admission), got %v", err)
	}
}

func TestUpsertRuntimeNodeConcurrentGenerationMonotonic(t *testing.T) {
	path := t.TempDir() + "/node-concurrent.sqlite"
	seed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(24 * time.Hour)
	if err := seed.UpsertRuntimeNode(context.Background(), contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 50, LeaseExpiry: exp,
	}); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	storeA, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()
	storeB, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func(s *SQLiteStore) {
			defer wg.Done()
			_ = s.UpsertRuntimeNode(context.Background(), contract.RuntimeNode{
				NodeID: "n1", CapacityUnits: 1, Generation: 49, LeaseExpiry: exp,
			})
		}(storeA)
		go func(s *SQLiteStore) {
			defer wg.Done()
			_ = s.UpsertRuntimeNode(context.Background(), contract.RuntimeNode{
				NodeID: "n1", CapacityUnits: 1, Generation: 51, LeaseExpiry: exp,
			})
		}(storeB)
	}
	wg.Wait()

	got, err := storeA.GetRuntimeNode(context.Background(), "n1")
	if err != nil || got == nil {
		t.Fatalf("node: %+v err=%v", got, err)
	}
	if got.Generation != 51 {
		t.Fatalf("generation must not roll back under concurrent stale upserts, got %d", got.Generation)
	}
}

func TestDesiredGenerationBumpDoesNotInvalidateAssignedReplica(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/desire-bump.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	valid := time.Now().UTC().Add(2 * time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 1, ServingDesireRow{
		DesiredReplicas: 2, Generation: 2, Reason: "scale",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9986,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &valid,
	}); err != nil {
		t.Fatalf("assigned replica at command gen 1 still observable after desire bump: %v", err)
	}
	err = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r2", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	if !errors.Is(err, ErrAssignmentCASConflict) {
		t.Fatalf("new claim at stale command generation must fail, got %v", err)
	}
}
