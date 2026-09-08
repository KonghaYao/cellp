package registryrelay_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

func TestMapRegistryErrorSemantics(t *testing.T) {
	stale := registryrelay.MapRegistryError(registry.ErrObservationStale)
	var relay *registryrelay.RelayError
	if !errors.As(stale, &relay) || relay.Reason != contract.ReasonGenerationStale {
		t.Fatalf("stale: %v", stale)
	}
	lease := registryrelay.MapRegistryError(registry.ErrLeaseExpired)
	if !errors.As(lease, &relay) || relay.Reason != contract.ReasonLeaseExpired {
		t.Fatalf("lease: %v", lease)
	}
	unknown := registryrelay.MapRegistryError(errors.New("sqlite: disk I/O error"))
	if !registryrelay.IsRegistryUnavailable(unknown) {
		t.Fatalf("unknown store: %v", unknown)
	}
	wrapped := fmt.Errorf("load replica: %w", registry.ErrRuntimeReplicaNotFound)
	notFound := registryrelay.MapRegistryError(wrapped)
	if !errors.As(notFound, &relay) || relay.Reason != contract.ReasonRuntimeReplicaNotFound {
		t.Fatalf("wrapped replica not found: %v", notFound)
	}
	wrappedNode := fmt.Errorf("load node: %w", registry.ErrRuntimeNodeNotFound)
	nodeNF := registryrelay.MapRegistryError(wrappedNode)
	if !errors.As(nodeNF, &relay) || relay.Reason != contract.ReasonRuntimeNodeNotFound {
		t.Fatalf("wrapped node not found: %v", nodeNF)
	}
	restored := registryrelay.MapClientError(notFound)
	if !errors.Is(restored, registry.ErrRuntimeReplicaNotFound) || errors.Is(restored, registry.ErrRuntimeNodeNotFound) {
		t.Fatalf("replica sentinel must not match node: %v", restored)
	}
	restoredNode := registryrelay.MapClientError(nodeNF)
	if !errors.Is(restoredNode, registry.ErrRuntimeNodeNotFound) || errors.Is(restoredNode, registry.ErrRuntimeReplicaNotFound) {
		t.Fatalf("node sentinel must not match replica: %v", restoredNode)
	}
	legacy := registryrelay.MapClientError(&registryrelay.RelayError{Reason: contract.ReasonNotFound})
	if errors.Is(legacy, registry.ErrRuntimeNodeNotFound) || errors.Is(legacy, registry.ErrRuntimeReplicaNotFound) {
		t.Fatalf("generic not_found must not map to runtime sentinels: %v", legacy)
	}
	unknownNF := registryrelay.MapRegistryError(errors.New("totally unknown"))
	if !registryrelay.IsRegistryUnavailable(unknownNF) {
		t.Fatal("unknown error must stay unavailable")
	}
	wireStale := registryrelay.MapClientError(registrywire.AuthoritativeFromReason(contract.ReasonGenerationStale))
	if !errors.Is(wireStale, registry.ErrObservationStale) {
		t.Fatalf("client stale: %v", wireStale)
	}
	if !registryrelay.IsRegistryUnavailable(errors.Join(registryrelay.ErrRegistryUnavailable, context.DeadlineExceeded)) {
		t.Fatal("expected unavailable")
	}
}
