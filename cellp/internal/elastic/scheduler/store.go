package scheduler

import (
	"context"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// RouteReader supplies gateway route state for scale-to-zero demotion guards.
type RouteReader interface {
	GetRoute(ctx context.Context, projectID, versionID string) (*registry.Route, error)
}

// Store is the registry surface required by the scheduler loop.
type Store interface {
	ListElasticServingPolicies(ctx context.Context) ([]registry.ServingPolicyRow, error)
	GetVersion(ctx context.Context, projectID, versionID string) (*registry.Version, error)
	GetServingDesire(ctx context.Context, projectID, versionID string) (*registry.ServingDesireRow, error)
	GetRoute(ctx context.Context, projectID, versionID string) (*registry.Route, error)
	ListRuntimeNodes(ctx context.Context) ([]contract.RuntimeNode, error)
	ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error)
	ListRuntimeReplicasForReconcile(ctx context.Context) ([]contract.RuntimeReplica, error)
	BuildQualificationViewAfter(ctx context.Context, afterRevision int64) (registry.QualificationView, bool, error)
	CompareAndSetElasticVersionStatus(ctx context.Context, projectID, versionID, expectedStatus, newStatus string, expectedDesireGeneration int64, expectedDesiredReplicas int) error
	ClaimAssignment(ctx context.Context, claim registry.AssignmentClaim) error
	ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error)
	RenewAssignmentLease(ctx context.Context, replicaID, nodeID string, generation, expectedNodeGeneration int64, expectedExpiry, newExpiry time.Time) error
	TerminalizeReplica(ctx context.Context, replicaID, nodeID string, generation int64, state contract.ReplicaState) error
}

// RegistryStore adapts registry.ServingStore.
type RegistryStore struct {
	registry.ServingStore
	Routes RouteReader
}

// RegistryStoreFromServing builds a scheduler store, wiring route lookup when available.
func RegistryStoreFromServing(store registry.ServingStore) RegistryStore {
	rs := RegistryStore{ServingStore: store}
	if routes, ok := store.(RouteReader); ok {
		rs.Routes = routes
	}
	return rs
}

func (s RegistryStore) GetRoute(ctx context.Context, projectID, versionID string) (*registry.Route, error) {
	if s.Routes == nil {
		return nil, nil
	}
	return s.Routes.GetRoute(ctx, projectID, versionID)
}
