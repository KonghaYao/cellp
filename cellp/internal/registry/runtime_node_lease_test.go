package registry

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestReleaseRuntimeNodeLeaseGenerationCAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	node := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 1,
		LeaseExpiry:  time.Now().UTC().Add(time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z1",
	}
	if err := store.ActivateRuntimeNode(ctx, node, 0); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseRuntimeNodeLease(ctx, "n1", 1); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRuntimeNode(ctx, "n1")
	if err != nil || got.LeaseExpiry.After(time.Now().UTC()) {
		t.Fatalf("lease should be expired: %+v err=%v", got, err)
	}
	if err := store.ReleaseRuntimeNodeLease(ctx, "n1", 1); err != ErrNodeLeaseCASConflict {
		t.Fatalf("second release err=%v", err)
	}
	if err := store.ReleaseRuntimeNodeLease(ctx, "n1", 2); err != ErrNodeLeaseCASConflict {
		t.Fatalf("wrong generation err=%v", err)
	}
}

func TestActivateRuntimeNodeRejectsBadMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	node := contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Hour),
		AgentBaseURL: "http://127.0.0.1:1", IdentityURI: "spiffe://cellp/local/node/n1", Zone: "z",
	}
	if err := store.ActivateRuntimeNode(context.Background(), node, 0); err == nil {
		t.Fatal("expected metadata rejection")
	}
}

func TestUpsertRuntimeNodeAllowsEmptyMetadataLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	node := contract.RuntimeNode{NodeID: "old", CapacityUnits: 1, Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Hour)}
	if err := store.UpsertRuntimeNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
}
