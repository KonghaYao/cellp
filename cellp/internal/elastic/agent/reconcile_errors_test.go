package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

func TestReconcileNodeErrorFatal(t *testing.T) {
	if ReconcileNodeErrorFatal(nil) {
		t.Fatal("nil must not be fatal")
	}
	transient := fmt.Errorf("validate assignment: %w", errors.New("registry unavailable"))
	if ReconcileNodeErrorFatal(transient) {
		t.Fatal("transient read must not be fatal")
	}
	fatal := errors.Join(&CommandError{Reason: contract.ReasonGenerationStale, Message: "node unavailable"})
	if !ReconcileNodeErrorFatal(fatal) {
		t.Fatal("generation stale must be fatal")
	}
	wireStale := registrywire.AuthoritativeFromReason(contract.ReasonGenerationStale)
	if !ReconcileNodeErrorFatal(wireStale) {
		t.Fatal("remote generation_stale must be fatal")
	}
	lease := registrywire.AuthoritativeFromReason(contract.ReasonLeaseExpired)
	if !ReconcileNodeErrorFatal(lease) || !RuntimeNodeHeartbeatFatal(lease) {
		t.Fatal("authoritative lease expired must be fatal")
	}
	if !errors.Is(lease, registry.ErrLeaseExpired) {
		t.Fatal("wire lease must unwrap to registry sentinel")
	}
	if ReconcileNodeErrorFatal(registryrelay.ErrRegistryUnavailable) {
		t.Fatal("registry unavailable must not be fatal")
	}
	if !errors.Is(wireStale, registry.ErrObservationStale) {
		t.Fatal("wire stale must unwrap to registry sentinel")
	}
}

func TestRuntimeNodeHeartbeatFatal(t *testing.T) {
	if RuntimeNodeHeartbeatFatal(errors.New("sqlite busy")) {
		t.Fatal("generic error must not be fatal")
	}
	if !RuntimeNodeHeartbeatFatal(registry.ErrNodeLeaseCASConflict) {
		t.Fatal("lease CAS conflict must be fatal")
	}
}
