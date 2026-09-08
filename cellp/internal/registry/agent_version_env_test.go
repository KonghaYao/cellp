package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestGetAuthorizedAgentVersionEnvLiveAssignmentStates(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/auth.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")

	now := time.Now().UTC()
	for _, state := range []contract.ReplicaState{contract.ReplicaPending, contract.ReplicaStarting, contract.ReplicaReady} {
		t.Run(string(state), func(t *testing.T) {
			if err := setReplicaState(t, store, "r1", state); err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now); err != nil {
				t.Fatalf("state %s should authorize: %v", state, err)
			}
		})
	}
}

func TestGetAuthorizedAgentVersionEnvRejectsTerminalAndDraining(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, *SQLiteStore)
	}{
		{
			name: "stopped",
			setup: func(t *testing.T, store *SQLiteStore) {
				seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
				if err := store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaStopped); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "failed",
			setup: func(t *testing.T, store *SQLiteStore) {
				seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
				if err := store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaFailed); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "draining",
			setup: func(t *testing.T, store *SQLiteStore) {
				seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
				if err := markReplicaReady(t, store, "r1"); err != nil {
					t.Fatal(err)
				}
				if err := markReplicaDraining(t, store, "r1"); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir() + "/" + tc.name + ".sqlite")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			tc.setup(t, store)
			if _, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now); !errors.Is(err, ErrAgentVersionEnvForbidden) {
				t.Fatalf("expected forbidden, got %v", err)
			}
		})
	}
}

func TestGetAuthorizedAgentVersionEnvRejectsExpiredAssignmentAndCordon(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	store, err := Open(t.TempDir() + "/exp.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	past := now.Add(-time.Minute)
	if err := store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1,
		State: contract.ReplicaPending, AssignmentValidUntil: &past,
	}); err == nil {
		t.Fatal("expected stale observation when shrinking valid_until")
	}
	// Advance authorization clock past the live assignment lease.
	if _, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now.Add(3*time.Hour)); !errors.Is(err, ErrAgentVersionEnvForbidden) {
		t.Fatalf("expired assignment: %v", err)
	}

	store2, err := Open(t.TempDir() + "/cordon.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	seedAuthorizeEnvFixture(t, store2, "n1", "demo", "v1", "r1")
	if err := store2.CordonRuntimeNode(ctx, "n1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store2.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now); !errors.Is(err, ErrAgentVersionEnvForbidden) {
		t.Fatalf("cordoned node: %v", err)
	}
}

func TestGetAuthorizedAgentVersionEnvRejectsGenerationMismatch(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/gen.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	now := time.Now().UTC()
	exp := now.Add(2 * time.Hour)
	if err := store.ActivateRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 2, LeaseExpiry: exp,
		AgentBaseURL: "https://n1", IdentityURI: "spiffe://test/n1", Zone: "z",
	}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now); !errors.Is(err, ErrAgentVersionEnvForbidden) {
		t.Fatalf("generation mismatch: %v", err)
	}
}

func TestGetAuthorizedAgentVersionEnvRejectsLegacyAssignedNodeGenerationZero(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/legacy-gen0.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	if _, err := store.db.Exec(`UPDATE runtime_replicas SET assigned_node_generation = 0 WHERE replica_id = ?`, "r1"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
	if !errors.Is(err, ErrAgentVersionEnvForbidden) {
		t.Fatalf("legacy assigned_node_generation=0: %v", err)
	}
}

func TestGetAuthorizedAgentVersionEnvRejectsWrongProjectAndVersion(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/scope.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	now := time.Now().UTC()
	if _, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "other", "v1", now); !errors.Is(err, ErrAgentVersionEnvForbidden) {
		t.Fatalf("wrong project: %v", err)
	}
	if _, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "missing", now); !errors.Is(err, ErrAgentVersionEnvForbidden) {
		t.Fatalf("wrong version: %v", err)
	}
}

func TestGetAuthorizedAgentVersionEnvRejectsExpiredNodeLease(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/lease.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	node, err := store.GetRuntimeNode(ctx, "n1")
	if err != nil || node == nil {
		t.Fatal(err)
	}
	node.LeaseExpiry = past
	if err := store.UpsertRuntimeNode(ctx, *node); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now); !errors.Is(err, ErrAgentVersionEnvForbidden) {
		t.Fatalf("expired node lease: %v", err)
	}
}

func seedAuthorizeEnvFixture(t *testing.T, store *SQLiteStore, nodeID, projectID, versionID, replicaID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, CreateProjectInput{ID: projectID}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, CreateVersionInput{ID: versionID, ProjectID: projectID}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, projectID, versionID, 0, ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "seed",
	}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(2 * time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: nodeID, CapacityUnits: 2, Generation: 1, LeaseExpiry: exp,
		AgentBaseURL: "https://" + nodeID, IdentityURI: "spiffe://test/" + nodeID, Zone: "z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: replicaID, ProjectID: projectID, VersionID: versionID, NodeID: nodeID,
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: exp,
	}); err != nil {
		t.Fatal(err)
	}
}

func setReplicaState(t *testing.T, store *SQLiteStore, replicaID string, state contract.ReplicaState) error {
	t.Helper()
	ctx := context.Background()
	rep, err := store.GetRuntimeReplica(ctx, replicaID)
	if err != nil || rep == nil || rep.ValidUntil == nil {
		t.Fatalf("replica %s: %+v err=%v", replicaID, rep, err)
	}
	exp := *rep.ValidUntil
	switch state {
	case contract.ReplicaStarting:
		return store.RecordObservation(ctx, ReplicaObservation{
			ReplicaID: replicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, NodeID: rep.NodeID,
			Generation: rep.Generation, State: state, AssignmentValidUntil: &exp,
		})
	case contract.ReplicaReady:
		return markReplicaReady(t, store, replicaID)
	default:
		return store.RecordObservation(ctx, ReplicaObservation{
			ReplicaID: replicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, NodeID: rep.NodeID,
			Generation: rep.Generation, State: state, AssignmentValidUntil: &exp,
		})
	}
}

func markReplicaReady(t *testing.T, store *SQLiteStore, replicaID string) error {
	t.Helper()
	ctx := context.Background()
	rep, err := store.GetRuntimeReplica(ctx, replicaID)
	if err != nil || rep == nil || rep.ValidUntil == nil {
		t.Fatalf("replica %s: %+v err=%v", replicaID, rep, err)
	}
	exp := *rep.ValidUntil
	if rep.State != contract.ReplicaStarting {
		if err := store.RecordObservation(ctx, ReplicaObservation{
			ReplicaID: replicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, NodeID: rep.NodeID,
			Generation: rep.Generation, State: contract.ReplicaStarting, AssignmentValidUntil: &exp,
		}); err != nil {
			return err
		}
	}
	return store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: replicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, NodeID: rep.NodeID,
		Generation: rep.Generation, State: contract.ReplicaReady, ListenHost: "127.0.0.1", ListenPort: 9101,
		EndpointState: contract.EndpointReady, AssignmentValidUntil: &exp, EndpointValidUntil: &exp,
	})
}

func markReplicaDraining(t *testing.T, store *SQLiteStore, replicaID string) error {
	t.Helper()
	ctx := context.Background()
	rep, err := store.GetRuntimeReplica(ctx, replicaID)
	if err != nil || rep == nil || rep.ValidUntil == nil {
		t.Fatalf("replica %s: %+v err=%v", replicaID, rep, err)
	}
	exp := *rep.ValidUntil
	return store.RecordObservation(ctx, ReplicaObservation{
		ReplicaID: replicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, NodeID: rep.NodeID,
		Generation: rep.Generation, State: contract.ReplicaDraining, ListenHost: "127.0.0.1", ListenPort: 9101,
		EndpointState: contract.EndpointDraining, AssignmentValidUntil: &exp, EndpointValidUntil: &exp,
	})
}
