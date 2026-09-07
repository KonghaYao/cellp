package agentstore

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/registry"
)

// Store implements agent.NodeStore and agent.LifecycleStore over the controller registry relay.
type Store struct {
	Client   *transport.Client
	NodeID   string
	ScopeTTL time.Duration

	nonceSeq uint64
}

// NewStore wires a relay client to a fixed node identity.
func NewStore(c *transport.Client, nodeID string, scopeTTL time.Duration) *Store {
	return &Store{Client: c, NodeID: nodeID, ScopeTTL: scopeTTL}
}

func (s *Store) scope() contract.RegistryRelayScope {
	if s.Client != nil {
		return s.Client.NewRelayScope(s.NodeID, s.ScopeTTL)
	}
	now := time.Now().UTC()
	ttl := s.ScopeTTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return contract.RegistryRelayScope{
		NodeID: s.NodeID, IssuedAt: now, ExpiresAt: now.Add(ttl),
		Nonce: fmt.Sprintf("fallback-%d", atomic.AddUint64(&s.nonceSeq, 1)),
	}
}

func (s *Store) UpsertRuntimeNode(context.Context, contract.RuntimeNode) error {
	return registryrelay.ErrRelayNotSupported
}

func (s *Store) GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error) {
	if s.Client == nil {
		return nil, registryrelay.ErrRegistryUnavailable
	}
	if nodeID != s.NodeID {
		return nil, &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node read denied"}
	}
	node, err := s.Client.GetRuntimeNode(ctx, s.scope())
	return node, err
}

func (s *Store) ListRuntimeNodes(context.Context) ([]contract.RuntimeNode, error) {
	return nil, registryrelay.ErrRelayNotSupported
}

func (s *Store) GetRuntimeReplica(ctx context.Context, replicaID string) (*contract.RuntimeReplica, error) {
	if s.Client == nil {
		return nil, registryrelay.ErrRegistryUnavailable
	}
	return s.Client.GetRuntimeReplica(ctx, s.scope(), replicaID)
}

func (s *Store) ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error) {
	if s.Client == nil {
		return nil, registryrelay.ErrRegistryUnavailable
	}
	if scope.NodeID != s.NodeID {
		return nil, &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node scope denied"}
	}
	return s.Client.ValidateAgentAssignment(ctx, s.scope(), scope, now)
}

func (s *Store) ValidateAgentCleanupAssignment(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error) {
	if s.Client == nil {
		return nil, registryrelay.ErrRegistryUnavailable
	}
	if scope.NodeID != s.NodeID {
		return nil, &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node scope denied"}
	}
	return s.Client.ValidateAgentCleanupAssignment(ctx, s.scope(), scope)
}

func (s *Store) ListRuntimeReplicasByNode(ctx context.Context, nodeID string) ([]contract.RuntimeReplica, error) {
	if s.Client == nil {
		return nil, registryrelay.ErrRegistryUnavailable
	}
	if nodeID != s.NodeID {
		return nil, &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node read denied"}
	}
	return s.Client.ListRuntimeReplicasByNode(ctx, s.scope())
}

func (s *Store) RecordObservation(ctx context.Context, obs registry.ReplicaObservation) error {
	if s.Client == nil {
		return registryrelay.ErrRegistryUnavailable
	}
	if obs.NodeID != s.NodeID {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return s.Client.RecordObservation(ctx, s.scope(), obs)
}

func (s *Store) ClaimAgentCommand(ctx context.Context, command registry.AgentCommand) (registry.AgentCommandClaim, error) {
	if s.Client == nil {
		return registry.AgentCommandClaim{}, registryrelay.ErrRegistryUnavailable
	}
	if command.NodeID != s.NodeID {
		return registry.AgentCommandClaim{}, &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return s.Client.ClaimAgentCommand(ctx, s.scope(), command)
}

func (s *Store) RenewAgentCommandLease(ctx context.Context, command registry.AgentCommand, expiry time.Time) error {
	if s.Client == nil {
		return registryrelay.ErrRegistryUnavailable
	}
	if command.NodeID != s.NodeID {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return s.Client.RenewAgentCommandLease(ctx, s.scope(), command, expiry)
}

func (s *Store) CompleteAgentCommand(ctx context.Context, command registry.AgentCommand) error {
	if s.Client == nil {
		return registryrelay.ErrRegistryUnavailable
	}
	if command.NodeID != s.NodeID {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return s.Client.CompleteAgentCommand(ctx, s.scope(), command)
}

func (s *Store) RecordObservationAndCompleteAgentCommand(ctx context.Context, obs registry.ReplicaObservation, command registry.AgentCommand) error {
	if s.Client == nil {
		return registryrelay.ErrRegistryUnavailable
	}
	if obs.NodeID != s.NodeID || command.NodeID != s.NodeID {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return s.Client.RecordObservationAndCompleteAgentCommand(ctx, s.scope(), obs, command)
}

func (s *Store) WithdrawReplica(ctx context.Context, replicaID, projectID, versionID, nodeID string, generation int64) error {
	if s.Client == nil {
		return registryrelay.ErrRegistryUnavailable
	}
	if nodeID != s.NodeID {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return s.Client.WithdrawReplica(ctx, s.scope(), replicaID, projectID, versionID, generation)
}

func (s *Store) TerminalizeReplica(ctx context.Context, replicaID, nodeID string, generation int64, state contract.ReplicaState) error {
	if s.Client == nil {
		return registryrelay.ErrRegistryUnavailable
	}
	if nodeID != s.NodeID {
		return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed, Message: "cross-node mutation denied"}
	}
	return s.Client.TerminalizeReplica(ctx, s.scope(), replicaID, generation, state)
}

// Split returns the store as NodeStore and LifecycleStore facets.
func (s *Store) Split() (agent.NodeStore, agent.LifecycleStore) {
	return s, s
}
