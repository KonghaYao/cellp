package agent

import (
	"context"
	"strings"

	"github.com/cellp/cellp/internal/elastic/contract"
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

func validateRuntimeNode(n contract.RuntimeNode) error {
	if strings.TrimSpace(n.NodeID) == "" {
		return errInvalidNode
	}
	if n.CapacityUnits < 0 {
		return errInvalidNode
	}
	return nil
}
