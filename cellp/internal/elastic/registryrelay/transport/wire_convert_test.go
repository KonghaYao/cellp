package transport_test

import (
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay/transport"
)

func TestWireRuntimeNodeRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	in := &contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Cordoned: false, LeaseExpiry: now,
		Generation: 3, AgentBaseURL: "https://n1.agent.example",
		IdentityURI: "spiffe://cellp/test/node/n1", Zone: "z",
	}
	w := transport.RuntimeNodeToWire(in)
	out := transport.RuntimeNodeFromWire(w)
	if out == nil || out.AgentBaseURL != in.AgentBaseURL || out.IdentityURI != in.IdentityURI || out.Zone != in.Zone {
		t.Fatalf("node roundtrip: %+v", out)
	}
}

func TestWireRuntimeReplicaRoundTrip(t *testing.T) {
	valid := time.Now().UTC().Add(time.Hour)
	in := &contract.RuntimeReplica{
		ReplicaID: "r1", ProjectID: "p", VersionID: "v", NodeID: "n1",
		Generation: 2, AssignedNodeGeneration: 3, State: contract.ReplicaReady, ValidUntil: &valid,
	}
	w := transport.RuntimeReplicaToWire(in)
	out := transport.RuntimeReplicaFromWire(w)
	if out == nil || out.AssignedNodeGeneration != 3 {
		t.Fatalf("replica roundtrip: %+v", out)
	}
}
