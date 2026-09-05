package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestRouteSnapshotHolderPoll(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/snap.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 9001,
	})

	h := NewRouteSnapshotHolder()
	h.PollOnce(ctx, store)
	_, ok := h.Snapshot()
	if !ok {
		t.Fatal("expected LKG after poll")
	}
	addr, found := h.LookupUpstreamFromSnapshot("demo", "v1")
	if !found || addr != "127.0.0.1:9001" {
		t.Fatalf("upstream %q found=%v", addr, found)
	}
	if h.LastAppliedRevision() < 1 {
		t.Fatal("expected revision bump")
	}

	_ = store.SetRouteActive(ctx, "demo", "v1", false)
	h.PollOnce(ctx, store)
	addr2, found2 := h.LookupUpstreamFromSnapshot("demo", "v1")
	if found2 {
		t.Fatalf("inactive route should not appear in snapshot endpoints: %s", addr2)
	}
}

func TestStartSnapshotPoller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := registry.Open(t.TempDir() + "/poll.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	h := NewRouteSnapshotHolder()
	StartSnapshotPoller(ctx, store, h, 50*time.Millisecond)
	time.Sleep(120 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond)
}
