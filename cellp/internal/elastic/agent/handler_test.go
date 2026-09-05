package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

type memStores struct {
	nodes map[string]contract.RuntimeNode
	reps  map[string]contract.RuntimeReplica
}

func newMemStores() *memStores {
	return &memStores{nodes: map[string]contract.RuntimeNode{}, reps: map[string]contract.RuntimeReplica{}}
}

func (m *memStores) UpsertRuntimeNode(_ context.Context, node contract.RuntimeNode) error {
	m.nodes[node.NodeID] = node
	return nil
}

func (m *memStores) GetRuntimeNode(_ context.Context, nodeID string) (*contract.RuntimeNode, error) {
	n, ok := m.nodes[nodeID]
	if !ok {
		return nil, nil
	}
	return &n, nil
}

func (m *memStores) ListRuntimeNodes(context.Context) ([]contract.RuntimeNode, error) {
	out := make([]contract.RuntimeNode, 0, len(m.nodes))
	for _, n := range m.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (m *memStores) UpsertRuntimeReplica(_ context.Context, rep contract.RuntimeReplica) error {
	m.reps[rep.ReplicaID] = rep
	return nil
}

func (m *memStores) ListRuntimeReplicas(_ context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error) {
	var out []contract.RuntimeReplica
	for _, r := range m.reps {
		if r.ProjectID == projectID && r.VersionID == versionID {
			out = append(out, r)
		}
	}
	return out, nil
}

func testScope() contract.CommandScope {
	return contract.CommandScope{
		NodeID:      "node-a",
		ProjectID:   "demo",
		VersionID:   "v1",
		ReplicaID:   "rep-1",
		Generation:  2,
		LeaseExpiry: time.Now().UTC().Add(time.Hour),
		Nonce:       "n-1",
		Action:      contract.ActionStartReplica,
	}
}

func TestHandlerDisabled(t *testing.T) {
	mem := newMemStores()
	h := NewHandler(false, mem, mem)
	_, err := h.StartReplica(context.Background(), contract.StartReplicaSpec{
		Scope:  testScope(),
		Bucket: "b-demo-v1",
	}, "idem")
	var cmd *CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonElasticDisabled {
		t.Fatalf("want elastic_disabled, got %v", err)
	}
}

func TestStartStopProbeLifecycle(t *testing.T) {
	ctx := context.Background()
	mem := newMemStores()
	_ = mem.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID:        "node-a",
		CapacityUnits: 4,
		Generation:    1,
		LeaseExpiry:   time.Now().UTC().Add(time.Hour),
	})
	h := NewHandler(true, mem, mem)
	scope := testScope()
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "b-demo-v1"}
	rep, err := h.StartReplica(ctx, spec, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if rep.State != contract.ReplicaStarting {
		t.Fatalf("state: %s", rep.State)
	}
	pr, err := h.ProbeReplica(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if pr.State != contract.ReplicaStarting {
		t.Fatalf("probe state: %s", pr.State)
	}
	stopped, err := h.StopReplica(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != contract.ReplicaStopped {
		t.Fatalf("stop state: %s", stopped.State)
	}
}

func TestCordonedNodeRejected(t *testing.T) {
	ctx := context.Background()
	mem := newMemStores()
	_ = mem.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID:        "node-a",
		CapacityUnits: 1,
		Cordoned:      true,
		Generation:    1,
		LeaseExpiry:   time.Now().UTC().Add(time.Hour),
	})
	h := NewHandler(true, mem, mem)
	_, err := h.StartReplica(ctx, contract.StartReplicaSpec{
		Scope:  testScope(),
		Bucket: "b",
	}, "")
	if !errors.Is(err, errNodeCordoned) {
		t.Fatalf("want cordoned, got %v", err)
	}
}
