package scheduler

import (
	"context"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// Store is the registry surface required by the scheduler loop.
type Store interface {
	ListElasticServingPolicies(ctx context.Context) ([]registry.ServingPolicyRow, error)
	GetVersion(ctx context.Context, projectID, versionID string) (*registry.Version, error)
	GetServingDesire(ctx context.Context, projectID, versionID string) (*registry.ServingDesireRow, error)
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
}
