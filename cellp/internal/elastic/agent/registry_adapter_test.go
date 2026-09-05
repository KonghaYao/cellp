package agent

import (
	"context"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

func TestNewFromRegistryIntegration(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/agent.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	exp := time.Now().UTC().Add(time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 1, LeaseExpiry: exp,
	}); err != nil {
		t.Fatal(err)
	}

	h := NewFromRegistry(true, store)
	scope := contract.CommandScope{
		NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "r1",
		Generation: 2, LeaseExpiry: exp, Nonce: "x", Action: contract.ActionStartReplica,
	}
	rep, err := h.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "b-demo-v1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.ReplicaID != "r1" {
		t.Fatalf("replica: %+v", rep)
	}
}
