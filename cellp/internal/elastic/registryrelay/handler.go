package registryrelay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// GuardChecker ensures only the active controller writer serves registry relay RPCs.
type GuardChecker interface {
	EnsureActiveController(ctx context.Context) error
}

// Handler applies node-scoped registry operations for authenticated remote agents.
type Handler struct {
	Store     registry.ServingStore
	Allowlist map[string]string // node_id -> spiffe identity URI
	Guard     GuardChecker
	Now       func() time.Time
}

func (h *Handler) now() time.Time {
	if h != nil && h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

func (h *Handler) ensure(ctx context.Context) error {
	if h == nil || h.Store == nil {
		return fmt.Errorf("%w: store unavailable", ErrRegistryUnavailable)
	}
	if h.Guard != nil {
		if err := h.Guard.EnsureActiveController(ctx); err != nil {
			return fmt.Errorf("%w: %v", ErrRegistryUnavailable, err)
		}
	}
	return nil
}

func (h *Handler) authorizePeer(nodeID, peerIdentity string) error {
	nodeID = strings.TrimSpace(nodeID)
	peerIdentity = strings.TrimSpace(peerIdentity)
	if nodeID == "" || peerIdentity == "" {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: "node identity binding required"}
	}
	expected, ok := h.Allowlist[nodeID]
	if !ok || expected != peerIdentity {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: "node not allowlisted"}
	}
	return nil
}

func (h *Handler) ensureRegisteredNode(ctx context.Context, peerIdentity, nodeID string, requireLease bool) error {
	if err := h.authorizePeer(nodeID, peerIdentity); err != nil {
		return err
	}
	node, err := h.Store.GetRuntimeNode(ctx, nodeID)
	if err != nil {
		return MapRegistryError(err)
	}
	if node == nil {
		return &RelayError{Reason: contract.ReasonRuntimeNodeNotFound, Message: string(contract.ReasonRuntimeNodeNotFound)}
	}
	if strings.TrimSpace(node.IdentityURI) != peerIdentity {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: "identity uri mismatch"}
	}
	now := h.now()
	if requireLease && !node.LeaseExpiry.After(now) {
		return &RelayError{Reason: contract.ReasonLeaseExpired, Message: registry.ErrLeaseExpired.Error()}
	}
	return nil
}

func (h *Handler) bindScope(peerIdentity string, scope contract.RegistryRelayScope) error {
	now := h.now()
	if err := contract.ValidateRegistryRelayScope(scope, now); err != nil {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: err.Error()}
	}
	return h.authorizePeer(scope.NodeID, peerIdentity)
}

func (h *Handler) bindCommandScope(peerIdentity string, scope contract.CommandScope) error {
	if err := contract.ValidateCommandScope(scope); err != nil {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: err.Error()}
	}
	return h.authorizePeer(scope.NodeID, peerIdentity)
}

// GetRuntimeNode returns registry metadata for the authenticated node only.
func (h *Handler) GetRuntimeNode(ctx context.Context, peerIdentity string, scope contract.RegistryRelayScope) (*contract.RuntimeNode, error) {
	if err := h.ensure(ctx); err != nil {
		return nil, err
	}
	if err := h.bindScope(peerIdentity, scope); err != nil {
		return nil, err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, scope.NodeID, false); err != nil {
		return nil, err
	}
	node, err := h.Store.GetRuntimeNode(ctx, scope.NodeID)
	if err != nil {
		return nil, MapRegistryError(err)
	}
	if node == nil {
		return nil, &RelayError{Reason: contract.ReasonRuntimeNodeNotFound, Message: string(contract.ReasonRuntimeNodeNotFound)}
	}
	return node, nil
}

// GetRuntimeReplica returns a replica fact when it belongs to the authenticated node.
func (h *Handler) GetRuntimeReplica(ctx context.Context, peerIdentity string, scope contract.RegistryRelayScope, replicaID string) (*contract.RuntimeReplica, error) {
	if err := h.ensure(ctx); err != nil {
		return nil, err
	}
	if err := h.bindScope(peerIdentity, scope); err != nil {
		return nil, err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, scope.NodeID, true); err != nil {
		return nil, err
	}
	replicaID = strings.TrimSpace(replicaID)
	if replicaID == "" {
		return nil, &RelayError{Reason: contract.ReasonAuthFailed, Message: "replica_id required"}
	}
	rep, err := h.Store.GetRuntimeReplica(ctx, replicaID)
	if err != nil {
		return nil, MapRegistryError(err)
	}
	if rep == nil {
		return nil, &RelayError{Reason: contract.ReasonRuntimeReplicaNotFound, Message: string(contract.ReasonRuntimeReplicaNotFound)}
	}
	if rep.NodeID != scope.NodeID {
		return nil, &RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node read denied"}
	}
	return rep, nil
}

// ValidateAgentAssignment proxies assignment validation for the authenticated node.
func (h *Handler) ValidateAgentAssignment(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, cmdScope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error) {
	if err := h.ensure(ctx); err != nil {
		return nil, err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return nil, err
	}
	if err := h.bindCommandScope(peerIdentity, cmdScope); err != nil {
		return nil, err
	}
	if relayScope.NodeID != cmdScope.NodeID {
		return nil, &RelayError{Reason: contract.ReasonAuthFailed, Message: "scope node mismatch"}
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, cmdScope.NodeID, true); err != nil {
		return nil, err
	}
	rep, err := h.Store.ValidateAgentAssignment(ctx, cmdScope, now.UTC())
	return rep, MapRegistryError(err)
}

// ValidateAgentCleanupAssignment proxies cleanup assignment validation.
// Intentional recovery path: node assignment lease may be expired; only identity/node/scope/generation binding is required.
func (h *Handler) ValidateAgentCleanupAssignment(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, cmdScope contract.CommandScope) (*contract.RuntimeReplica, error) {
	if err := h.ensure(ctx); err != nil {
		return nil, err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return nil, err
	}
	if err := h.bindCommandScope(peerIdentity, cmdScope); err != nil {
		return nil, err
	}
	if relayScope.NodeID != cmdScope.NodeID {
		return nil, &RelayError{Reason: contract.ReasonAuthFailed, Message: "scope node mismatch"}
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, cmdScope.NodeID, false); err != nil {
		return nil, err
	}
	rep, err := h.Store.ValidateAgentCleanupAssignment(ctx, cmdScope)
	return rep, MapRegistryError(err)
}

// ListRuntimeReplicasByNode lists replicas assigned to the authenticated node.
func (h *Handler) ListRuntimeReplicasByNode(ctx context.Context, peerIdentity string, scope contract.RegistryRelayScope) ([]contract.RuntimeReplica, error) {
	if err := h.ensure(ctx); err != nil {
		return nil, err
	}
	if err := h.bindScope(peerIdentity, scope); err != nil {
		return nil, err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, scope.NodeID, true); err != nil {
		return nil, err
	}
	reps, err := h.Store.ListRuntimeReplicasByNode(ctx, scope.NodeID)
	return reps, MapRegistryError(err)
}

func (h *Handler) authorizeCommandNode(peerIdentity, nodeID string, command registry.AgentCommand) error {
	if err := h.authorizePeer(nodeID, peerIdentity); err != nil {
		return err
	}
	if strings.TrimSpace(command.NodeID) != nodeID {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: "command node mismatch"}
	}
	return nil
}

// RecordObservation records a replica observation for the authenticated node.
func (h *Handler) RecordObservation(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, obs registry.ReplicaObservation) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return err
	}
	if obs.NodeID != relayScope.NodeID {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return MapRegistryError(h.Store.RecordObservation(ctx, obs))
}

// ClaimAgentCommand claims a durable agent command for the authenticated node.
func (h *Handler) ClaimAgentCommand(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, command registry.AgentCommand) (registry.AgentCommandClaim, error) {
	if err := h.ensure(ctx); err != nil {
		return registry.AgentCommandClaim{}, err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return registry.AgentCommandClaim{}, err
	}
	if err := h.authorizeCommandNode(peerIdentity, relayScope.NodeID, command); err != nil {
		return registry.AgentCommandClaim{}, err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return registry.AgentCommandClaim{}, err
	}
	claim, err := h.Store.ClaimAgentCommand(ctx, command)
	return claim, MapRegistryError(err)
}

// RenewAgentCommandLease renews a command lease for the authenticated node.
func (h *Handler) RenewAgentCommandLease(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, command registry.AgentCommand, expiry time.Time) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return err
	}
	if err := h.authorizeCommandNode(peerIdentity, relayScope.NodeID, command); err != nil {
		return err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return err
	}
	return MapRegistryError(h.Store.RenewAgentCommandLease(ctx, command, expiry.UTC()))
}

// CompleteAgentCommand completes a command for the authenticated node.
func (h *Handler) CompleteAgentCommand(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, command registry.AgentCommand) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return err
	}
	if err := h.authorizeCommandNode(peerIdentity, relayScope.NodeID, command); err != nil {
		return err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return err
	}
	return MapRegistryError(h.Store.CompleteAgentCommand(ctx, command))
}

// RecordObservationAndCompleteAgentCommand atomically records observation and completes a command.
func (h *Handler) RecordObservationAndCompleteAgentCommand(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, obs registry.ReplicaObservation, command registry.AgentCommand) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return err
	}
	if err := h.authorizeCommandNode(peerIdentity, relayScope.NodeID, command); err != nil {
		return err
	}
	if obs.NodeID != relayScope.NodeID || command.NodeID != relayScope.NodeID {
		return &RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return err
	}
	return MapRegistryError(h.Store.RecordObservationAndCompleteAgentCommand(ctx, obs, command))
}

// WithdrawReplica withdraws a replica on the authenticated node.
func (h *Handler) WithdrawReplica(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, replicaID, projectID, versionID string, generation int64) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return err
	}
	return MapRegistryError(h.Store.WithdrawReplica(ctx, replicaID, projectID, versionID, relayScope.NodeID, generation))
}

// TerminalizeReplica terminalizes a replica on the authenticated node.
func (h *Handler) TerminalizeReplica(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, replicaID string, generation int64, state contract.ReplicaState) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return err
	}
	return MapRegistryError(h.Store.TerminalizeReplica(ctx, replicaID, relayScope.NodeID, generation, state))
}

// GetVersionEnv returns worker env overrides for a version assigned to the authenticated node.
func (h *Handler) GetVersionEnv(ctx context.Context, peerIdentity string, relayScope contract.RegistryRelayScope, projectID, versionID string) (map[string]string, error) {
	if err := h.ensure(ctx); err != nil {
		return nil, err
	}
	if err := h.bindScope(peerIdentity, relayScope); err != nil {
		return nil, err
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, relayScope.NodeID, true); err != nil {
		return nil, err
	}
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	if projectID == "" || versionID == "" {
		return nil, &RelayError{Reason: contract.ReasonAuthFailed, Message: "project and version required"}
	}
	env, err := h.Store.GetAuthorizedAgentVersionEnv(ctx, relayScope.NodeID, projectID, versionID, h.now())
	if errors.Is(err, registry.ErrAgentVersionEnvForbidden) {
		return nil, &RelayError{Reason: contract.ReasonAuthFailed, Message: "version env not authorized for node"}
	}
	return env, MapRegistryError(err)
}

// ErrRelayNotSupported is returned for NodeStore operations that must not be proxied.
var ErrRelayNotSupported = errors.New("registry relay operation not supported")
