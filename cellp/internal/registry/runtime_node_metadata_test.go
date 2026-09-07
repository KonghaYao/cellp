package registry

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	_ "modernc.org/sqlite"
)

func TestRuntimeNodeMetadataRoundTripReopenAndGenerationCAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	node := contract.RuntimeNode{NodeID: "node-a", CapacityUnits: 4, Generation: 2, LeaseExpiry: time.Now().UTC().Add(time.Hour), AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/node-a", Zone: "local-a"}
	if err := store.UpsertRuntimeNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.GetRuntimeNode(context.Background(), node.NodeID)
	if err != nil || got.AgentBaseURL != node.AgentBaseURL || got.IdentityURI != node.IdentityURI || got.Zone != node.Zone {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	competing := node
	if err := store.ActivateRuntimeNode(context.Background(), competing, node.Generation-1); err != ErrNodeLeaseCASConflict {
		t.Fatalf("activation CAS err=%v", err)
	}
	old := node
	old.Generation = 1
	old.AgentBaseURL = "https://evil.example"
	if err := store.UpsertRuntimeNode(context.Background(), old); err != ErrNodeLeaseCASConflict {
		t.Fatalf("old writer err=%v", err)
	}
	got, _ = store.GetRuntimeNode(context.Background(), node.NodeID)
	if got.AgentBaseURL != node.AgentBaseURL || got.Generation != node.Generation {
		t.Fatalf("old writer changed node: %+v", got)
	}
	if err := store.RenewRuntimeNodeLease(context.Background(), node.NodeID, node.Generation-1, time.Now().UTC().Add(2*time.Hour)); err != ErrNodeLeaseCASConflict {
		t.Fatalf("renew CAS err=%v", err)
	}
}

func TestUpsertRuntimeNodeRejectsHigherGenerationMetadataSubstitution(t *testing.T) {
	store, err := Open(t.TempDir() + "/meta-sub.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exp := time.Now().UTC().Add(time.Hour)
	node := contract.RuntimeNode{
		NodeID: "node-a", CapacityUnits: 4, Generation: 2, LeaseExpiry: exp,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/node-a", Zone: "local-a",
	}
	if err := store.UpsertRuntimeNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	attack := node
	attack.Generation = 3
	attack.AgentBaseURL = "https://evil.example"
	if err := store.UpsertRuntimeNode(context.Background(), attack); err != ErrNodeLeaseCASConflict {
		t.Fatalf("expected metadata substitution rejection, got %v", err)
	}
	got, _ := store.GetRuntimeNode(context.Background(), node.NodeID)
	if got.AgentBaseURL != node.AgentBaseURL || got.Generation != node.Generation {
		t.Fatalf("metadata mutated: %+v", got)
	}
}

func TestUpsertRuntimeNodeSameGenerationHeartbeatPreservesMetadata(t *testing.T) {
	store, err := Open(t.TempDir() + "/meta-hb.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exp := time.Now().UTC().Add(time.Hour)
	node := contract.RuntimeNode{
		NodeID: "node-a", CapacityUnits: 4, Generation: 2, LeaseExpiry: exp,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/node-a", Zone: "local-a",
	}
	if err := store.UpsertRuntimeNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	renew := node
	renew.LeaseExpiry = exp.Add(30 * time.Minute)
	renew.CapacityUnits = 6
	if err := store.UpsertRuntimeNode(context.Background(), renew); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRuntimeNode(context.Background(), node.NodeID)
	if got.CapacityUnits != 6 || got.AgentBaseURL != node.AgentBaseURL || got.IdentityURI != node.IdentityURI {
		t.Fatalf("heartbeat update wrong: %+v", got)
	}
}

func TestRuntimeNodeMetadataMigratesOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE runtime_nodes (node_id TEXT PRIMARY KEY, capacity_units INTEGER NOT NULL DEFAULT 0, cordoned INTEGER NOT NULL DEFAULT 0, lease_expiry TEXT NOT NULL, generation INTEGER NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	node := contract.RuntimeNode{NodeID: "old", CapacityUnits: 1, Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Hour)}
	if err := store.UpsertRuntimeNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRuntimeNode(context.Background(), "old")
	if err != nil || got.AgentBaseURL != "" || got.IdentityURI != "" || got.Zone != "" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
