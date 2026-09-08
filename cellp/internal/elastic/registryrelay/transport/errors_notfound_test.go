package transport

import (
	"errors"
	"testing"

	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/registry"
)

func TestDecodeRelayTypedNotFoundSentinels(t *testing.T) {
	nodeErr := decodeRelayError(404, []byte(`{"reason":"runtime_node_not_found"}`))
	nodeMapped := registryrelay.MapClientError(nodeErr)
	if !errors.Is(nodeMapped, registry.ErrRuntimeNodeNotFound) || errors.Is(nodeMapped, registry.ErrRuntimeReplicaNotFound) {
		t.Fatalf("node wire mapping: %v", nodeMapped)
	}
	replicaErr := decodeRelayError(404, []byte(`{"reason":"runtime_replica_not_found"}`))
	replicaMapped := registryrelay.MapClientError(replicaErr)
	if !errors.Is(replicaMapped, registry.ErrRuntimeReplicaNotFound) || errors.Is(replicaMapped, registry.ErrRuntimeNodeNotFound) {
		t.Fatalf("replica wire mapping: %v", replicaMapped)
	}
	legacy := registryrelay.MapClientError(decodeRelayError(404, []byte(`{"reason":"not_found"}`)))
	if errors.Is(legacy, registry.ErrRuntimeNodeNotFound) || errors.Is(legacy, registry.ErrRuntimeReplicaNotFound) {
		t.Fatalf("generic not_found must stay untyped: %v", legacy)
	}
	unavail := decodeRelayError(503, []byte(`{"reason":"registry_unavailable"}`))
	if !registryrelay.IsRegistryUnavailable(registryrelay.MapClientError(unavail)) {
		t.Fatalf("expected registry unavailable, got %v", unavail)
	}
	unknown := decodeRelayError(500, []byte(`not-json`))
	if !registryrelay.IsRegistryUnavailable(registryrelay.MapClientError(unknown)) {
		t.Fatalf("unknown store must map to unavailable: %v", unknown)
	}
}
