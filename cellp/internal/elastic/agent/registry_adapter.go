package agent

import (
	"context"

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

// NewFromRegistry builds a flag-gated handler backed by registry serving facts.
func NewFromRegistry(enabled bool, store registry.ServingStore) *Handler {
	adapter := RegistryStores{Store: store}
	return NewHandler(enabled, adapter, adapter)
}
