package transport_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

func TestRelayWireRuntimeNodeFieldsOverTLS(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	node, err := env.remote.GetRuntimeNode(ctx, "n1")
	if err != nil || node == nil || node.AgentBaseURL == "" || node.IdentityURI == "" || node.Zone == "" {
		t.Fatalf("node metadata: %+v err=%v", node, err)
	}
	rep, err := env.remote.GetRuntimeReplica(ctx, "r1")
	if err != nil || rep == nil || rep.AssignedNodeGeneration != 1 {
		t.Fatalf("replica fields: %+v err=%v", rep, err)
	}
}

func TestRemoteValidateAssignmentStaleOverTLS(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	reps, err := env.remote.ListRuntimeReplicasByNode(ctx, "n1")
	if err != nil || len(reps) != 1 {
		t.Fatal(err)
	}
	rep := reps[0]
	if rep.ValidUntil == nil {
		t.Fatal("missing valid_until")
	}
	scope := contract.CommandScope{
		NodeID: "n1", ProjectID: rep.ProjectID, VersionID: rep.VersionID,
		ReplicaID: rep.ReplicaID, Generation: 999,
		LeaseExpiry: *rep.ValidUntil, Nonce: "stale-test", Action: contract.ActionStopReplica,
	}
	_, err = env.remote.ValidateAgentAssignment(ctx, scope, time.Now().UTC())
	if !errors.Is(err, registry.ErrObservationStale) {
		t.Fatalf("expected stale sentinel over relay, got %v", err)
	}
}

func TestRemoteValidateAssignmentRegistryUnavailablePreserves(t *testing.T) {
	env := startRelayEnv(t, "n1")
	env.store.Close()
	ctx := context.Background()
	_, err := env.remote.ListRuntimeReplicasByNode(ctx, "n1")
	if !registryrelay.IsRegistryUnavailable(err) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestRelayMissingRuntimeNodeAbsentOverTLS(t *testing.T) {
	env := startRelayEnvUnregistered(t, "n1")
	ctx := context.Background()
	node, err := env.remote.GetRuntimeNode(ctx, "n1")
	if err != nil || node != nil {
		t.Fatalf("expected (nil,nil) absence, got node=%+v err=%v", node, err)
	}
}

func TestRelayMissingRuntimeReplicaAbsentOverTLS(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	rep, err := env.remote.GetRuntimeReplica(ctx, "missing-replica")
	if err != nil || rep != nil {
		t.Fatalf("expected (nil,nil) absence, got rep=%+v err=%v", rep, err)
	}
}

func TestRelayValidateAssignmentAbsentReplicaOverTLS(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	scope := contract.CommandScope{
		NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "missing",
		Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Hour), Nonce: "absent-replica",
		Action: contract.ActionStopReplica,
	}
	rep, err := env.remote.ValidateAgentAssignment(ctx, scope, time.Now().UTC())
	if err != nil || rep != nil {
		t.Fatalf("expected (nil,nil) validate absence, got rep=%+v err=%v", rep, err)
	}
}

func TestRelayTypedNodeNotFoundSentinelOverTLS(t *testing.T) {
	env := startRelayEnvUnregistered(t, "n1")
	ctx := context.Background()
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err := env.client.GetRuntimeReplica(ctx, scope, "ghost")
	if err == nil || !errors.Is(err, registry.ErrRuntimeNodeNotFound) || errors.Is(err, registry.ErrRuntimeReplicaNotFound) {
		t.Fatalf("expected node sentinel only over TLS, got %v", err)
	}
}

func TestRelayMutationMissingReplicaNotNodeNotFoundOverTLS(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	scope := env.client.NewRelayScope("n1", time.Minute)
	err := env.client.TerminalizeReplica(ctx, scope, "missing-replica", 1, contract.ReplicaStopped)
	if err == nil {
		t.Fatal("expected error for missing replica terminalize")
	}
	if errors.Is(err, registry.ErrRuntimeNodeNotFound) {
		t.Fatalf("must not classify as node not found: %v", err)
	}
	if !errors.Is(err, registry.ErrObservationStale) {
		t.Fatalf("expected stale/conflict sentinel for missing replica mutation, got %v", err)
	}
}

func TestReconcileNodeRemoteStaleAssignmentStopsInventoryOverTLS(t *testing.T) {
	const nodeID = "n1"
	env := startRelayEnv(t, nodeID)
	ctx := context.Background()
	seedRelayReplicaReady(t, env.store, nodeID)

	backend := newRelayObserveBackend()
	backend.inventory["r1"] = agent.BackendReplica{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", Host: "127.0.0.1", Port: 9101, Healthy: true}

	node, err := env.store.GetRuntimeNode(ctx, nodeID)
	if err != nil || node == nil {
		t.Fatalf("node: %+v err=%v", node, err)
	}
	node.Generation = 2
	if err := env.store.ActivateRuntimeNode(ctx, *node, 1); err != nil {
		t.Fatal(err)
	}

	reps, err := env.remote.ListRuntimeReplicasByNode(ctx, nodeID)
	if err != nil || len(reps) != 1 || reps[0].ValidUntil == nil {
		t.Fatalf("replicas: %+v err=%v", reps, err)
	}
	stopScope := contract.CommandScope{
		NodeID: nodeID, ProjectID: "demo", VersionID: "v1", ReplicaID: "r1",
		Generation: 1, LeaseExpiry: *reps[0].ValidUntil, Nonce: "stale-check", Action: contract.ActionStopReplica,
	}
	_, validateErr := env.remote.ValidateAgentAssignment(ctx, stopScope, time.Now().UTC())
	if !errors.Is(validateErr, registry.ErrObservationStale) && !registrywire.IsAuthoritativeGenerationStale(validateErr) {
		t.Fatalf("expected authoritative stale over relay, got %v", validateErr)
	}

	h := newRelayLifecycleHandler(env.remote, backend)
	if err := h.ReconcileNode(ctx, nodeID); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if backend.stopCount() < 1 || backend.hasReplica("r1") {
		t.Fatalf("expected local stop, stops=%d has=%v", backend.stopCount(), backend.hasReplica("r1"))
	}
	rep, err := env.store.GetRuntimeReplica(ctx, "r1")
	if err != nil || rep.State != contract.ReplicaStopped {
		t.Fatalf("expected terminalized replica, rep=%+v err=%v", rep, err)
	}
}

func TestReconcileNodeRemoteRegistryUnavailablePreservesInventoryOverTLS(t *testing.T) {
	const nodeID = "n1"
	env := startRelayEnv(t, nodeID)
	ctx := context.Background()
	seedRelayReplicaReady(t, env.store, nodeID)

	backend := newRelayObserveBackend()
	backend.inventory["r1"] = agent.BackendReplica{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", Host: "127.0.0.1", Port: 9101, Healthy: true}
	h := newRelayLifecycleHandler(env.remote, backend)

	node, err := env.store.GetRuntimeNode(ctx, nodeID)
	if err != nil || node == nil {
		t.Fatalf("node: %+v err=%v", node, err)
	}
	identityURI := node.IdentityURI

	_ = env.store.Close()
	err = h.ReconcileNode(ctx, nodeID)
	if err == nil || !registryrelay.IsRegistryUnavailable(err) {
		t.Fatalf("expected transient unavailable, got %v", err)
	}
	if agent.ReconcileNodeErrorFatal(err) {
		t.Fatalf("unavailable must not be fatal: %v", err)
	}
	if backend.stopCount() != 0 || !backend.hasReplica("r1") {
		t.Fatalf("inventory preserved, stops=%d has=%v", backend.stopCount(), backend.hasReplica("r1"))
	}

	recoverStore, err := registry.Open(t.TempDir() + "/relay-recover.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recoverStore.Close() })
	seedRelayControllerRegistry(t, recoverStore, nodeID, identityURI)
	seedRelayReplicaReady(t, recoverStore, nodeID)
	env.relayHandler.Store = recoverStore

	if err := h.ReconcileNode(ctx, nodeID); err != nil {
		t.Fatalf("recover reconcile: %v", err)
	}
}

func TestReconcileNodeRemotePerReplicaValidateUncertaintyPreservesInventoryOverTLS(t *testing.T) {
	const nodeID = "n1"
	env := startRelayEnv(t, nodeID)
	ctx := context.Background()
	seedRelayReplicaReady(t, env.store, nodeID)

	injectErr := errors.New("injected validate assignment uncertainty")
	env.relayHandler.Store = &relayValidateFaultStore{
		SQLiteStore: env.store,
		validateErr: errors.Join(registryrelay.ErrRegistryUnavailable, injectErr),
	}

	reps, err := env.remote.ListRuntimeReplicasByNode(ctx, nodeID)
	if err != nil || len(reps) != 1 {
		t.Fatalf("list must succeed before per-replica validate fault: %+v err=%v", reps, err)
	}

	backend := newRelayObserveBackend()
	backend.inventory["r1"] = agent.BackendReplica{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", Host: "127.0.0.1", Port: 9101, Healthy: true}
	h := newRelayLifecycleHandler(env.remote, backend)

	err = h.ReconcileNode(ctx, nodeID)
	if err == nil {
		t.Fatal("expected per-replica validate uncertainty")
	}
	if !strings.Contains(err.Error(), "validate assignment") {
		t.Fatalf("expected per-replica validate branch, got %v", err)
	}
	if !registryrelay.IsRegistryUnavailable(err) {
		t.Fatalf("expected transient registry uncertainty, got %v", err)
	}
	if agent.ReconcileNodeErrorFatal(err) {
		t.Fatalf("per-replica uncertainty must not be fatal: %v", err)
	}
	if backend.stopCount() != 0 || !backend.hasReplica("r1") {
		t.Fatalf("inventory preserved, stops=%d has=%v", backend.stopCount(), backend.hasReplica("r1"))
	}
	rep, err := env.store.GetRuntimeReplica(ctx, "r1")
	if err != nil || rep == nil || rep.State != contract.ReplicaReady {
		t.Fatalf("replica must not be terminalized on uncertainty: rep=%+v err=%v", rep, err)
	}
}

func TestReconcileNodeRemoteMissingNodeCleansInventoryOverTLS(t *testing.T) {
	const nodeID = "n1"
	ctx := context.Background()

	absent := startRelayEnvUnregistered(t, nodeID)
	node, err := absent.remote.GetRuntimeNode(ctx, nodeID)
	if err != nil || node != nil {
		t.Fatalf("typed not-found must map to local absence, node=%+v err=%v", node, err)
	}
	if registryrelay.IsRegistryUnavailable(err) {
		t.Fatal("absent node must not surface as generic relay unavailable")
	}

	env := startRelayEnv(t, nodeID)
	seedRelayReplicaReady(t, env.store, nodeID)
	backend := newRelayObserveBackend()
	backend.inventory["r1"] = agent.BackendReplica{ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", Host: "127.0.0.1", Port: 9101, Healthy: true}
	h := newRelayLifecycleHandler(env.remote, backend)

	regNode, err := env.store.GetRuntimeNode(ctx, nodeID)
	if err != nil || regNode == nil {
		t.Fatalf("node: %+v err=%v", regNode, err)
	}
	regNode.Cordoned = true
	if err := env.store.UpsertRuntimeNode(ctx, *regNode); err != nil {
		t.Fatal(err)
	}

	err = h.ReconcileNode(ctx, nodeID)
	if err == nil || !agent.ReconcileNodeErrorFatal(err) {
		t.Fatalf("expected fatal unavailable-node cleanup, got %v", err)
	}
	if registryrelay.IsRegistryUnavailable(err) {
		t.Fatalf("unavailable-node cleanup must not be generic relay unavailable: %v", err)
	}
	var staleCmd *agent.CommandError
	if !errors.As(err, &staleCmd) || staleCmd.Reason != contract.ReasonGenerationStale {
		t.Fatalf("expected generation_stale unavailable cleanup, got %v", err)
	}
	if backend.stopCount() != 1 || backend.hasReplica("r1") {
		t.Fatalf("expected inventory cleanup, stops=%d has=%v", backend.stopCount(), backend.hasReplica("r1"))
	}

	if crossErr := h.ReconcileNode(ctx, "n2"); crossErr == nil {
		t.Fatal("expected cross-node reconcile denial")
	}
	if backend.stopCount() != 1 {
		t.Fatalf("cross-node must not touch inventory, stops=%d", backend.stopCount())
	}
}
