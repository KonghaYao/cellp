package contract

import (
	"fmt"
	"strings"
	"time"
)

// RegistryRelayScope binds registry relay RPCs to a node identity and replay window.
type RegistryRelayScope struct {
	NodeID    string    `json:"node_id"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Nonce     string    `json:"nonce"`
}

// ValidateRegistryRelayScope checks bounded fields and the request time window.
func ValidateRegistryRelayScope(scope RegistryRelayScope, now time.Time) error {
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
	if err := validateControlPlaneScopeWindow(issued, expires, now, "relay scope expired"); err != nil {
		return err
	}
	return nil
}
