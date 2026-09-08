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

func TestUpsertRuntimeNodeRejectsGenerationRollback(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/node-gen.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exp := time.Now().UTC().Add(time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 5, LeaseExpiry: exp,
	}); err != nil {
		t.Fatal(err)
	}
	err = store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 9, Generation: 1, LeaseExpiry: exp,
	})
	if !errors.Is(err, ErrNodeLeaseCASConflict) {
		t.Fatalf("want cas conflict, got %v", err)
	}
	got, err := store.GetRuntimeNode(ctx, "n1")
	if err != nil || got == nil || got.Generation != 5 || got.CapacityUnits != 2 {
		t.Fatalf("row unchanged: %+v err=%v", got, err)
	}
}

func TestUpsertRuntimeNodeAllowsSameGenerationHeartbeat(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/node-heartbeat.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exp := time.Now().UTC().Add(2 * time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 3, LeaseExpiry: exp,
	}); err != nil {
		t.Fatal(err)
	}
	newExp := exp.Add(time.Minute)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 3, LeaseExpiry: newExp,
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRuntimeNode(ctx, "n1")
	if got.CapacityUnits != 4 || got.Generation != 3 {
		t.Fatalf("heartbeat update: %+v", got)
	}
}

func TestEnrolledVersionSkipsLegacyRouteWithoutElasticEndpoints(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/legacy-skip.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateProject(ctx, CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	mustElasticEnrolledReady(t, ctx, store)
	if err := store.SetRoute(ctx, Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8800,
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 0 {
		t.Fatalf("enrolled version must not fall back to legacy route: %+v", snap.EndpointSets)
	}
}

func TestQualificationViewExcludesNonEnrolledDeployReady(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/qual-non-enrolled.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusDeployReady, nil); err != nil {
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
		ListenHost: "127.0.0.1", ListenPort: 9940,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	rev, _ := store.GetRouteRevision(ctx)
	q, ok, err := store.BuildQualificationViewAfter(ctx, rev-1)
	if err != nil {
		t.Fatal(err)
	}
	if ok && len(q.EndpointSets) > 0 {
		t.Fatalf("non-enrolled deploy_ready must not qualify: %+v", q.EndpointSets)
	}
}

func TestSnapshotCorruptNodeLeaseFailsClosed(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/snap-corrupt-node.sqlite")
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
		ListenHost: "127.0.0.1", ListenPort: 9950,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	if _, err := store.db.Exec(`UPDATE runtime_nodes SET lease_expiry = ? WHERE node_id = ?`, "not-a-time", "n1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.BuildLegacyRouteSnapshot(ctx)
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("want corrupt node lease error, got %v", err)
	}
}

func TestObservationStaleOnNodeGenerationDrift(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/obs-node-gen.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	mustUpsertRuntimeNode(t, ctx, store, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 2, LeaseExpiry: valid,
	})
	err = store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	if !errors.Is(err, ErrObservationStale) {
		t.Fatalf("want stale on node gen drift, got %v", err)
	}
}

func TestObservationFailsOnExpiredNodeLease(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/obs-node-lease.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	past := time.Now().UTC().Add(-time.Minute)
	mustUpsertRuntimeNode(t, ctx, store, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1, LeaseExpiry: past,
	})
	err = store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("want expired node lease, got %v", err)
	}
}

func TestClaimTerminalReplicaIDReuseRejected(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/claim-terminal.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaFailed,
	})
	err = store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	if !errors.Is(err, ErrAssignmentCASConflict) {
		t.Fatalf("want terminal reuse conflict, got %v", err)
	}
}

func TestClaimSameGenerationDoesNotResetReplicaState(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/claim-no-reset.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	claim := AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}
	mustClaimAssignment(t, ctx, store, claim)
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	if err := store.ClaimAssignment(ctx, claim); err != nil {
		t.Fatalf("idempotent claim: %v", err)
	}
	rep, err := store.GetRuntimeReplica(ctx, "r1")
	if err != nil || rep == nil || rep.State != contract.ReplicaStarting {
		t.Fatalf("state must not reset to pending: %+v err=%v", rep, err)
	}
}

func TestClaimConcurrentSameReplicaSingleWinner(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/claim-same-race.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	validA := time.Now().UTC().Add(time.Hour)
	validB := validA.Add(time.Minute)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	claims := []AssignmentClaim{
		{
			ReplicaID: "r-same", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
			Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: validA,
		},
		{
			ReplicaID: "r-same", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
			Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: validB,
		},
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(c AssignmentClaim) {
			defer wg.Done()
			errs <- store.ClaimAssignment(ctx, c)
		}(claims[i])
	}
	wg.Wait()
	close(errs)
	var ok, conflict int
	for err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrAssignmentCASConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("want one winner one conflict, ok=%d conflict=%d", ok, conflict)
	}
}

func TestSnapshotExcludesExpiredReadyEndpoint(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/snap-expired-ready.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	mustElasticEnrolledReady(t, ctx, store)
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r-exp", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	past := time.Now().UTC().Add(-time.Minute)
	epUntil := time.Now().UTC().Add(2 * time.Hour)
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r-exp", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r-exp", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9960,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	if _, err := store.db.Exec(`UPDATE runtime_replica_endpoints SET valid_until = ? WHERE replica_id = ?`,
		past.Format(time.RFC3339Nano), "r-exp"); err != nil {
		t.Fatal(err)
	}
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 0 {
		t.Fatalf("expired ready endpoint must be excluded: %+v", snap.EndpointSets)
	}
}

func TestSnapshotCorruptAssignmentTimestampFailsClosed(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/snap-corrupt-assign.sqlite")
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
		ListenHost: "127.0.0.1", ListenPort: 9951,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	if _, err := store.db.Exec(`UPDATE runtime_replicas SET valid_until = ? WHERE replica_id = ?`, "bad-assign", "r1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.BuildLegacyRouteSnapshot(ctx)
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("want corrupt assignment error, got %v", err)
	}
}

func TestSnapshotCorruptEndpointTimestampFailsClosed(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/snap-corrupt-ep.sqlite")
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
		ListenHost: "127.0.0.1", ListenPort: 9952,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	})
	if _, err := store.db.Exec(`UPDATE runtime_replica_endpoints SET valid_until = ? WHERE replica_id = ?`, "bad-ep", "r1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.BuildLegacyRouteSnapshot(ctx)
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("want corrupt endpoint error, got %v", err)
	}
}

func TestObservationReplayFailsOnCorruptStoredEndpoint(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/obs-corrupt-ep.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSeedElasticClaimBase(t, ctx, store, "n1")
	valid := time.Now().UTC().Add(time.Hour)
	mustClaimAssignment(t, ctx, store, AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	})
	epUntil := time.Now().UTC().Add(time.Hour)
	ready := ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 9970,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	}
	mustRecordObservation(t, ctx, store, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	})
	mustRecordObservation(t, ctx, store, ready)
	if _, err := store.db.Exec(`UPDATE runtime_replica_endpoints SET endpoint_state = ? WHERE replica_id = ?`,
		"bogus", "r1"); err != nil {
		t.Fatal(err)
	}
	err = store.RecordObservation(ctx, ready)
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("want corrupt endpoint error on replay, got %v", err)
	}
}
