package scheduler

import (
	"context"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/contract"
)

// RuntimeNodeClient is the narrow Node Agent surface used by the scheduler.
type RuntimeNodeClient interface {
	StartReplica(ctx context.Context, spec contract.StartReplicaSpec, idempotencyKey string) (contract.RuntimeReplica, error)
	ProbeReplica(ctx context.Context, scope contract.CommandScope) (agent.ProbeResult, error)
	DrainReplica(ctx context.Context, scope contract.CommandScope, deadline time.Time) (contract.RuntimeReplica, error)
	StopReplica(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error)
}

// ClientFactory constructs an mTLS client for a registered runtime node.
type ClientFactory func(node contract.RuntimeNode) (RuntimeNodeClient, error)
