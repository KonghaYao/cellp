package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// Handler executes Node Agent lifecycle commands (stubs until process backend lands).
type Handler struct {
	enabled bool
	nodes   NodeStore
	reps    ReplicaStore
	now     func() time.Time
}

// NewHandler builds a handler. When enabled is false, all commands fail closed.
func NewHandler(enabled bool, nodes NodeStore, reps ReplicaStore) *Handler {
	return &Handler{
		enabled: enabled,
		nodes:   nodes,
		reps:    reps,
		now:     time.Now,
	}
}

// Enabled reports whether elastic agent commands are active.
func (h *Handler) Enabled() bool {
	return h != nil && h.enabled
}

// ProbeResult is the observation for a single replica.
type ProbeResult struct {
	ReplicaID string               `json:"replica_id"`
	State     contract.ReplicaState  `json:"state"`
	Generation int64               `json:"generation"`
}

// StartReplica records a starting replica (no celld spawn in E3 scaffold).
func (h *Handler) StartReplica(ctx context.Context, spec contract.StartReplicaSpec, idempotencyKey string) (contract.RuntimeReplica, error) {
	_ = idempotencyKey
	if err := h.guard(); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if err := contract.ValidateCommandScope(spec.Scope); err != nil {
		return contract.RuntimeReplica{}, fmt.Errorf("scope: %w", err)
	}
	if strings.TrimSpace(spec.Bucket) == "" {
		return contract.RuntimeReplica{}, fmt.Errorf("bucket required")
	}
	if err := h.checkNode(ctx, spec.Scope); err != nil {
		return contract.RuntimeReplica{}, err
	}
	replicaID := spec.Scope.ReplicaID
	if replicaID == "" {
		replicaID = fmt.Sprintf("%s-%s-%d", spec.Scope.VersionID, spec.Scope.NodeID, spec.Scope.Generation)
	}
	rep := contract.RuntimeReplica{
		ReplicaID:  replicaID,
		ProjectID:  spec.Scope.ProjectID,
		VersionID:  spec.Scope.VersionID,
		NodeID:     spec.Scope.NodeID,
		Generation: spec.Scope.Generation,
		State:      contract.ReplicaStarting,
	}
	if h.reps != nil {
		if err := h.reps.UpsertRuntimeReplica(ctx, rep); err != nil {
			return contract.RuntimeReplica{}, err
		}
	}
	return rep, nil
}

// ProbeReplica returns persisted replica state.
func (h *Handler) ProbeReplica(ctx context.Context, scope contract.CommandScope) (ProbeResult, error) {
	if err := h.guard(); err != nil {
		return ProbeResult{}, err
	}
	if err := contract.ValidateCommandScope(scope); err != nil {
		return ProbeResult{}, fmt.Errorf("scope: %w", err)
	}
	if strings.TrimSpace(scope.ReplicaID) == "" {
		return ProbeResult{}, fmt.Errorf("replica_id required")
	}
	if err := h.checkNode(ctx, scope); err != nil {
		return ProbeResult{}, err
	}
	if h.reps == nil {
		return ProbeResult{}, errReplicaNotFound
	}
	reps, err := h.reps.ListRuntimeReplicas(ctx, scope.ProjectID, scope.VersionID)
	if err != nil {
		return ProbeResult{}, err
	}
	for _, r := range reps {
		if r.ReplicaID != scope.ReplicaID {
			continue
		}
		if r.Generation > scope.Generation {
			return ProbeResult{}, &CommandError{Reason: contract.ReasonGenerationStale, Message: "replica generation ahead of scope"}
		}
		if r.Generation < scope.Generation {
			return ProbeResult{}, &CommandError{Reason: contract.ReasonGenerationStale, Message: "replica generation behind scope"}
		}
		return ProbeResult{ReplicaID: r.ReplicaID, State: r.State, Generation: r.Generation}, nil
	}
	return ProbeResult{}, errReplicaNotFound
}

// StopReplica marks a replica stopped when generation matches.
func (h *Handler) StopReplica(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	if err := h.guard(); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if err := contract.ValidateCommandScope(scope); err != nil {
		return contract.RuntimeReplica{}, fmt.Errorf("scope: %w", err)
	}
	if strings.TrimSpace(scope.ReplicaID) == "" {
		return contract.RuntimeReplica{}, fmt.Errorf("replica_id required")
	}
	if err := h.checkNode(ctx, scope); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if h.reps == nil {
		return contract.RuntimeReplica{}, errReplicaNotFound
	}
	reps, err := h.reps.ListRuntimeReplicas(ctx, scope.ProjectID, scope.VersionID)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	for _, r := range reps {
		if r.ReplicaID != scope.ReplicaID {
			continue
		}
		if r.Generation != scope.Generation {
			return contract.RuntimeReplica{}, &CommandError{Reason: contract.ReasonGenerationStale, Message: "generation mismatch"}
		}
		r.State = contract.ReplicaStopped
		if err := h.reps.UpsertRuntimeReplica(ctx, r); err != nil {
			return contract.RuntimeReplica{}, err
		}
		return r, nil
	}
	return contract.RuntimeReplica{}, errReplicaNotFound
}

func (h *Handler) guard() error {
	if h == nil || !h.enabled {
		return elasticDisabled()
	}
	if h.nodes == nil {
		return fmt.Errorf("node store required")
	}
	return nil
}

func (h *Handler) checkNode(ctx context.Context, scope contract.CommandScope) error {
	now := h.now().UTC()
	if !scope.LeaseExpiry.After(now) {
		return &CommandError{Reason: contract.ReasonGenerationStale, Message: "lease expired"}
	}
	node, err := h.nodes.GetRuntimeNode(ctx, scope.NodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return errNodeNotFound
	}
	if node.Cordoned {
		return errNodeCordoned
	}
	if scope.Generation < node.Generation {
		return &CommandError{Reason: contract.ReasonGenerationStale, Message: "node generation stale"}
	}
	return nil
}
