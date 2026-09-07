package orch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

type integrationSnapshotAck struct {
	t                *testing.T
	wantDeployReady  bool
	fail             error
	called           bool
	observedRevision int64
}

func (a *integrationSnapshotAck) WaitPublished(ctx context.Context, store registry.Store, minRevision int64, projectID, versionID string) error {
	a.t.Helper()
	a.called = true
	v, err := store.GetVersion(ctx, projectID, versionID)
	if err != nil {
		a.t.Fatal(err)
	}
	if a.wantDeployReady && (v == nil || v.Status != registry.StatusDeployReady) {
		a.t.Fatalf("snapshot acknowledgement observed status %+v, want deploy_ready", v)
	}
	q, ok, err := store.BuildQualificationViewAfter(ctx, minRevision-1)
	if err != nil {
		a.t.Fatal(err)
	}
	if !ok || q.RouteRevision < minRevision {
		a.t.Fatalf("qualification view revision=%d want >=%d ok=%v", q.RouteRevision, minRevision, ok)
	}
	a.observedRevision = q.RouteRevision
	for _, set := range q.EndpointSets {
		if set.ProjectID == projectID && set.VersionID == versionID && len(set.Endpoints) > 0 {
			return a.fail
		}
	}
	a.t.Fatalf("deploy_ready endpoint absent from qualification view: %+v", q.EndpointSets)
	return nil
}

func prepareElasticDeploy(t *testing.T) (*Orchestrator, registry.Store, context.Context, *registry.Job, int) {
	t.Helper()
	installFakeCelld(t)
	t.Setenv(contract.EnvElasticRuntime, "1")
	o, store, ctx := newTestOrch(t)
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	uri := artifact.ServerArtifactURI("cellp-artifacts", "demo", "v1")
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v1", ProjectID: "demo", ArtifactURI: uri, GitRef: "main", GitSHA: "abc",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone,
		ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	qualPort := freeRuntimeBasePort(t) + 11
	qualCmd := exec.Command("celld", "--listen", fmt.Sprintf("127.0.0.1:%d", qualPort))
	if err := qualCmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = qualCmd.Process.Kill() })
	time.Sleep(150 * time.Millisecond)
	var seeded sync.Once
	o.SetElasticSchedulerTick(func(ctx context.Context) error {
		seeded.Do(func() {
			seedElasticQualificationEndpoint(t, ctx, store, "demo", "v1", qualPort)
		})
		return nil
	})
	destDir := filepath.Join(o.cfg.ArtifactsDir, "demo", "v1")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err != nil {
		t.Fatal(err)
	}
	return o, store, ctx, j, qualPort
}

func TestElasticDeployReadySnapshotReadyOrdering(t *testing.T) {
	o, store, ctx, _, qualPort := prepareElasticDeploy(t)
	ack := &integrationSnapshotAck{t: t, wantDeployReady: true}
	o.SetRouteSnapshotAck(ack)

	before, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cj, err := store.ClaimJob(ctx, "elastic-worker", jobLease)
	if err != nil || cj == nil {
		t.Fatal(err)
	}
	if err := o.runDeploy(ctx, cj, "elastic-worker"); err != nil {
		t.Fatal(err)
	}
	if !ack.called {
		t.Fatal("snapshot acknowledgement was not called")
	}
	v, err := store.GetVersion(ctx, "demo", "v1")
	if err != nil || v == nil || v.Status != registry.StatusReady {
		t.Fatalf("version=%+v err=%v", v, err)
	}
	after, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Fatalf("route revision did not advance: before=%d after=%d", before, after)
	}
	if ack.observedRevision < before {
		t.Fatalf("qualification ack revision=%d before deploy baseline=%d", ack.observedRevision, before)
	}
	if after <= ack.observedRevision {
		t.Fatalf("ready transition should bump route revision after qual ack: observed=%d after=%d", ack.observedRevision, after)
	}
	route, err := store.GetRoute(ctx, "demo", "v1")
	if err != nil || route == nil || route.UpstreamPort != qualPort {
		t.Fatalf("elastic deploy must use scheduler qualification endpoint, not runtime.Start: route=%+v qualPort=%d", route, qualPort)
	}
}

func TestElasticSnapshotFailureFailsClosedAndCompensates(t *testing.T) {
	o, store, ctx, _, _ := prepareElasticDeploy(t)
	ack := &integrationSnapshotAck{t: t, wantDeployReady: true, fail: errors.New("snapshot rejected")}
	o.SetRouteSnapshotAck(ack)

	o.processOne(ctx, "test-worker")
	v, err := store.GetVersion(ctx, "demo", "v1")
	if err != nil || v == nil || v.Status != registry.StatusFailed {
		t.Fatalf("version=%+v err=%v", v, err)
	}
	route, err := store.GetRoute(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if route != nil && route.Active {
		t.Fatalf("failed deploy retained active route: %+v", route)
	}
	desire, err := store.GetServingDesire(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if desire == nil || desire.DesiredReplicas != 0 || desire.Reason != desireReasonDeployFailed {
		t.Fatalf("failed deploy should clear owned qualification desire: %+v", desire)
	}
}
