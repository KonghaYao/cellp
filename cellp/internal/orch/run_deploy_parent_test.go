package orch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestRunDeployChildD1AndKVBranchWithGateway(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(gw.Close)
	installFakeCelld(t)

	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "deploy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Config{GatewayURL: gw.URL, ArtifactsDir: dir, ArtifactsBucket: "cellp-artifacts"}
	rm := runtime.New(freeRuntimeBasePort(t), "", "us-east-1", "s3://cellp-celld", "k", "s")
	t.Cleanup(func() { _ = rm.StopAll(context.Background()) })
	o := New(store, job.NewSQLiteQueue(store), branch.New(dir+"/off", store),
		rm,
		&artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}, cfg)
	wireElasticDeployTestFixtures(t, o, store)
	ctx := t.Context()

	parent := "v-parent"
	pid := parent
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: parent, ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", parent, registry.StatusReady, nil)
	uri := artifact.ServerArtifactURI("cellp-artifacts", "demo", "v-child")
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v-child", ProjectID: "demo", ParentVersionID: &pid,
		ArtifactURI: uri, GitRef: "main", GitSHA: "abc",
	})

	parentDir := filepath.Join(dir, "demo", parent)
	childDir := filepath.Join(dir, "demo", "v-child")
	for _, d := range []string{parentDir, childDir} {
		_ = os.MkdirAll(d, 0o755)
	}
	wrangler := `{"name":"app","d1_databases":[{"binding":"DB","database_name":"guestbook","database_id":"db-id-parent"}],"kv_namespaces":[{"binding":"KV","id":"ns-1"}]}`
	_ = os.WriteFile(filepath.Join(parentDir, "wrangler.jsonc"), []byte(wrangler), 0o644)
	_ = os.WriteFile(filepath.Join(childDir, "wrangler.jsonc"), []byte(wrangler), 0o644)

	_, _ = store.EnqueueJob(ctx, "demo", "v-child", registry.StatusFetching)
	cj, err := store.ClaimJob(ctx, "w", jobLease)
	if err != nil || cj == nil {
		t.Fatal(err)
	}
	if err := o.runDeploy(ctx, cj, "w"); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v-child")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("child %+v", v)
	}
}

func TestRunDeployChildD1BranchWithoutCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	parent := "v-parent"
	pid := parent
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: parent, ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", parent, registry.StatusReady, nil)
	uri := artifact.ServerArtifactURI("cellp-artifacts", "demo", "v-child")
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: "v-child", ProjectID: "demo", ParentVersionID: &pid,
		ArtifactURI: uri, GitRef: "main", GitSHA: "abc",
	})

	parentDir := filepath.Join(o.cfg.ArtifactsDir, "demo", parent)
	childDir := filepath.Join(o.cfg.ArtifactsDir, "demo", "v-child")
	for _, dir := range []string{parentDir, childDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wrangler := `{"name":"app","d1_databases":[{"binding":"DB","database_name":"guestbook","database_id":"db-id-parent"}]}`
	if err := os.WriteFile(filepath.Join(parentDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _ = store.EnqueueJob(ctx, "demo", "v-child", registry.StatusFetching)
	cj, err := store.ClaimJob(ctx, "w", jobLease)
	if err != nil || cj == nil {
		t.Fatal(err)
	}
	if err := o.runDeploy(ctx, cj, "w"); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v-child")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("child %+v", v)
	}
}

// The binding branch steps fork through the child's own live fleet, so the deploy must
// wait for the child's replica before running them. The wait is on the assignment, not
// on the qualification endpoint view: that view only lists deploy_ready versions.
func TestWaitBranchChildReplicaRequiresLiveAssignment(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	var ticks int
	o.SetElasticSchedulerTick(func(context.Context) error {
		ticks++
		return nil
	})
	// The wait observes the tick and the assignment, and honors the caller's context
	// instead of sleeping out its budget.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := o.waitBranchChildReplica(cancelled, "demo", "v-child"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", err)
	}
	if ticks != 0 {
		t.Fatalf("cancelled wait drove the scheduler: ticks=%d", ticks)
	}
	// A ready, still-valid assignment means the child's celld wrote its node lease.
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-child", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v-child", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "branch-child-test",
	}); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().UTC().Add(time.Hour)
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: "r-child", ProjectID: "demo", VersionID: "v-child", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: "r-child", ProjectID: "demo", VersionID: "v-child", NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting, AssignmentValidUntil: &valid,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: "r-child", ProjectID: "demo", VersionID: "v-child", NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady, ListenHost: "127.0.0.1", ListenPort: 9101,
		EndpointState: contract.EndpointReady, AssignmentValidUntil: &valid, EndpointValidUntil: &valid,
	}); err != nil {
		t.Fatal(err)
	}
	if err := o.waitBranchChildReplica(ctx, "demo", "v-child"); err != nil {
		t.Fatalf("live assignment: %v", err)
	}
	if ticks == 0 {
		t.Fatal("branch child wait did not drive the scheduler tick")
	}
}

// An archived enrolled version comes back through the elastic track: the wake marks it
// ready and raises its serving desire so the scheduler starts a fresh replica. It must not
// start a legacy per-version instance, which in the single track would serve from a
// process the platform does not route and would write a route the scheduler never owns.
func TestWakeElasticVersionDoesNotStartLegacyInstance(t *testing.T) {
	installFakeCelld(t)
	ctx := t.Context()
	store, err := registry.Open(filepath.Join(t.TempDir(), "wake.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	dir := t.TempDir()
	cfg := config.Config{ArtifactsDir: dir, ArtifactsBucket: "cellp-artifacts"}
	rm := runtime.New(freeRuntimeBasePort(t), "", "us-east-1", "s3://cellp-celld", "k", "s")
	t.Cleanup(func() { _ = rm.StopAll(context.Background()) })
	o := New(store, job.NewSQLiteQueue(store), branch.New(dir+"/off", store),
		rm, &artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}, cfg)
	wireElasticDeployTestFixtures(t, o, store)

	project, version := "demo", "v-archived"
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: project}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: version, ProjectID: project}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, project, version, registry.StatusArchived, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: project, VersionID: version, Revision: 1, MinReplicas: 0, MaxReplicas: 8,
		BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	// The scheduler tick places the replica the wake is waiting for.
	o.SetElasticSchedulerTick(func(ctx context.Context) error {
		reps, err := store.ListRuntimeReplicas(ctx, project, version)
		if err != nil {
			return err
		}
		if len(reps) > 0 {
			return nil
		}
		valid := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
		if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
			ReplicaID: "r-wake", ProjectID: project, VersionID: version, NodeID: "n1",
			Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
		}); err != nil {
			return err
		}
		for _, state := range []contract.ReplicaState{contract.ReplicaStarting, contract.ReplicaReady} {
			obs := registry.ReplicaObservation{
				ReplicaID: "r-wake", ProjectID: project, VersionID: version, NodeID: "n1",
				Generation: 1, State: state, AssignmentValidUntil: &valid,
			}
			if state == contract.ReplicaReady {
				obs.ListenHost, obs.ListenPort = "127.0.0.1", 9101
				obs.EndpointState, obs.EndpointValidUntil = contract.EndpointReady, &valid
			}
			if err := store.RecordObservation(ctx, obs); err != nil {
				return err
			}
		}
		return nil
	})
	if err := o.Wake(ctx, project, version); err != nil {
		t.Fatalf("wake: %v", err)
	}
	v, err := store.GetVersion(ctx, project, version)
	if err != nil || v == nil || v.Status != registry.StatusReady {
		t.Fatalf("status after wake: %+v err=%v", v, err)
	}
	// Wake restores readiness, not residency: its pin is dropped again, so the version
	// scales to zero like any other ready version instead of holding a hidden permanent pin.
	desire, err := store.GetServingDesire(ctx, project, version)
	if err != nil || desire == nil {
		t.Fatalf("serving desire after wake: %+v err=%v", desire, err)
	}
	if desire.DesiredReplicas != 0 {
		t.Fatalf("wake left a permanent pin: %+v", desire)
	}
	route, err := store.GetRoute(ctx, project, version)
	if err != nil {
		t.Fatal(err)
	}
	if route != nil && route.Active {
		t.Fatalf("elastic wake wrote a legacy route: %+v", route)
	}
}
