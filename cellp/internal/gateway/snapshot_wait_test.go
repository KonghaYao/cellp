package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

func seedElasticQualificationFixture(t *testing.T, ctx context.Context, store registry.Store) int64 {
	t.Helper()
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 2, ElasticEnrolled: true,
		BackgroundMode: contract.BackgroundModeNone,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().UTC().Add(time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, LeaseExpiry: valid,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}); err != nil {
		t.Fatal(err)
	}
	epUntil := time.Now().UTC().Add(time.Hour)
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: 8792,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
	}); err != nil {
		t.Fatal(err)
	}
	rev, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func TestWaitRouteSnapshotPublished(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/snap-wait.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rev := seedElasticQualificationFixture(t, ctx, store)

	h := NewRouteSnapshotHolder()
	if err := h.WaitRouteSnapshotPublished(ctx, store, rev, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	h.PollOnce(ctx, store)
	if _, ok := h.LookupUpstreamFromSnapshot("demo", "v1"); ok {
		t.Fatal("public LKG holder must not expose deploy_ready qualification endpoint")
	}
	q, ok, err := store.BuildQualificationViewAfter(ctx, rev-1)
	if err != nil || !ok || len(q.EndpointSets) != 1 {
		t.Fatalf("qualification view: ok=%v err=%v sets=%+v", ok, err, q.EndpointSets)
	}
}

func TestGatewayDoesNotRouteDeployReadySnapshotEndpoint(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/snap-closed.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_ = seedElasticQualificationFixture(t, ctx, store)
	host := "v1.demo.ingress.local"
	versionID := "v1"
	if err := store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: "preview:demo:v1", ProjectID: "demo", VersionID: &versionID,
		Role: registry.IngressRolePreview, Host: &host, SyntheticHost: host, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	rev, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, ok, err := store.BuildQualificationViewAfter(ctx, rev-1)
	if err != nil || !ok || len(q.EndpointSets) == 0 {
		t.Fatalf("qualification view missing endpoint: ok=%v err=%v sets=%+v", ok, err, q.EndpointSets)
	}

	g := NewWithConfig(store, GatewayConfig{GatewayPort: 8787})
	g.RouteSnapshotHolder().PollOnce(ctx, store)
	if _, ok := g.RouteSnapshotHolder().LookupUpstreamFromSnapshot("demo", "v1"); ok {
		t.Fatal("public snapshot must not expose deploy_ready endpoint")
	}
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	req.Host = host
	rr := httptest.NewRecorder()
	g.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("deploy_ready route status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
	}
}
