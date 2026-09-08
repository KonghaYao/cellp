package agent

import (
	"context"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// NodeStore persists runtime node registration (WP-REG runtime_nodes).
type NodeStore interface {
	UpsertRuntimeNode(ctx context.Context, node contract.RuntimeNode) error
	GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error)
	ListRuntimeNodes(ctx context.Context) ([]contract.RuntimeNode, error)
}

// ReplicaStore records replica lifecycle facts observed by the agent.
type ReplicaStore interface {
	UpsertRuntimeReplica(ctx context.Context, rep contract.RuntimeReplica) error
	ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error)
}

// LifecycleStore is the fail-closed production persistence boundary.
type LifecycleStore interface {
	GetRuntimeReplica(ctx context.Context, replicaID string) (*contract.RuntimeReplica, error)
	ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error)
	ValidateAgentCleanupAssignment(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error)
	ListRuntimeReplicasByNode(ctx context.Context, nodeID string) ([]contract.RuntimeReplica, error)
	RecordObservation(ctx context.Context, obs registry.ReplicaObservation) error
	ClaimAgentCommand(ctx context.Context, command registry.AgentCommand) (registry.AgentCommandClaim, error)
	RenewAgentCommandLease(ctx context.Context, command registry.AgentCommand, expiry time.Time) error
	CompleteAgentCommand(ctx context.Context, command registry.AgentCommand) error
	RecordObservationAndCompleteAgentCommand(ctx context.Context, obs registry.ReplicaObservation, command registry.AgentCommand) error
	WithdrawReplica(ctx context.Context, replicaID, projectID, versionID, nodeID string, generation int64) error
	TerminalizeReplica(ctx context.Context, replicaID, nodeID string, generation int64, state contract.ReplicaState) error
}

// LifecycleBackend controls actual local runtime instances.
type LifecycleBackend interface {
	ExpectedReplicaBucket(scope contract.CommandScope) (string, error)
	Diagnose(ctx context.Context, spec contract.StartReplicaSpec) error
	Start(ctx context.Context, spec contract.StartReplicaSpec) (host string, port int, err error)
	Probe(ctx context.Context, scope contract.CommandScope) (BackendReplica, error)
	Drain(ctx context.Context, scope contract.CommandScope, deadline time.Time) error
	Stop(ctx context.Context, scope contract.CommandScope) error
	List(ctx context.Context) ([]BackendReplica, error)
}

// BackendReplica is non-sensitive local process inventory.
type BackendReplica struct {
	ReplicaID string
	ProjectID string
	VersionID string
	Host      string
	Port      int
	Healthy   bool
}

func validateRuntimeNode(n contract.RuntimeNode) error {
	if strings.TrimSpace(n.NodeID) == "" {
		return errInvalidNode
	}
	if n.CapacityUnits < 0 {
		return errInvalidNode
	}
	return nil
}
