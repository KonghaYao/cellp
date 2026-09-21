package serve

import (
	"context"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

// operatorNoopGuard stands in for the singleton controller guard in tests.
type operatorNoopGuard struct{}

func (operatorNoopGuard) HoldsWriteLock(context.Context) error { return nil }

type recordingEnsure struct {
	calls    int
	onEnsure func()
	err      error
}

func (c *recordingEnsure) EnsureCapacity(context.Context, string, string, int) error {
	c.calls++
	if c.onEnsure != nil {
		c.onEnsure()
	}
	return c.err
}

func operatorFixture(t *testing.T) (*registry.SQLiteStore, string, string) {
	t.Helper()
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/operator.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	project, version := "demo-app", "v-cold"
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: project}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: version, ProjectID: project}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, project, version, registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 4, Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Hour),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/test/node/n1", Zone: "zone-a",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: project, VersionID: version, Revision: 1, MinReplicas: 0, MaxReplicas: 8,
		BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, project, version, 0, registry.ServingDesireRow{
		ProjectID: project, VersionID: version, DesiredReplicas: 0, Generation: 1, Reason: "idle",
	}); err != nil {
		t.Fatal(err)
	}
	return store, project, version
}

func seedReadyReplica(t *testing.T, store *registry.SQLiteStore, project, version string) {
	t.Helper()
	if err := seedReadyReplicaErr(store, project, version); err != nil {
		t.Fatal(err)
	}
}

// seedReadyReplicaErr places the replica the scheduler would place, at the desire
// generation the wake left behind.
func seedReadyReplicaErr(store *registry.SQLiteStore, project, version string) error {
	ctx := context.Background()
	valid := time.Now().UTC().Add(time.Hour)
	desire, err := store.GetServingDesire(ctx, project, version)
	if err != nil {
		return err
	}
	generation := int64(1)
	if desire != nil {
		generation = desire.Generation
	}
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: "r1", ProjectID: project, VersionID: version, NodeID: "n1",
		Generation: generation, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}); err != nil {
		return err
	}
	// Observations must carry the lease the row actually holds, as the agent reads it.
	stored, err := store.GetRuntimeReplica(ctx, "r1")
	if err != nil || stored == nil || stored.ValidUntil == nil {
		return err
	}
	lease := *stored.ValidUntil
	for _, state := range []contract.ReplicaState{contract.ReplicaStarting, contract.ReplicaReady} {
		obs := registry.ReplicaObservation{
			ReplicaID: "r1", ProjectID: project, VersionID: version, NodeID: "n1",
			Generation: generation, State: state, AssignmentValidUntil: &lease,
		}
		if state == contract.ReplicaReady {
			obs.ListenHost, obs.ListenPort = "127.0.0.1", 9101
			obs.EndpointState, obs.EndpointValidUntil = contract.EndpointReady, &lease
		}
		if err := store.RecordObservation(ctx, obs); err != nil {
			return err
		}
	}
	return nil
}

// A cold enrolled version is woken with the activator's ensure mechanism, used once, and
// released afterwards so scale-to-zero keeps applying.
func TestOperatorEnsurerWakesColdVersionAndReleases(t *testing.T) {
	ctx := context.Background()
	store, project, version := operatorFixture(t)
	// Use the production ensure client, with a replica appearing as the scheduler would
	// place it, so the pin path under test is the real one.
	ensure := &activator.RegistryEnsureClient{Store: store, Guard: operatorNoopGuard{}}
	e := newOperatorServingEnsurer(store, ensure)
	seedDone := make(chan error, 1)
	go func() {
		time.Sleep(5 * time.Millisecond)
		seedDone <- seedReadyReplicaErr(store, project, version)
	}()
	e.wait, e.poll = 5*time.Second, time.Millisecond
	release, err := e.Ensure(ctx, project, version)
	seedErr := <-seedDone
	if err != nil {
		t.Fatalf("ensure cold version: %v (seed: %v)", err, seedErr)
	}
	if seedErr != nil {
		t.Fatalf("seed replica: %v", seedErr)
	}
	if release == nil {
		t.Fatal("cold wake must return a release pin")
	}
	desire, err := store.GetServingDesire(ctx, project, version)
	if err != nil || desire == nil || desire.Reason != operatorEnsureReason {
		t.Fatalf("pinned desire: %+v err=%v", desire, err)
	}
	release()
	after, err := store.GetServingDesire(ctx, project, version)
	if err != nil || after == nil {
		t.Fatalf("desire after release: %+v err=%v", after, err)
	}
	if after.DesiredReplicas != 0 || after.Reason != operatorIdleReason {
		t.Fatalf("release did not restore scale-to-zero: %+v", after)
	}
}

// A version with a live replica is operated without any wake, and a version the activator
// raised for its own reason is not dropped by an operator release.
// A warm version is pinned too: a scale-to-zero decision taken while the command runs
// would retire the fleet it is talking to. Concurrent operators share that pin, so one
// finishing never drops the wake another is still using.
func TestOperatorEnsurerPinsWarmVersionAndSharesPin(t *testing.T) {
	ctx := context.Background()
	store, project, version := operatorFixture(t)
	seedReadyReplica(t, store, project, version)
	e := newOperatorServingEnsurer(store, &activator.RegistryEnsureClient{Store: store, Guard: operatorNoopGuard{}})
	first, err := e.Ensure(ctx, project, version)
	if err != nil || first == nil {
		t.Fatalf("warm ensure: release=%v err=%v", first != nil, err)
	}
	second, err := e.Ensure(ctx, project, version)
	if err != nil || second == nil {
		t.Fatalf("second ensure: release=%v err=%v", second != nil, err)
	}
	held, err := store.GetServingDesire(ctx, project, version)
	if err != nil || held == nil || held.DesiredReplicas < 1 {
		t.Fatalf("warm version was not pinned: %+v err=%v", held, err)
	}
	// One operator leaves: the other's pin must survive.
	first()
	still, err := store.GetServingDesire(ctx, project, version)
	if err != nil || still == nil || still.DesiredReplicas < 1 {
		t.Fatalf("concurrent operator released the shared pin: %+v err=%v", still, err)
	}
	second()
	after, err := store.GetServingDesire(ctx, project, version)
	if err != nil || after == nil {
		t.Fatalf("desire after release: %+v err=%v", after, err)
	}
	if after.DesiredReplicas != 0 || after.Reason != operatorIdleReason {
		t.Fatalf("last operator out did not restore scale-to-zero: %+v", after)
	}
}

// A pin the activator (or a promotion) holds is not the operator's to drop.
func TestOperatorEnsurerKeepsForeignPin(t *testing.T) {
	ctx := context.Background()
	store, project, version := operatorFixture(t)
	seedReadyReplica(t, store, project, version)
	if err := store.CompareAndSetDesired(ctx, project, version, 1, registry.ServingDesireRow{
		ProjectID: project, VersionID: version, DesiredReplicas: 1, Generation: 2, Reason: operatorEnsureReason,
	}); err != nil {
		t.Fatal(err)
	}
	e := newOperatorServingEnsurer(store, &activator.RegistryEnsureClient{Store: store, Guard: operatorNoopGuard{}})
	release, err := e.Ensure(ctx, project, version)
	if err != nil || release == nil {
		t.Fatalf("ensure: release=%v err=%v", release != nil, err)
	}
	release()
	held, err := store.GetServingDesire(ctx, project, version)
	if err != nil || held == nil {
		t.Fatalf("desire: %+v err=%v", held, err)
	}
	if held.DesiredReplicas != 1 || held.Reason != operatorEnsureReason {
		t.Fatalf("operator dropped a pin it did not create: %+v", held)
	}
}
