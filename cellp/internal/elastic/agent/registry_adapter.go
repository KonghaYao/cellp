package agent

import (
	"context"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// RegistryStores adapts registry.ServingStore to NodeStore and ReplicaStore.
type RegistryStores struct {
	Store registry.ServingStore
}

func (a RegistryStores) UpsertRuntimeNode(ctx context.Context, node contract.RuntimeNode) error {
	if a.Store == nil {
		return errInvalidNode
	}
	return a.Store.UpsertRuntimeNode(ctx, node)
}

func (a RegistryStores) ActivateRuntimeNode(ctx context.Context, node contract.RuntimeNode, expectedGeneration int64) error {
	if a.Store == nil {
		return errInvalidNode
	}
	return a.Store.ActivateRuntimeNode(ctx, node, expectedGeneration)
}

func (a RegistryStores) GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error) {
	if a.Store == nil {
		return nil, errInvalidNode
	}
	return a.Store.GetRuntimeNode(ctx, nodeID)
}

func (a RegistryStores) ListRuntimeNodes(ctx context.Context) ([]contract.RuntimeNode, error) {
	if a.Store == nil {
		return nil, errInvalidNode
	}
	return a.Store.ListRuntimeNodes(ctx)
}

func (a RegistryStores) UpsertRuntimeReplica(ctx context.Context, rep contract.RuntimeReplica) error {
	if a.Store == nil {
		return errReplicaNotFound
	}
	return a.Store.UpsertRuntimeReplica(ctx, rep)
}

func (a RegistryStores) ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error) {
	if a.Store == nil {
		return nil, errReplicaNotFound
	}
	return a.Store.ListRuntimeReplicas(ctx, projectID, versionID)
}

func (a RegistryStores) GetRuntimeReplica(ctx context.Context, replicaID string) (*contract.RuntimeReplica, error) {
	if a.Store == nil {
		return nil, errReplicaNotFound
	}
	return a.Store.GetRuntimeReplica(ctx, replicaID)
}

func (a RegistryStores) ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error) {
	if a.Store == nil {
		return nil, errReplicaNotFound
	}
	return a.Store.ValidateAgentAssignment(ctx, scope, now)
}

func (a RegistryStores) ValidateAgentCleanupAssignment(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error) {
	if a.Store == nil {
		return nil, errReplicaNotFound
	}
	return a.Store.ValidateAgentCleanupAssignment(ctx, scope)
}

func (a RegistryStores) ListRuntimeReplicasByNode(ctx context.Context, nodeID string) ([]contract.RuntimeReplica, error) {
	if a.Store == nil {
		return nil, errReplicaNotFound
	}
	return a.Store.ListRuntimeReplicasByNode(ctx, nodeID)
}

func (a RegistryStores) RecordObservation(ctx context.Context, obs registry.ReplicaObservation) error {
	if a.Store == nil {
		return errReplicaNotFound
	}
	return a.Store.RecordObservation(ctx, obs)
}

func (a RegistryStores) ClaimAgentCommand(ctx context.Context, command registry.AgentCommand) (registry.AgentCommandClaim, error) {
	if a.Store == nil {
		return registry.AgentCommandClaim{}, errReplicaNotFound
	}
	return a.Store.ClaimAgentCommand(ctx, command)
}

func (a RegistryStores) RenewAgentCommandLease(ctx context.Context, command registry.AgentCommand, expiry time.Time) error {
	if a.Store == nil {
		return errReplicaNotFound
	}
	return a.Store.RenewAgentCommandLease(ctx, command, expiry)
}

func (a RegistryStores) CompleteAgentCommand(ctx context.Context, command registry.AgentCommand) error {
	if a.Store == nil {
		return errReplicaNotFound
	}
	return a.Store.CompleteAgentCommand(ctx, command)
}

func (a RegistryStores) RecordObservationAndCompleteAgentCommand(ctx context.Context, obs registry.ReplicaObservation, command registry.AgentCommand) error {
	if a.Store == nil {
		return errReplicaNotFound
	}
	return a.Store.RecordObservationAndCompleteAgentCommand(ctx, obs, command)
}

func (a RegistryStores) WithdrawReplica(ctx context.Context, replicaID, projectID, versionID, nodeID string, generation int64) error {
	if a.Store == nil {
		return errReplicaNotFound
	}
	return a.Store.WithdrawReplica(ctx, replicaID, projectID, versionID, nodeID, generation)
}

func (a RegistryStores) TerminalizeReplica(ctx context.Context, replicaID, nodeID string, generation int64, state contract.ReplicaState) error {
	if a.Store == nil {
		return errReplicaNotFound
	}
	return a.Store.TerminalizeReplica(ctx, replicaID, nodeID, generation, state)
}

// NewLifecycleFromRegistry builds a production handler with strict assignment/observation persistence.
func NewLifecycleFromRegistry(enabled bool, store registry.ServingStore, backend LifecycleBackend) *Handler {
	adapter := RegistryStores{Store: store}
	return NewLifecycleHandler(enabled, adapter, adapter, backend)
}

// NewFromRegistry builds the compatibility registry scaffold handler.
func NewFromRegistry(enabled bool, store registry.ServingStore) *Handler {
	adapter := RegistryStores{Store: store}
	return NewHandler(enabled, adapter, adapter)
}
