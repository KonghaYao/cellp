package registrywire_test

import (
	"errors"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

func TestAuthoritativeErrIsRegistrySentinels(t *testing.T) {
	stale := registrywire.AuthoritativeFromReason(contract.ReasonGenerationStale)
	if !errors.Is(stale, registry.ErrObservationStale) {
		t.Fatal("generation_stale must match ErrObservationStale")
	}
	if !errors.Is(stale, registry.ErrNodeLeaseCASConflict) {
		t.Fatal("generation_stale must match ErrNodeLeaseCASConflict")
	}
	lease := registrywire.AuthoritativeFromReason(contract.ReasonLeaseExpired)
	if !errors.Is(lease, registry.ErrLeaseExpired) {
		t.Fatal("lease_expired must match ErrLeaseExpired")
	}
	node := registrywire.AuthoritativeFromReason(contract.ReasonRuntimeNodeNotFound)
	if !errors.Is(node, registry.ErrRuntimeNodeNotFound) || errors.Is(node, registry.ErrRuntimeReplicaNotFound) {
		t.Fatalf("runtime_node_not_found mapping: %v", node)
	}
	replica := registrywire.AuthoritativeFromReason(contract.ReasonRuntimeReplicaNotFound)
	if !errors.Is(replica, registry.ErrRuntimeReplicaNotFound) || errors.Is(replica, registry.ErrRuntimeNodeNotFound) {
		t.Fatalf("runtime_replica_not_found mapping: %v", replica)
	}
}
