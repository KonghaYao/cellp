package registry

import (
	"context"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestElasticMigrationAndSnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/elastic.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rev, err := store.GetRouteRevision(ctx)
	if err != nil || rev != 0 {
		t.Fatalf("initial revision: %v err=%v", rev, err)
	}

	_, err = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRoute(ctx, Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	}); err != nil {
		t.Fatal(err)
	}
	rev2, err := store.GetRouteRevision(ctx)
	if err != nil || rev2 < 1 {
		t.Fatalf("bumped revision: %v err=%v", rev2, err)
	}

	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.EndpointSets) != 1 {
		t.Fatalf("endpoint sets: %+v", snap.EndpointSets)
	}
	if snap.EndpointSets[0].Endpoints[0].Address != "127.0.0.1:8792" {
		t.Fatalf("addr: %s", snap.EndpointSets[0].Endpoints[0].Address)
	}
}

func TestControllerGuard(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/guard.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.TryAcquireControllerGuard(ctx, "cellpd-a", 100); err != nil {
		t.Fatal(err)
	}
	if err := store.TryAcquireControllerGuard(ctx, "cellpd-b", 101); err != ErrControllerGuardHeld {
		t.Fatalf("want held, got %v", err)
	}
	if err := store.ReleaseControllerGuard(ctx, "cellpd-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.TryAcquireControllerGuard(ctx, "cellpd-b", 101); err != nil {
		t.Fatal(err)
	}
}

func TestServingPolicyAndDesiredCAS(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/policy.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _ = store.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})

	if err := store.UpsertServingPolicy(ctx, ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	pol, err := store.GetServingPolicy(ctx, "demo", "v1")
	if err != nil || pol == nil || !pol.ElasticEnrolled {
		t.Fatalf("policy: %+v err=%v", pol, err)
	}

	desire := ServingDesireRow{DesiredReplicas: 1, Generation: 1, Reason: "init"}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, desire); err != nil {
		t.Fatal(err)
	}
	desire.Generation = 2
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 1, desire); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 1, desire); err != ErrDesiredCASConflict {
		t.Fatalf("want cas conflict, got %v", err)
	}
}

func TestRuntimeNodesSQLite(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/nodes.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	exp := time.Now().UTC().Add(2 * time.Hour)
	node := contract.RuntimeNode{
		NodeID:        "local-1",
		CapacityUnits: 8,
		Generation:    3,
		LeaseExpiry:   exp,
	}
	if err := store.UpsertRuntimeNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRuntimeNode(ctx, "local-1")
	if err != nil || got == nil || got.CapacityUnits != 8 {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	node.Cordoned = true
	if err := store.UpsertRuntimeNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListRuntimeNodes(ctx)
	if err != nil || len(list) != 1 || !list[0].Cordoned {
		t.Fatalf("list: %+v err=%v", list, err)
	}
}
