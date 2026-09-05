package autoscaler

import (
	"context"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// Store is the registry surface required by the autoscaler loop.
type Store interface {
	ListElasticServingPolicies(ctx context.Context) ([]registry.ServingPolicyRow, error)
	GetServingDesire(ctx context.Context, projectID, versionID string) (*registry.ServingDesireRow, error)
	ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error)
}

// RegistryStore adapts registry.Store / ServingStore.
type RegistryStore struct {
	registry.ServingStore
}

// ListElasticServingPolicies lists enrolled serving policies.
func (r RegistryStore) ListElasticServingPolicies(ctx context.Context) ([]registry.ServingPolicyRow, error) {
	return r.ServingStore.ListElasticServingPolicies(ctx)
}
