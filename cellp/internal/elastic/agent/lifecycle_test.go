package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

type fakeLifecycleBackend struct {
	mu          sync.Mutex
	events      []string
	inventory   map[string]BackendReplica
	diagnoseErr error
	startErr    error
	probeErr    error
	drainErr    error
	stopErr     error
	listErr     error
	healthy     bool
	startGate   chan struct{}
	starts      int
	stops       int
}

func newFakeLifecycleBackend() *fakeLifecycleBackend {
	return &fakeLifecycleBackend{inventory: map[string]BackendReplica{}, healthy: true}
}

func (b *fakeLifecycleBackend) ExpectedReplicaBucket(scope contract.CommandScope) (string, error) {
	return "s3://cellp-celld/" + scope.ProjectID + "/" + scope.VersionID, nil
}

func (b *fakeLifecycleBackend) Diagnose(context.Context, contract.StartReplicaSpec) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, "diagnose")
	return b.diagnoseErr
}

func (b *fakeLifecycleBackend) Start(_ context.Context, spec contract.StartReplicaSpec) (string, int, error) {
	b.mu.Lock()
	b.events = append(b.events, "start")
	b.starts++
	gate := b.startGate
	err := b.startErr
	b.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if err != nil {
		return "", 0, err
	}
	item := BackendReplica{
		ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID,
		Host: "127.0.0.1", Port: 9101, Healthy: b.healthy,
	}
	b.mu.Lock()
	b.inventory[item.ReplicaID] = item
	b.mu.Unlock()
	return item.Host, item.Port, nil
}

func (b *fakeLifecycleBackend) Probe(_ context.Context, scope contract.CommandScope) (BackendReplica, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, "probe")
	if b.probeErr != nil {
		return BackendReplica{}, b.probeErr
	}
	item, ok := b.inventory[scope.ReplicaID]
	if !ok {
		return BackendReplica{}, errors.New("not running")
	}
	item.Healthy = b.healthy
	return item, nil
}

func (b *fakeLifecycleBackend) Drain(_ context.Context, scope contract.CommandScope, deadline time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, "drain")
	if b.drainErr != nil {
		return b.drainErr
	}
	delete(b.inventory, scope.ReplicaID)
	b.stops++
	return nil
}

func (b *fakeLifecycleBackend) Stop(_ context.Context, scope contract.CommandScope) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, "stop")
	if b.stopErr != nil {
		return b.stopErr
	}
	delete(b.inventory, scope.ReplicaID)
	b.stops++
	return nil
}

func (b *fakeLifecycleBackend) List(context.Context) ([]BackendReplica, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listErr != nil {
		return nil, b.listErr
	}
	out := make([]BackendReplica, 0, len(b.inventory))
	for _, item := range b.inventory {
		out = append(out, item)
	}
	return out, nil
}

func setupLifecycle(t *testing.T) (*registry.SQLiteStore, contract.CommandScope) {
	return setupLifecycleWithOptions(t, registry.OpenOptions{})
}

func setupLifecycleWithOptions(t *testing.T, opts registry.OpenOptions) (*registry.SQLiteStore, contract.CommandScope) {
	t.Helper()
	ctx := context.Background()
	store, err := registry.OpenWithOptions(t.TempDir()+"/agent-lifecycle.sqlite", opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{NodeID: "n1", CapacityUnits: 2, Generation: 1, LeaseExpiry: exp}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: exp,
	}); err != nil {
		t.Fatal(err)
	}
	return store, contract.CommandScope{
		NodeID: "n1", ProjectID: "demo", VersionID: "v1", ReplicaID: "r1",
		Generation: 1, LeaseExpiry: exp, Nonce: "nonce", Action: contract.ActionStartReplica,
	}
}

func TestReconcileFailedReplicaWithLiveProcessPromotesReady(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	if _, err := h.StartReplica(ctx, spec, "start-1"); err != nil {
		t.Fatal(err)
	}
	backend.healthy = false
	probe := scope
	probe.Action = contract.ActionProbeReplica
	if _, err := h.ProbeReplica(ctx, probe); err != nil {
		t.Fatal(err)
	}
	backend.healthy = true
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	rep, err := store.ValidateAgentAssignment(ctx, scope, time.Now().UTC())
	if err != nil || rep == nil || rep.State != contract.ReplicaReady {
		t.Fatalf("after reconcile: rep=%+v err=%v", rep, err)
	}
}

func TestReconcileRestartFromFailedRecordsStartingBeforeReady(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	if _, err := h.StartReplica(ctx, spec, "start-1"); err != nil {
		t.Fatal(err)
	}
	backend.healthy = false
	probe := scope
	probe.Action = contract.ActionProbeReplica
	if _, err := h.ProbeReplica(ctx, probe); err != nil {
		t.Fatal(err)
	}
	delete(backend.inventory, scope.ReplicaID)
	backend.healthy = true
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	rep, err := store.ValidateAgentAssignment(ctx, scope, time.Now().UTC())
	if err != nil || rep == nil || rep.State != contract.ReplicaReady {
		t.Fatalf("after reconcile restart: rep=%+v err=%v", rep, err)
	}
}

func TestLifecycleStartDiagnoseReadyAndDurableReplay(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	got, err := h.StartReplica(ctx, spec, "command-1")
	if err != nil || got.State != contract.ReplicaReady {
		t.Fatalf("start: %+v err=%v", got, err)
	}
	if len(backend.events) < 2 || backend.events[0] != "diagnose" || backend.events[1] != "start" {
		t.Fatalf("order: %v", backend.events)
	}
	h2 := NewLifecycleFromRegistry(true, store, backend)
	got, err = h2.StartReplica(ctx, spec, "command-1")
	if err != nil || got.State != contract.ReplicaReady || backend.starts != 1 {
		t.Fatalf("replay: %+v err=%v starts=%d", got, err, backend.starts)
	}
	conflict := spec
	conflict.Scope.ReplicaID = "other"
	if _, err := h2.StartReplica(ctx, conflict, "command-1"); err == nil {
		t.Fatal("expected idempotency scope conflict")
	}
}

func TestLifecycleStartRequiresAssignmentAndCompensates(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	missing := scope
	missing.ReplicaID = "missing"
	if _, err := h.StartReplica(ctx, contract.StartReplicaSpec{Scope: missing, Bucket: "s3://cellp-celld/demo/v1"}, "missing-key"); !errors.Is(err, errReplicaNotFound) {
		t.Fatalf("missing assignment: %v", err)
	}
	backend.startErr = errors.New("start failed")
	if _, err := h.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "fail-key"); err == nil {
		t.Fatal("expected failure")
	}
	rep, _ := store.GetRuntimeReplica(ctx, "r1")
	if rep.State != contract.ReplicaFailed || backend.stops != 1 {
		t.Fatalf("compensation: rep=%+v stops=%d", rep, backend.stops)
	}
}

func TestLifecycleReadyReplicaNewCommandDoesNotRestart(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	if _, err := h.StartReplica(ctx, spec, "first-key"); err != nil {
		t.Fatal(err)
	}
	got, err := NewLifecycleFromRegistry(true, store, backend).StartReplica(ctx, spec, "recovery-key")
	if err != nil || got.State != contract.ReplicaReady {
		t.Fatalf("recover ready: %+v err=%v", got, err)
	}
	if backend.starts != 1 || backend.stops != 0 {
		t.Fatalf("healthy replica was restarted or stopped: starts=%d stops=%d", backend.starts, backend.stops)
	}
}

type faultLifecycleStore struct {
	LifecycleStore
	recordState           contract.ReplicaState
	recordErr             error
	atomicErr             error
	completeErr           error
	validateAssignmentErr error
}

func (s faultLifecycleStore) ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error) {
	if s.validateAssignmentErr != nil {
		return nil, s.validateAssignmentErr
	}
	return s.LifecycleStore.ValidateAgentAssignment(ctx, scope, now)
}

func (s faultLifecycleStore) RecordObservation(ctx context.Context, obs registry.ReplicaObservation) error {
	if obs.State == s.recordState && s.recordErr != nil {
		return s.recordErr
	}
	return s.LifecycleStore.RecordObservation(ctx, obs)
}

func (s faultLifecycleStore) RecordObservationAndCompleteAgentCommand(ctx context.Context, obs registry.ReplicaObservation, command registry.AgentCommand) error {
	if s.atomicErr != nil {
		return s.atomicErr
	}
	return s.LifecycleStore.RecordObservationAndCompleteAgentCommand(ctx, obs, command)
}

func (s faultLifecycleStore) CompleteAgentCommand(ctx context.Context, command registry.AgentCommand) error {
	if s.completeErr != nil {
		return s.completeErr
	}
	return s.LifecycleStore.CompleteAgentCommand(ctx, command)
}

func (s faultLifecycleStore) TerminalizeReplica(ctx context.Context, replicaID, nodeID string, generation int64, state contract.ReplicaState) error {
	if state == s.recordState && s.recordErr != nil {
		return s.recordErr
	}
	return s.LifecycleStore.TerminalizeReplica(ctx, replicaID, nodeID, generation, state)
}

func TestLifecyclePersistenceFailuresAreReturned(t *testing.T) {
	injected := errors.New("injected persistence failure")
	t.Run("starting", func(t *testing.T) {
		store, scope := setupLifecycle(t)
		backend := newFakeLifecycleBackend()
		adapter := RegistryStores{Store: store}
		h := NewLifecycleHandler(true, adapter, faultLifecycleStore{LifecycleStore: adapter, recordState: contract.ReplicaStarting, recordErr: injected}, backend)
		if _, err := h.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "starting-fault"); !errors.Is(err, injected) {
			t.Fatalf("starting persistence error lost: %v", err)
		}
	})
	t.Run("ready atomic completion", func(t *testing.T) {
		store, scope := setupLifecycle(t)
		backend := newFakeLifecycleBackend()
		adapter := RegistryStores{Store: store}
		h := NewLifecycleHandler(true, adapter, faultLifecycleStore{LifecycleStore: adapter, atomicErr: injected}, backend)
		if _, err := h.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "ready-fault"); !errors.Is(err, injected) {
			t.Fatalf("ready persistence error lost: %v", err)
		}
		snap, err := store.BuildLegacyRouteSnapshot(context.Background())
		if err != nil || len(snap.EndpointSets) != 0 {
			t.Fatalf("failed start left route: %+v err=%v", snap.EndpointSets, err)
		}
	})
	t.Run("failed and command complete", func(t *testing.T) {
		store, scope := setupLifecycle(t)
		backend := newFakeLifecycleBackend()
		backend.diagnoseErr = errors.New("diagnose failed")
		adapter := RegistryStores{Store: store}
		h := NewLifecycleHandler(true, adapter, faultLifecycleStore{
			LifecycleStore: adapter, recordState: contract.ReplicaFailed, recordErr: injected, completeErr: injected,
		}, backend)
		if _, err := h.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "failed-fault"); !errors.Is(err, injected) {
			t.Fatalf("compensation persistence errors lost: %v", err)
		}
	})
	t.Run("stopped", func(t *testing.T) {
		store, scope := setupLifecycle(t)
		backend := newFakeLifecycleBackend()
		base := NewLifecycleFromRegistry(true, store, backend)
		if _, err := base.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "stop-setup"); err != nil {
			t.Fatal(err)
		}
		adapter := RegistryStores{Store: store}
		h := NewLifecycleHandler(true, adapter, faultLifecycleStore{LifecycleStore: adapter, recordState: contract.ReplicaStopped, recordErr: injected}, backend)
		scope.Action = contract.ActionStopReplica
		if _, err := h.StopReplica(context.Background(), scope); !errors.Is(err, injected) {
			t.Fatalf("stopped persistence error lost: %v", err)
		}
	})
}

func TestLifecycleProbeReturnsFailedObservationError(t *testing.T) {
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	base := NewLifecycleFromRegistry(true, store, backend)
	if _, err := base.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "probe-setup"); err != nil {
		t.Fatal(err)
	}
	backend.healthy = false
	injected := errors.New("failed observation rejected")
	adapter := RegistryStores{Store: store}
	h := NewLifecycleHandler(true, adapter, faultLifecycleStore{LifecycleStore: adapter, recordState: contract.ReplicaFailed, recordErr: injected}, backend)
	scope.Action = contract.ActionProbeReplica
	if _, err := h.ProbeReplica(context.Background(), scope); !errors.Is(err, injected) {
		t.Fatalf("probe swallowed persistence error: %v", err)
	}
}

type renewalFaultStore struct {
	LifecycleStore
	fail chan struct{}
}

func (s renewalFaultStore) RenewAgentCommandLease(ctx context.Context, command registry.AgentCommand, expiry time.Time) error {
	select {
	case <-s.fail:
		return registry.ErrAgentCommandConflict
	default:
		return s.LifecycleStore.RenewAgentCommandLease(ctx, command, expiry)
	}
}

func TestLifecycleLeaseLossDoesNotCompensatingStop(t *testing.T) {
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	backend.startGate = make(chan struct{})
	adapter := RegistryStores{Store: store}
	fail := make(chan struct{})
	h := NewLifecycleHandler(true, adapter, renewalFaultStore{LifecycleStore: adapter, fail: fail}, backend)
	h.commandLease = 30 * time.Millisecond
	h.leaseTick = 5 * time.Millisecond
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	done := make(chan error, 1)
	go func() { _, err := h.StartReplica(context.Background(), spec, "lease-loss"); done <- err }()
	for {
		backend.mu.Lock()
		started := backend.starts == 1
		backend.mu.Unlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(fail)
	time.Sleep(10 * time.Millisecond)
	close(backend.startGate)
	if err := <-done; err == nil {
		t.Fatal("ownership loss must fail closed")
	}
	if err := h.ReconcileNode(context.Background(), scope.NodeID); err != nil {
		t.Fatalf("reconcile after transient renewal failure: %v", err)
	}
	rep, err := store.GetRuntimeReplica(context.Background(), scope.ReplicaID)
	if err != nil || rep.State != contract.ReplicaReady {
		t.Fatalf("reconcile did not converge: rep=%+v err=%v", rep, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.stops != 0 {
		t.Fatalf("stale attempt destructively stopped runtime: %d", backend.stops)
	}
}

func TestLifecycleBucketAuthorization(t *testing.T) {
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	for _, bucket := range []string{"s3://cellp-celld/demo/v10", "s3://cellp-celld/demo/v1?x=1", "s3://cellp-celld/demo/v1/../other"} {
		if _, err := h.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: scope, Bucket: bucket}, "bucket-"+bucket); err == nil {
			t.Fatalf("accepted unauthorized bucket %q", bucket)
		}
	}
	if backend.starts != 0 {
		t.Fatalf("runtime touched before bucket validation: starts=%d", backend.starts)
	}
}

type overlapBackend struct {
	*fakeLifecycleBackend
	firstStarted chan struct{}
	once         sync.Once
}

func (b *overlapBackend) Start(ctx context.Context, spec contract.StartReplicaSpec) (string, int, error) {
	first := false
	b.once.Do(func() {
		first = true
		close(b.firstStarted)
	})
	if first {
		b.mu.Lock()
		b.events = append(b.events, "start")
		b.starts++
		b.mu.Unlock()
		<-ctx.Done()
		return "", 0, ctx.Err()
	}
	return b.fakeLifecycleBackend.Start(ctx, spec)
}

type countedRenewalFaultStore struct {
	LifecycleStore
	fail  <-chan struct{}
	mu    sync.Mutex
	calls int
}

func (s *countedRenewalFaultStore) RenewAgentCommandLease(ctx context.Context, command registry.AgentCommand, expiry time.Time) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	select {
	case <-s.fail:
		return registry.ErrAgentCommandConflict
	default:
		return s.LifecycleStore.RenewAgentCommandLease(ctx, command, expiry)
	}
}

func (s *countedRenewalFaultStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestLifecycleTwoHandlerLeaseTakeoverFencesOldAttempt(t *testing.T) {
	store, scope := setupLifecycleWithOptions(t, registry.OpenOptions{AgentCommandLease: 20 * time.Millisecond})
	if err := store.UpdateVersionStatus(context.Background(), "demo", "v1", registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(context.Background(), registry.ServingPolicyRow{ProjectID: "demo", VersionID: "v1", Revision: 1, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true}); err != nil {
		t.Fatal(err)
	}
	base := newFakeLifecycleBackend()
	backend := &overlapBackend{fakeLifecycleBackend: base, firstStarted: make(chan struct{})}
	adapter := RegistryStores{Store: store}
	lose := make(chan struct{})
	firstStore := &countedRenewalFaultStore{LifecycleStore: adapter, fail: lose}
	h1 := NewLifecycleHandler(true, adapter, firstStore, backend)
	h1.commandLease, h1.leaseTick = 20*time.Millisecond, 2*time.Millisecond
	h2 := NewLifecycleFromRegistry(true, store, backend)
	h2.commandLease, h2.leaseTick = 20*time.Millisecond, 2*time.Millisecond
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	firstDone := make(chan error, 1)
	go func() { _, err := h1.StartReplica(context.Background(), spec, "overlap-key"); firstDone <- err }()
	<-backend.firstStarted
	close(lose)
	if err := <-firstDone; err == nil {
		t.Fatal("old attempt did not lose ownership")
	}
	callsAtReturn := firstStore.count()
	time.Sleep(25 * time.Millisecond)
	got, err := h2.StartReplica(context.Background(), spec, "overlap-key")
	if err != nil || got.State != contract.ReplicaReady {
		t.Fatalf("takeover: rep=%+v err=%v", got, err)
	}
	replayed, err := NewLifecycleFromRegistry(true, store, backend).StartReplica(context.Background(), spec, "overlap-key")
	if err != nil || replayed.State != contract.ReplicaReady {
		t.Fatalf("terminal command replay: rep=%+v err=%v", replayed, err)
	}
	snap, err := store.BuildLegacyRouteSnapshot(context.Background())
	if err != nil || len(snap.EndpointSets) != 1 || len(snap.EndpointSets[0].Endpoints) != 1 {
		t.Fatalf("single ready endpoint: %+v err=%v", snap.EndpointSets, err)
	}
	time.Sleep(5 * time.Millisecond)
	if firstStore.count() != callsAtReturn {
		t.Fatalf("renew goroutine survived Start return: %d -> %d", callsAtReturn, firstStore.count())
	}
	base.mu.Lock()
	defer base.mu.Unlock()
	if base.starts != 2 || base.stops != 0 || len(base.inventory) != 1 {
		t.Fatalf("runtime identity: starts=%d stops=%d inventory=%+v", base.starts, base.stops, base.inventory)
	}
}

func TestLifecycleConcurrentIdempotencyStartsOnce(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	backend.startGate = make(chan struct{})
	h1 := NewLifecycleFromRegistry(true, store, backend)
	h2 := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	done := make(chan error, 1)
	go func() { _, err := h1.StartReplica(ctx, spec, "race-key"); done <- err }()
	for {
		backend.mu.Lock()
		starts := backend.starts
		backend.mu.Unlock()
		if starts == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := h2.StartReplica(ctx, spec, "race-key"); err == nil {
		t.Fatal("in-progress claim must fail closed")
	}
	close(backend.startGate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if backend.starts != 1 {
		t.Fatalf("starts=%d", backend.starts)
	}
}

func TestStopProbeErrorStillStopsAndRetries(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	if _, err := h.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "probe-stop-start"); err != nil {
		t.Fatal(err)
	}
	backend.probeErr = errors.New("probe transient")
	backend.stopErr = errors.New("stop failed")
	scope.Action = contract.ActionStopReplica
	if _, err := h.StopReplica(ctx, scope); err == nil {
		t.Fatal("expected joined probe/stop failure")
	}
	if backend.stops != 0 {
		t.Fatalf("failed stop counted as success: %d", backend.stops)
	}
	rep, _ := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if rep.State != contract.ReplicaDraining {
		t.Fatalf("failed stop became %s", rep.State)
	}
	backend.stopErr = nil
	got, err := h.StopReplica(ctx, scope)
	if err != nil || got.State != contract.ReplicaStopped || backend.stops != 1 {
		t.Fatalf("retry: rep=%+v stops=%d err=%v", got, backend.stops, err)
	}
}

func TestExpiredAndCordonedAssignmentsAllowOnlyExactCleanup(t *testing.T) {
	for _, action := range []contract.LifecycleAction{contract.ActionStopReplica, contract.ActionDrainReplica} {
		t.Run(string(action), func(t *testing.T) {
			ctx := context.Background()
			store, scope := setupLifecycle(t)
			backend := newFakeLifecycleBackend()
			h := NewLifecycleFromRegistry(true, store, backend)
			if _, err := h.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "cleanup-start-"+string(action)); err != nil {
				t.Fatal(err)
			}
			if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{NodeID: scope.NodeID, CapacityUnits: 1, Generation: 1, Cordoned: true, LeaseExpiry: time.Now().UTC().Add(-time.Second)}); err != nil {
				t.Fatal(err)
			}
			scope.Action = action
			scope.LeaseExpiry = time.Now().UTC().Add(-time.Second)
			var got contract.RuntimeReplica
			var err error
			if action == contract.ActionStopReplica {
				got, err = h.StopReplica(ctx, scope)
			} else {
				got, err = h.DrainReplica(ctx, scope, time.Now().UTC())
			}
			if err != nil || got.State != contract.ReplicaStopped {
				t.Fatalf("cleanup failed: rep=%+v err=%v", got, err)
			}
		})
	}

	store, scope := setupLifecycle(t)
	h := NewLifecycleFromRegistry(true, store, newFakeLifecycleBackend())
	scope.Action = contract.ActionStopReplica
	for name, mutate := range map[string]func(*contract.CommandScope){
		"generation": func(s *contract.CommandScope) { s.Generation++ },
		"node":       func(s *contract.CommandScope) { s.NodeID = "other" },
		"project":    func(s *contract.CommandScope) { s.ProjectID = "other" },
		"version":    func(s *contract.CommandScope) { s.VersionID = "other" },
	} {
		t.Run("reject-"+name, func(t *testing.T) {
			bad := scope
			mutate(&bad)
			if _, err := h.StopReplica(context.Background(), bad); err == nil {
				t.Fatal("stale cleanup accepted")
			}
		})
	}
}

func TestLifecycleUnsafeStorageIDsRejectedBeforeDiagnose(t *testing.T) {
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	for _, value := range []string{"a/b+c", "a+b/c", ".", "..", "%2F", "?", "#", "雪", "/prefix", "trailing/", `back\\slash`} {
		bad := scope
		bad.ProjectID = value
		if _, err := h.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: bad, Bucket: "s3://cellp-celld/x/v1"}, "unsafe-"+value); err == nil {
			t.Fatalf("accepted unsafe ID %q", value)
		}
	}
	if len(backend.events) != 0 {
		t.Fatalf("backend reached before validation: %v", backend.events)
	}
}

func TestReconcileDrainingStoppedProcessConvergesIdempotently(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	if _, err := h.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "draining-idempotent-start"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
		NodeID: scope.NodeID, Generation: scope.Generation, State: contract.ReplicaDraining,
		ListenHost: "127.0.0.1", ListenPort: 9901, EndpointState: contract.EndpointDraining,
		AssignmentValidUntil: &scope.LeaseExpiry, EndpointValidUntil: &scope.LeaseExpiry,
	}); err != nil {
		t.Fatal(err)
	}
	delete(backend.inventory, scope.ReplicaID)
	backend.starts = 0
	backend.stops = 0
	backend.events = nil
	for i := 0; i < 2; i++ {
		if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
			t.Fatal(err)
		}
		rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
		if err != nil || rep.State != contract.ReplicaStopped {
			t.Fatalf("reconcile %d state=%+v err=%v", i, rep, err)
		}
	}
	if backend.starts != 0 || backend.stops != 0 {
		t.Fatalf("terminal reconcile touched backend: starts=%d stops=%d", backend.starts, backend.stops)
	}
}

func TestStopFailureRemainsDrainingAndReconcileRetries(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	if _, err := h.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "stop-retry-start"); err != nil {
		t.Fatal(err)
	}
	backend.stopErr = errors.New("stop failed")
	scope.Action = contract.ActionStopReplica
	if _, err := h.StopReplica(ctx, scope); err == nil {
		t.Fatal("expected stop failure")
	}
	rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil || rep.State != contract.ReplicaDraining {
		t.Fatalf("stop failure state=%+v err=%v", rep, err)
	}
	backend.stopErr = nil
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		t.Fatal(err)
	}
	rep, err = store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil || rep.State != contract.ReplicaStopped {
		t.Fatalf("retry state=%+v err=%v", rep, err)
	}
}

type unavailableNodeStore struct {
	NodeStore
	node *contract.RuntimeNode
	err  error
}

func (s unavailableNodeStore) GetRuntimeNode(context.Context, string) (*contract.RuntimeNode, error) {
	return s.node, s.err
}

func TestReconcileObservationStaleStopsLocalProcess(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	base := NewLifecycleFromRegistry(true, store, backend)
	if _, err := base.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "stale-setup"); err != nil {
		t.Fatal(err)
	}
	adapter := RegistryStores{Store: store}
	h := NewLifecycleHandler(true, adapter, faultLifecycleStore{LifecycleStore: adapter, validateAssignmentErr: registry.ErrObservationStale}, backend)
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.stops == 0 {
		t.Fatalf("expected local stop on stale observation, events=%v", backend.events)
	}
	if _, ok := backend.inventory[scope.ReplicaID]; ok {
		t.Fatal("stale replica must be removed from local inventory")
	}
}

func TestReconcileNodeRegistryReadErrorFailsClosed(t *testing.T) {
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	backend.inventory[scope.ReplicaID] = BackendReplica{ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID, Healthy: true}
	readErr := errors.New("registry unavailable")
	adapter := RegistryStores{Store: store}
	h := NewLifecycleHandler(true, unavailableNodeStore{NodeStore: adapter, err: readErr}, adapter, backend)
	err := h.ReconcileNode(context.Background(), scope.NodeID)
	if err == nil || !errors.Is(err, readErr) {
		t.Fatalf("expected read error, got %v", err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	for _, event := range backend.events {
		if event == "stop" {
			t.Fatalf("must not stop inventory on read error, events=%v", backend.events)
		}
	}
}

func TestReconcileNodeAssignmentReadErrorPreservesRunningReplica(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	base := NewLifecycleFromRegistry(true, store, backend)
	if _, err := base.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}, "assignment-read-setup"); err != nil {
		t.Fatal(err)
	}
	readErr := errors.New("assignment registry unavailable")
	adapter := RegistryStores{Store: store}
	h := NewLifecycleHandler(true, adapter, faultLifecycleStore{LifecycleStore: adapter, validateAssignmentErr: readErr}, backend)
	if err := h.ReconcileNode(ctx, scope.NodeID); !errors.Is(err, readErr) {
		t.Fatalf("expected assignment read error, got %v", err)
	}
	rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil || rep.State != contract.ReplicaReady {
		t.Fatalf("running replica was terminalized: rep=%+v err=%v", rep, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.stops != 0 {
		t.Fatalf("running replica was stopped on unknown assignment state: stops=%d events=%v", backend.stops, backend.events)
	}
	if _, ok := backend.inventory[scope.ReplicaID]; !ok {
		t.Fatal("running replica disappeared on unknown assignment state")
	}
}

func TestReconcileUnavailableNodeStillCleansInventory(t *testing.T) {
	for _, tc := range []struct {
		name string
		node *contract.RuntimeNode
		err  error
	}{
		{name: "missing"},
		{name: "cordoned", node: &contract.RuntimeNode{NodeID: "n1", Generation: 1, Cordoned: true, LeaseExpiry: time.Now().UTC().Add(time.Hour)}},
		{name: "expired", node: &contract.RuntimeNode{NodeID: "n1", Generation: 1, LeaseExpiry: time.Now().UTC().Add(-time.Second)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, scope := setupLifecycle(t)
			backend := newFakeLifecycleBackend()
			backend.inventory[scope.ReplicaID] = BackendReplica{ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID, Healthy: true}
			backend.stopErr = errors.New("stop failed")
			adapter := RegistryStores{Store: store}
			h := NewLifecycleHandler(true, unavailableNodeStore{NodeStore: adapter, node: tc.node, err: tc.err}, adapter, backend)
			err := h.ReconcileNode(context.Background(), scope.NodeID)
			backend.mu.Lock()
			defer backend.mu.Unlock()
			found := false
			for _, event := range backend.events {
				found = found || event == "stop"
			}
			if err == nil || !found {
				t.Fatalf("cleanup err=%v events=%v", err, backend.events)
			}
		})
	}
}

func TestLifecycleProbeDrainStopAndReconcile(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	if _, err := h.StartReplica(ctx, spec, "start-key"); err != nil {
		t.Fatal(err)
	}
	backend.healthy = false
	probe := scope
	probe.Action = contract.ActionProbeReplica
	result, err := h.ProbeReplica(ctx, probe)
	if err != nil || result.State != contract.ReplicaFailed {
		t.Fatalf("probe down: %+v err=%v", result, err)
	}

	// A fresh assignment proves restart reconciliation without using legacy ReconcileFleet.
	store2, scope2 := setupLifecycle(t)
	backend2 := newFakeLifecycleBackend()
	h2 := NewLifecycleFromRegistry(true, store2, backend2)
	if err := h2.ReconcileNode(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	if backend2.starts != 1 {
		t.Fatalf("reconcile starts=%d", backend2.starts)
	}
	drain := scope2
	drain.Action = contract.ActionDrainReplica
	drained, err := h2.DrainReplica(ctx, drain, time.Now().Add(-time.Second))
	if err != nil || drained.State != contract.ReplicaStopped || backend2.stops != 1 {
		t.Fatalf("deadline drain: %+v err=%v stops=%d", drained, err, backend2.stops)
	}
	stop := scope2
	stop.Action = contract.ActionStopReplica
	if _, err := h2.StopReplica(ctx, stop); err != nil || backend2.stops != 1 {
		t.Fatalf("terminal stop replay: err=%v stops=%d", err, backend2.stops)
	}
}
