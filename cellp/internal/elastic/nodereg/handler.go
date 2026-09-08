package nodereg

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// ValidateHandlerConfig checks allowlist bijection before serving node registration.
func ValidateHandlerConfig(h *Handler) error {
	if h == nil || h.Store == nil {
		return errors.New("nodereg store unavailable")
	}
	return contract.ValidateNodeRegistrationAllowlist(h.Allowlist)
}

// GuardChecker ensures only the active controller writer accepts node registration.
type GuardChecker interface {
	EnsureActiveController(ctx context.Context) error
}

// Handler applies generation-fenced runtime node registration to the registry.
type Handler struct {
	Store        registry.ServingStore
	Allowlist    map[string]contract.NodeRegistrationBinding // node_id -> identity + authorized agent URL
	Guard        GuardChecker
	HeartbeatTTL time.Duration
	Now          func() time.Time
}

func (h *Handler) now() time.Time {
	if h != nil && h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

func (h *Handler) ensure(ctx context.Context) error {
	if h == nil || h.Store == nil {
		return ErrRegistryUnavailable
	}
	if err := contract.ValidateNodeRegistrationAllowlist(h.Allowlist); err != nil {
		return &HandlerError{Reason: contract.ReasonAuthFailed, Message: "invalid allowlist", HTTPStatus: http.StatusBadRequest}
	}
	if h.Guard != nil {
		if err := h.Guard.EnsureActiveController(ctx); err != nil {
			return errors.Join(ErrRegistryUnavailable, err)
		}
	}
	return nil
}

func (h *Handler) authFailed(msg string) error {
	return &HandlerError{Reason: contract.ReasonAuthFailed, Message: msg, HTTPStatus: http.StatusUnauthorized}
}

func (h *Handler) authFailedBadRequest(msg string) error {
	return &HandlerError{Reason: contract.ReasonAuthFailed, Message: msg, HTTPStatus: http.StatusBadRequest}
}

func (h *Handler) scopeFailed(err error) error {
	if err == nil {
		return nil
	}
	return &HandlerError{Reason: contract.ReasonAuthFailed, Message: err.Error(), HTTPStatus: http.StatusBadRequest}
}

func (h *Handler) ensureBinding(nodeID, peerIdentity string) (contract.NodeRegistrationBinding, error) {
	nodeID = strings.TrimSpace(nodeID)
	peerIdentity = strings.TrimSpace(peerIdentity)
	if nodeID == "" || peerIdentity == "" {
		return contract.NodeRegistrationBinding{}, h.authFailed("node identity binding required")
	}
	binding, ok := h.Allowlist[nodeID]
	if !ok {
		return contract.NodeRegistrationBinding{}, h.authFailed("node not allowlisted")
	}
	if strings.TrimSpace(binding.IdentityURI) != peerIdentity {
		return contract.NodeRegistrationBinding{}, h.authFailed("node not allowlisted")
	}
	return binding, nil
}

func (h *Handler) ensureRegisteredNode(ctx context.Context, peerIdentity, nodeID string, scopeGeneration int64, requireActiveLease bool) error {
	if _, err := h.ensureBinding(nodeID, peerIdentity); err != nil {
		return err
	}
	node, err := h.Store.GetRuntimeNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return registry.ErrNodeLeaseCASConflict
	}
	if strings.TrimSpace(node.IdentityURI) != peerIdentity {
		return h.authFailed("identity uri mismatch")
	}
	if scopeGeneration > 0 && node.Generation != scopeGeneration {
		return registry.ErrNodeLeaseCASConflict
	}
	now := h.now()
	if requireActiveLease && !node.LeaseExpiry.After(now) {
		return registry.ErrLeaseExpired
	}
	return nil
}

// Activate registers or bumps node generation with CAS fencing.
func (h *Handler) Activate(ctx context.Context, peerIdentity string, req contract.ActivateNodeRequest) (contract.ActivateNodeResponse, error) {
	if err := h.ensure(ctx); err != nil {
		return contract.ActivateNodeResponse{}, err
	}
	now := h.now()
	if err := contract.ValidateNodeRegistrationScope(req.Scope, now); err != nil {
		return contract.ActivateNodeResponse{}, h.scopeFailed(err)
	}
	binding, err := h.ensureBinding(req.Scope.NodeID, peerIdentity)
	if err != nil {
		return contract.ActivateNodeResponse{}, err
	}
	configuredURI := strings.TrimSpace(binding.IdentityURI)
	if strings.TrimSpace(req.IdentityURI) != peerIdentity || peerIdentity != configuredURI {
		return contract.ActivateNodeResponse{}, h.authFailed("identity uri mismatch")
	}
	declaredURL, err := contract.CanonicalAgentBaseURL(req.AgentBaseURL)
	if err != nil {
		return contract.ActivateNodeResponse{}, h.authFailedBadRequest("agent base url mismatch")
	}
	authorizedURL, err := contract.CanonicalAgentBaseURL(binding.AgentBaseURL)
	if err != nil {
		return contract.ActivateNodeResponse{}, &HandlerError{Reason: contract.ReasonAuthFailed, Message: "invalid allowlist", HTTPStatus: http.StatusBadRequest}
	}
	if declaredURL != authorizedURL {
		return contract.ActivateNodeResponse{}, h.authFailedBadRequest("agent base url mismatch")
	}
	existing, err := h.Store.GetRuntimeNode(ctx, req.Scope.NodeID)
	if err != nil {
		return contract.ActivateNodeResponse{}, err
	}
	if existing != nil && strings.TrimSpace(existing.IdentityURI) != "" && existing.IdentityURI != configuredURI {
		return contract.ActivateNodeResponse{}, registry.ErrNodeLeaseCASConflict
	}
	generation, err := nextGeneration(existing, now)
	if err != nil {
		return contract.ActivateNodeResponse{}, err
	}
	if req.ExpectedGeneration != generation-1 {
		return contract.ActivateNodeResponse{}, registry.ErrNodeLeaseCASConflict
	}
	ttl := h.HeartbeatTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	lease := now.Add(ttl)
	node := contract.RuntimeNode{
		NodeID:        req.Scope.NodeID,
		CapacityUnits: req.CapacityUnits,
		Generation:    generation,
		LeaseExpiry:   lease,
		AgentBaseURL:  declaredURL,
		IdentityURI:   req.IdentityURI,
		Zone:          req.Zone,
	}
	policy := contract.AgentBaseURLPolicyForBinding(binding)
	if err := contract.ValidateRuntimeNodeActivationMetadataWithPolicy(node, policy); err != nil {
		return contract.ActivateNodeResponse{}, h.authFailedBadRequest("invalid node metadata")
	}
	if err := h.Store.ActivateRuntimeNode(ctx, node, generation-1); err != nil {
		return contract.ActivateNodeResponse{}, err
	}
	return contract.ActivateNodeResponse{Generation: generation, LeaseExpiry: lease}, nil
}

// Heartbeat renews the node lease for the exact generation.
func (h *Handler) Heartbeat(ctx context.Context, peerIdentity string, req contract.HeartbeatNodeRequest) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	now := h.now()
	if err := contract.ValidateNodeRegistrationScope(req.Scope, now); err != nil {
		return h.scopeFailed(err)
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, req.Scope.NodeID, req.Scope.Generation, true); err != nil {
		return err
	}
	ttl := h.HeartbeatTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	lease := now.Add(ttl)
	return h.Store.RenewRuntimeNodeLease(ctx, req.Scope.NodeID, req.Scope.Generation, lease)
}

// Cordon marks the node cordoned for the exact generation.
func (h *Handler) Cordon(ctx context.Context, peerIdentity string, req contract.CordonNodeRequest) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	now := h.now()
	if err := contract.ValidateNodeRegistrationScope(req.Scope, now); err != nil {
		return h.scopeFailed(err)
	}
	if err := h.ensureRegisteredNode(ctx, peerIdentity, req.Scope.NodeID, req.Scope.Generation, true); err != nil {
		return err
	}
	return h.Store.CordonRuntimeNode(ctx, req.Scope.NodeID, req.Scope.Generation)
}

// Release expires the node lease for the exact generation.
func (h *Handler) Release(ctx context.Context, peerIdentity string, req contract.ReleaseNodeRequest) error {
	if err := h.ensure(ctx); err != nil {
		return err
	}
	now := h.now()
	if err := contract.ValidateNodeRegistrationScope(req.Scope, now); err != nil {
		return h.scopeFailed(err)
	}
	// Release after lease expiry is a no-op at the store CAS layer; still require identity/generation binding.
	if err := h.ensureRegisteredNode(ctx, peerIdentity, req.Scope.NodeID, req.Scope.Generation, false); err != nil {
		return err
	}
	return h.Store.ReleaseRuntimeNodeLease(ctx, req.Scope.NodeID, req.Scope.Generation)
}

// Status returns registry metadata for restart fencing.
func (h *Handler) Status(ctx context.Context, peerIdentity string, req contract.StatusNodeRequest) (contract.StatusNodeResponse, error) {
	if err := h.ensure(ctx); err != nil {
		return contract.StatusNodeResponse{}, err
	}
	now := h.now()
	if err := contract.ValidateNodeRegistrationScope(req.Scope, now); err != nil {
		return contract.StatusNodeResponse{}, h.scopeFailed(err)
	}
	if _, err := h.ensureBinding(req.Scope.NodeID, peerIdentity); err != nil {
		return contract.StatusNodeResponse{}, err
	}
	node, err := h.Store.GetRuntimeNode(ctx, req.Scope.NodeID)
	if err != nil {
		return contract.StatusNodeResponse{}, err
	}
	if node == nil {
		return contract.StatusNodeResponse{Found: false}, nil
	}
	if strings.TrimSpace(node.IdentityURI) != peerIdentity {
		return contract.StatusNodeResponse{}, h.authFailed("identity uri mismatch")
	}
	if req.Scope.Generation > 0 && node.Generation != req.Scope.Generation {
		return contract.StatusNodeResponse{}, registry.ErrNodeLeaseCASConflict
	}
	return contract.StatusNodeResponse{
		Found: true, Generation: node.Generation, LeaseExpiry: node.LeaseExpiry, Cordoned: node.Cordoned,
	}, nil
}

func nextGeneration(existing *contract.RuntimeNode, now time.Time) (int64, error) {
	if existing == nil {
		return 1, nil
	}
	if existing.LeaseExpiry.After(now) {
		return 0, registry.ErrNodeLeaseCASConflict
	}
	return existing.Generation + 1, nil
}
