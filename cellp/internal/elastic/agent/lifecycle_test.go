package agent

import (
	"context"
	"errors"
	"fmt"
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

func TestReconcileNodeKeepsBootingReplicaProcess(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	// A booting replica: the process exists and is not healthy yet. celld withholds
	// {"ok":true} until its ready gate settles, so reconcile must wait for the process
	// instead of killing it: the kill leaves a lingering node lease in the version
	// bucket and the replacement then waits that lease out (replica churn).
	backend.inventory[scope.ReplicaID] = BackendReplica{
		ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
		Host: "127.0.0.1", Port: 9101, Alive: true, Healthy: false,
	}
	h := NewLifecycleFromRegistry(true, store, backend)
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		t.Fatalf("reconcile booting replica: %v", err)
	}
	backend.mu.Lock()
	stops, starts, alive := backend.stops, backend.starts, true
	if _, ok := backend.inventory[scope.ReplicaID]; !ok {
		alive = false
	}
	events := append([]string(nil), backend.events...)
	backend.mu.Unlock()
	if stops != 0 || !alive {
		t.Fatalf("booting replica process was killed: stops=%d alive=%v events=%v", stops, alive, events)
	}
	if starts != 0 {
		t.Fatalf("booting replica was restarted: starts=%d events=%v", starts, events)
	}
	rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.State == contract.ReplicaReady {
		t.Fatalf("unhealthy booting replica published ready: %+v", rep)
	}
	// Once the gate opens the same process is published ready without a restart.
	backend.mu.Lock()
	healthyItem := backend.inventory[scope.ReplicaID]
	healthyItem.Healthy = true
	backend.inventory[scope.ReplicaID] = healthyItem
	backend.mu.Unlock()
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		t.Fatalf("reconcile after gate open: %v", err)
	}
	rep, err = store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil || rep.State != contract.ReplicaReady {
		t.Fatalf("gate open did not publish ready: rep=%+v err=%v", rep, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.stops != 0 || backend.starts != 0 {
		t.Fatalf("booting process churned: stops=%d starts=%d events=%v", backend.stops, backend.starts, backend.events)
	}
}

func TestReconcileNodeDoesNotKeepTerminalOrDeadProcesses(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	if _, err := h.StartReplica(ctx, spec, "terminal-key"); err != nil {
		t.Fatal(err)
	}
	// A serving replica that lost health is an authoritative terminal decision upstream:
	// withdraw the endpoint and stop the process, keeping slot and lease consistent.
	backend.healthy = false
	backend.mu.Lock()
	item := backend.inventory[scope.ReplicaID]
	item.Healthy = false
	item.Alive = true
	backend.inventory[scope.ReplicaID] = item
	backend.mu.Unlock()
	// The pass withdraws the endpoint, stops the process and retries the start; the
	// retry fails here because the backend stays unhealthy. The assertions below are
	// about the fail-closed decision, not about that retry succeeding.
	_ = h.ReconcileNode(ctx, scope.NodeID)
	rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.State == contract.ReplicaReady {
		t.Fatalf("unhealthy serving replica kept its endpoint: %+v", rep)
	}
	backend.mu.Lock()
	stops := backend.stops
	backend.mu.Unlock()
	if stops == 0 {
		t.Fatal("terminal serving replica kept a live process")
	}
	// An inventory record for a process that already exited must not be mistaken for a
	// running replica and must not keep the assignment alive.
	store2, scope2 := setupLifecycle(t)
	backend2 := newFakeLifecycleBackend()
	backend2.inventory[scope2.ReplicaID] = BackendReplica{
		ReplicaID: scope2.ReplicaID, ProjectID: scope2.ProjectID, VersionID: scope2.VersionID,
		Host: "127.0.0.1", Port: 9102, Alive: false, Healthy: true,
	}
	// The record claims health but its process is gone; with no process to start the
	// replica must not be published ready from that record alone.
	backend2.startErr = errors.New("no process to start")
	h2 := NewLifecycleFromRegistry(true, store2, backend2)
	_ = h2.ReconcileNode(ctx, scope2.NodeID)
	rep2, err := store2.GetRuntimeReplica(ctx, scope2.ReplicaID)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.State == contract.ReplicaReady {
		t.Fatalf("dead process record published ready: %+v", rep2)
	}
	backend2.mu.Lock()
	starts := backend2.starts
	backend2.mu.Unlock()
	if starts == 0 {
		t.Fatal("dead process record left the assignment without a start attempt")
	}
}

// movedLeaseOnceStore fails the first ready commit the way a concurrent renewal does:
// the stored lease no longer matches the one the attempt was claimed with.
type movedLeaseOnceStore struct {
	LifecycleStore
	mu     sync.Mutex
	fails  int
	failed int
}

func (s *movedLeaseOnceStore) RecordObservationAndCompleteAgentCommand(ctx context.Context, obs registry.ReplicaObservation, command registry.AgentCommand) error {
	s.mu.Lock()
	s.fails++
	first := s.fails == 1
	if first {
		s.failed++
	}
	s.mu.Unlock()
	if first {
		return registry.ErrObservationStale
	}
	return s.LifecycleStore.RecordObservationAndCompleteAgentCommand(ctx, obs, command)
}

func TestStartReplicaCommitRetriesMovedLeaseWithoutStopping(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	adapter := RegistryStores{Store: store}
	faults := &movedLeaseOnceStore{LifecycleStore: adapter}
	h := NewLifecycleHandler(true, adapter, faults, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	rep, err := h.StartReplica(ctx, spec, "moved-lease-key")
	if err != nil {
		t.Fatalf("start with a moved lease: %v", err)
	}
	if rep.State != contract.ReplicaReady {
		t.Fatalf("ready commit lost the moved lease: %+v", rep)
	}
	if faults.failed == 0 {
		t.Fatal("test did not exercise the moved-lease commit")
	}
	backend.mu.Lock()
	stops := backend.stops
	_, alive := backend.inventory[scope.ReplicaID]
	backend.mu.Unlock()
	if stops != 0 || !alive {
		t.Fatalf("moved lease stopped a serving process: stops=%d alive=%v", stops, alive)
	}
	stored, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil || stored.State != contract.ReplicaReady {
		t.Fatalf("ready not committed after retry: rep=%+v err=%v", stored, err)
	}
}

func TestReconcileNodeExpiredLocalLeaseCleansUpWhileMovedLeaseWaits(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		wantStopped bool
	}{
		{name: "expired", err: registry.ErrLeaseExpired, wantStopped: true},
		{name: "moved", err: registry.ErrAssignmentCASConflict, wantStopped: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store, scope := setupLifecycle(t)
			backend := newFakeLifecycleBackend()
			backend.inventory[scope.ReplicaID] = BackendReplica{
				ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
				Host: "127.0.0.1", Port: 9103, Alive: true, Healthy: true,
			}
			adapter := RegistryStores{Store: store}
			h := NewLifecycleHandler(true, adapter, faultLifecycleStore{LifecycleStore: adapter, validateAssignmentErr: tc.err}, backend)
			err := h.ReconcileNode(ctx, scope.NodeID)
			if !tc.wantStopped && err == nil {
				t.Fatal("an uncertain assignment must report the read error instead of acting")
			}
			backend.mu.Lock()
			stops := backend.stops
			_, alive := backend.inventory[scope.ReplicaID]
			backend.mu.Unlock()
			rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantStopped {
				// An expired assignment is authoritative for this node: the process must go.
				if stops == 0 || alive || rep.State == contract.ReplicaReady {
					t.Fatalf("expired assignment kept serving: stops=%d alive=%v rep=%+v", stops, alive, rep)
				}
				return
			}
			if stops != 0 || !alive || rep.State == contract.ReplicaStopped {
				t.Fatalf("moved lease cleaned up a live replica: stops=%d alive=%v rep=%+v", stops, alive, rep)
			}
		})
	}
}

// A terminal replica is authoritative. The agent owns local process cleanup, the
// scheduler owns replacement through a new assignment, so reconcile must never turn a
// failed or stopped replica back into a ready one — not even when a healthy process of
// that replica is still around, and not by recording `starting` again.
func TestReconcileTerminalReplicaIsCleanedNotPromoted(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	h := NewLifecycleFromRegistry(true, store, backend)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "s3://cellp-celld/demo/v1"}
	if _, err := h.StartReplica(ctx, spec, "start-1"); err != nil {
		t.Fatal(err)
	}
	// The command path marks the replica failed (probe loss) and stops the process.
	backend.healthy = false
	probe := scope
	probe.Action = contract.ActionProbeReplica
	if _, err := h.ProbeReplica(ctx, probe); err != nil {
		t.Fatal(err)
	}
	// A healthy leftover process for the same failed replica must not be promoted.
	backend.healthy = true
	backend.mu.Lock()
	backend.inventory[scope.ReplicaID] = BackendReplica{
		ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
		Host: "127.0.0.1", Port: 9101, Alive: true, Healthy: true,
	}
	startsBefore := backend.starts
	backend.mu.Unlock()
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	backend.mu.Lock()
	stops, starts := backend.stops, backend.starts
	_, alive := backend.inventory[scope.ReplicaID]
	events := append([]string(nil), backend.events...)
	backend.mu.Unlock()
	if starts != startsBefore {
		t.Fatalf("reconcile restarted a terminal replica: starts=%d -> %d events=%v", startsBefore, starts, events)
	}
	if alive || stops == 0 {
		t.Fatalf("terminal replica process was not cleaned up: stops=%d alive=%v", stops, alive)
	}
	rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.State == contract.ReplicaReady || rep.State == contract.ReplicaStarting {
		t.Fatalf("terminal replica was revived by reconcile: %+v", rep)
	}
}

// Replacement belongs to the scheduler: it starts the version again through a new
// assignment and command, which the agent then executes. Reconcile alone must not.
func TestSchedulerStartCommandRevivesPreviouslyFailedReplica(t *testing.T) {
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
	rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil || rep.State != contract.ReplicaFailed {
		t.Fatalf("setup: rep=%+v err=%v", rep, err)
	}
	// The scheduler re-issues start for the same assignment after a fresh claim.
	probeScope := scope
	probeScope.Action = contract.ActionStartReplica
	got, err := h.StartReplica(ctx, spec, "start-2")
	if err != nil {
		t.Fatalf("scheduler start after failure: %v", err)
	}
	if got.State != contract.ReplicaReady {
		t.Fatalf("scheduler start revived: %+v", got)
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

// staleNodeOnceStore hands out one expired node snapshot, as a reconcile that read the
// node before a slow inventory leaves behind, and the live row afterwards.
type staleNodeOnceStore struct {
	NodeStore
	stale     *contract.RuntimeNode
	delivered bool
	mu        sync.Mutex
}

func (s *staleNodeOnceStore) GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error) {
	s.mu.Lock()
	if !s.delivered && s.stale != nil {
		s.delivered = true
		stale := s.stale
		s.mu.Unlock()
		return stale, nil
	}
	s.mu.Unlock()
	return s.NodeStore.GetRuntimeNode(ctx, nodeID)
}

// The inventory read can outlive the node snapshot it was taken with. A node whose lease
// has since been renewed is not inactive: deciding from the stale snapshot would take a
// healthy node — and with it every replica — offline.
func TestReconcileNodeRereadsNodeBeforeDeclaringItInactive(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	backend.inventory[scope.ReplicaID] = BackendReplica{
		ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
		Host: "127.0.0.1", Port: 9101, Alive: true, Healthy: false,
	}
	live, err := store.GetRuntimeNode(ctx, scope.NodeID)
	if err != nil || live == nil {
		t.Fatalf("node: %+v err=%v", live, err)
	}
	stale := *live
	stale.LeaseExpiry = time.Now().UTC().Add(-time.Minute)
	adapter := RegistryStores{Store: store}
	h := NewLifecycleHandler(true, &staleNodeOnceStore{NodeStore: adapter, stale: &stale}, adapter, backend)
	if err := h.ReconcileNode(ctx, scope.NodeID); err != nil {
		if ErrReconcileAuthorityFatal(err) {
			t.Fatalf("stale node snapshot was reported as authority loss: %v", err)
		}
		t.Fatalf("reconcile with a stale node snapshot: %v", err)
	}

	rep, err := store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.State == contract.ReplicaFailed || rep.State == contract.ReplicaStopped {
		t.Fatalf("stale node snapshot terminalized a live assignment: %+v", rep)
	}
	backend.mu.Lock()
	stops := backend.stops
	alive := true
	backend.mu.Unlock()
	if stops != 0 || !alive {
		t.Fatalf("stale node snapshot stopped a replica: stops=%d", stops)
	}
}

// withdrawOrderBackend inspects routing durability inside the only window that matters:
// the moment before the local process is stopped.
type withdrawOrderBackend struct {
	*fakeLifecycleBackend
	store   *registry.SQLiteStore
	before  int64
	checked bool
	err     error
}

func (b *withdrawOrderBackend) Stop(ctx context.Context, scope contract.CommandScope) error {
	rev, err := b.store.GetRouteRevision(ctx)
	if err != nil {
		b.err = err
		return err
	}
	rep, err := b.store.GetRuntimeReplica(ctx, scope.ReplicaID)
	if err != nil {
		b.err = err
		return err
	}
	b.checked = true
	if rev <= b.before {
		b.err = fmt.Errorf("routing revision %d was not bumped before the process stop (was %d)", rev, b.before)
	}
	if rep == nil || rep.State != contract.ReplicaDraining {
		b.err = fmt.Errorf("replica state %+v is still routable at the process stop", rep)
	}
	return b.fakeLifecycleBackend.Stop(ctx, scope)
}

// A Gateway routes from the published revision, so the durable withdrawal (endpoint
// removed + route revision bumped) must land before the local process is stopped.
// Otherwise a holder of the previous snapshot dials a port whose process is already gone.
func TestStopReplicaWithdrawsRoutingBeforeStoppingProcess(t *testing.T) {
	ctx := context.Background()
	store, scope := setupLifecycle(t)
	backend := newFakeLifecycleBackend()
	backend.inventory[scope.ReplicaID] = BackendReplica{
		ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
		Host: "127.0.0.1", Port: 9101, Alive: true, Healthy: true,
	}
	lease := scope.LeaseExpiry
	for _, obs := range []registry.ReplicaObservation{
		{
			ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
			NodeID: scope.NodeID, Generation: scope.Generation, State: contract.ReplicaStarting,
			AssignmentValidUntil: &lease,
		},
		{
			ReplicaID: scope.ReplicaID, ProjectID: scope.ProjectID, VersionID: scope.VersionID,
			NodeID: scope.NodeID, Generation: scope.Generation, State: contract.ReplicaReady,
			ListenHost: "127.0.0.1", ListenPort: 9101, EndpointState: contract.EndpointReady,
			AssignmentValidUntil: &lease, EndpointValidUntil: &lease,
		},
	} {
		if err := store.RecordObservation(ctx, obs); err != nil {
			t.Fatal(err)
		}
	}
	revBefore, err := store.GetRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}

	probe := &withdrawOrderBackend{fakeLifecycleBackend: backend, store: store, before: revBefore}
	h := NewLifecycleFromRegistry(true, store, probe)
	stopScope := scope
	stopScope.Action = contract.ActionStopReplica
	if _, err := h.StopReplica(ctx, stopScope); err != nil {
		t.Fatalf("stop replica: %v", err)
	}
	if !probe.checked {
		t.Fatal("backend stop was never reached")
	}
	if probe.err != nil {
		t.Fatal(probe.err)
	}
	if revAfter, err := store.GetRouteRevision(ctx); err != nil || revAfter <= revBefore {
		t.Fatalf("route revision after stop: %d err=%v (was %d)", revAfter, err, revBefore)
	}
}
