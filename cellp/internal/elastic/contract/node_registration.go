package contract

import (
	"fmt"
	"strings"
	"time"
)

// NodeRegistrationScope binds node lease RPCs to identity, generation fencing, and replay windows.
type NodeRegistrationScope struct {
	NodeID     string    `json:"node_id"`
	Generation int64     `json:"generation"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Nonce      string    `json:"nonce"`
}

// ActivateNodeRequest registers or restarts a runtime node generation on the active controller.
type ActivateNodeRequest struct {
	Scope              NodeRegistrationScope `json:"scope"`
	CapacityUnits      int                   `json:"capacity_units"`
	AgentBaseURL       string                `json:"agent_base_url"`
	IdentityURI        string                `json:"identity_uri"`
	Zone               string                `json:"zone"`
	ExpectedGeneration int64                 `json:"expected_generation"`
}

// ActivateNodeResponse returns the authoritative generation and lease after CAS activation.
type ActivateNodeResponse struct {
	Generation  int64     `json:"generation"`
	LeaseExpiry time.Time `json:"lease_expiry"`
}

// HeartbeatNodeRequest renews a generation-fenced node lease.
type HeartbeatNodeRequest struct {
	Scope       NodeRegistrationScope `json:"scope"`
	LeaseExpiry time.Time             `json:"lease_expiry"`
}

// CordonNodeRequest marks the node cordoned for the exact generation (no new assignments).
type CordonNodeRequest struct {
	Scope NodeRegistrationScope `json:"scope"`
}

// ReleaseNodeRequest expires the node lease for the exact generation (shutdown).
type ReleaseNodeRequest struct {
	Scope NodeRegistrationScope `json:"scope"`
}

// StatusNodeRequest reads registry node metadata for restart fencing.
type StatusNodeRequest struct {
	Scope NodeRegistrationScope `json:"scope"`
}

// StatusNodeResponse exposes the current registry generation when present.
type StatusNodeResponse struct {
	Found       bool      `json:"found"`
	Generation  int64     `json:"generation"`
	LeaseExpiry time.Time `json:"lease_expiry"`
	Cordoned    bool      `json:"cordoned"`
}

// ValidateNodeRegistrationScope checks bounded fields and time window.
func ValidateNodeRegistrationScope(scope NodeRegistrationScope, now time.Time) error {
	if strings.TrimSpace(scope.NodeID) == "" {
		return fmt.Errorf("node_id required")
	}
	if len(scope.NodeID) > MaxCommandFieldLen {
		return fmt.Errorf("node_id too long")
	}
	nonce := strings.TrimSpace(scope.Nonce)
	if nonce == "" || len(nonce) > MaxCommandNonceLen {
		return fmt.Errorf("nonce required")
	}
	now = now.UTC()
	if scope.IssuedAt.IsZero() || scope.ExpiresAt.IsZero() {
		return fmt.Errorf("issued_at and expires_at required")
	}
	issued := scope.IssuedAt.UTC()
	expires := scope.ExpiresAt.UTC()
	if err := validateControlPlaneScopeWindow(issued, expires, now, "registration scope expired"); err != nil {
		return err
	}
	return nil
}
