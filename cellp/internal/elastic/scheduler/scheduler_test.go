package scheduler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

type recordingClient struct {
	mu     sync.Mutex
	store  registry.ServingStore
	starts []contract.StartReplicaSpec
	idems  []string
	probes []contract.CommandScope
	drains []contract.CommandScope
	stops  []contract.CommandScope
}

func (c *recordingClient) StartReplica(ctx context.Context, spec contract.StartReplicaSpec, idem string) (contract.RuntimeReplica, error) {
	c.mu.Lock()
	c.starts = append(c.starts, spec)
	c.idems = append(c.idems, idem)
	port := 9000 + len(c.starts)
	c.mu.Unlock()
	rep := contract.RuntimeReplica{
		ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID,
		NodeID: spec.Scope.NodeID, Generation: spec.Scope.Generation, State: contract.ReplicaStarting,
		ValidUntil: &spec.Scope.LeaseExpiry,
	}
	if err := c.store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: rep.ReplicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID,
		NodeID: rep.NodeID, Generation: rep.Generation, State: contract.ReplicaStarting,
		AssignmentValidUntil: rep.ValidUntil,
	}); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if err := c.store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: rep.ReplicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID,
		NodeID: rep.NodeID, Generation: rep.Generation, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: port, EndpointState: contract.EndpointReady,
		AssignmentValidUntil: rep.ValidUntil, EndpointValidUntil: rep.ValidUntil,
	}); err != nil {
		return contract.RuntimeReplica{}, err
	}
	rep.State = contract.ReplicaReady
	return rep, nil
}

func (c *recordingClient) ProbeReplica(ctx context.Context, scope contract.CommandScope) (agent.ProbeResult, error) {
	c.mu.Lock()
	c.probes = append(c.probes, scope)
	c.mu.Unlock()
	rep, err := c.store.ValidateAgentAssignment(ctx, scope, time.Now().UTC())
	if err != nil {
		return agent.ProbeResult{}, err
	}
	if rep == nil {
		return agent.ProbeResult{}, registry.ErrObservationStale
	}
	return agent.ProbeResult{ReplicaID: rep.ReplicaID, State: rep.State, Generation: rep.Generation}, nil
}

func (c *recordingClient) DrainReplica(ctx context.Context, scope contract.CommandScope, _ time.Time) (contract.RuntimeReplica, error) {
	c.mu.Lock()
	c.drains = append(c.drains, scope)
	c.mu.Unlock()
	return c.cleanup(ctx, scope)
}

func (c *recordingClient) StopReplica(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	c.mu.Lock()
	c.stops = append(c.stops, scope)
	c.mu.Unlock()
	return c.cleanup(ctx, scope)
}

func (c *recordingClient) cleanup(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	rep, err := c.store.ValidateAgentCleanupAssignment(ctx, scope)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	if rep == nil {
		return contract.RuntimeReplica{}, registry.ErrObservationStale
	}
	if err := c.store.WithdrawReplica(ctx, rep.ReplicaID, rep.ProjectID, rep.VersionID, rep.NodeID, rep.Generation); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if err := c.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaStopped); err != nil {
		return contract.RuntimeReplica{}, err
	}
	rep.State = contract.ReplicaStopped
	return *rep, nil
}

func (c *recordingClient) counts() (starts, probes, drains, stops int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.starts), len(c.probes), len(c.drains), len(c.stops)
}

type schedulerFixture struct {
	store   *registry.SQLiteStore
	clients map[string]*recordingClient
	ctrl    *Controller
	now     time.Time
}

func newSchedulerFixture(t *testing.T, desired int, nodes ...contract.RuntimeNode) *schedulerFixture {
	t.Helper()
	t.Setenv(contract.EnvElasticRuntime, "1")
	store, err := registry.Open(t.TempDir() + "/scheduler.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1, MinReplicas: 0, MaxReplicas: 8,
		BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: desired, Generation: 1, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	clients := make(map[string]*recordingClient, len(nodes))
	for _, node := range nodes {
		if node.LeaseExpiry.IsZero() {
			node.LeaseExpiry = now.Add(time.Hour)
		}
		if err := store.UpsertRuntimeNode(ctx, node); err != nil {
			t.Fatal(err)
		}
		clients[node.NodeID] = &recordingClient{store: store}
	}
	ctrl := &Controller{
		Store: RegistryStoreFromServing(store), Guard: NoopGuard{}, Now: func() time.Time { return now },
		Clients: func(node contract.RuntimeNode) (RuntimeNodeClient, error) {
			client := clients[node.NodeID]
			if client == nil {
				return nil, errors.New("unknown node")
			}
			return client, nil
		},
	}
	return &schedulerFixture{store: store, clients: clients, ctrl: ctrl, now: now}
}

func eligibleNode(id string, capacity int, generation int64) contract.RuntimeNode {
	return contract.RuntimeNode{
		NodeID: id, CapacityUnits: capacity, Generation: generation,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp/test/node/" + id, Zone: "zone-a",
	}
}

func TestPickNodeDeterministicEligibilityCapacityAndSpread(t *testing.T) {
	now := time.Now().UTC()
	n1, n2 := eligibleNode("n1", 2, 1), eligibleNode("n2", 2, 1)
	n1.LeaseExpiry, n2.LeaseExpiry = now.Add(time.Hour), now.Add(time.Hour)
	exp := now.Add(time.Hour)
	replicas := []contract.RuntimeReplica{
		{ReplicaID: "other", ProjectID: "other", VersionID: "v", NodeID: "n1", Generation: 1, AssignedNodeGeneration: 1, State: contract.ReplicaReady, ValidUntil: &exp},
		{ReplicaID: "same", ProjectID: "demo", VersionID: "v1", NodeID: "n2", Generation: 1, AssignedNodeGeneration: 1, State: contract.ReplicaReady, ValidUntil: &exp},
	}
	pick := PickNode(PlacementInput{ProjectID: "demo", VersionID: "v1", Nodes: []contract.RuntimeNode{n2, n1}, Replicas: replicas, Now: now})
	if pick == nil || pick.NodeID != "n1" {
		t.Fatalf("anti-affinity selection: %+v", pick)
	}
	n1.CapacityUnits = 1
	pick = PickNode(PlacementInput{ProjectID: "demo", VersionID: "v1", Nodes: []contract.RuntimeNode{n1, n2}, Replicas: replicas, Now: now})
	if pick == nil || pick.NodeID != "n2" {
		t.Fatalf("capacity selection: %+v", pick)
	}
	bad := []contract.RuntimeNode{n1, n2}
	bad[0].Cordoned = true
	bad[1].LeaseExpiry = now
	if pick := PickNode(PlacementInput{ProjectID: "demo", VersionID: "v1", Nodes: bad, Now: now}); pick != nil {
		t.Fatalf("ineligible node selected: %+v", pick)
	}
	bad[0] = eligibleNode("n1", 1, 1)
	bad[0].LeaseExpiry = now.Add(time.Hour)
	bad[0].IdentityURI = ""
	if pick := PickNode(PlacementInput{ProjectID: "demo", VersionID: "v1", Nodes: bad[:1], Now: now}); pick != nil {
		t.Fatalf("incomplete metadata selected: %+v", pick)
	}
	stale := []contract.RuntimeReplica{{ReplicaID: "stale", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1, AssignedNodeGeneration: 9, State: contract.ReplicaReady, ValidUntil: &exp}}
	bad[0] = eligibleNode("n1", 1, 1)
	bad[0].LeaseExpiry = now.Add(time.Hour)
	if pick := PickNode(PlacementInput{ProjectID: "demo", VersionID: "v1", Nodes: bad[:1], Replicas: stale, Now: now}); pick != nil {
		t.Fatalf("uncertain live assignment capacity was reused: %+v", pick)
	}
}

func TestControllerZeroToNOneToNScaleDownAndRestart(t *testing.T) {
	fix := newSchedulerFixture(t, 2, eligibleNode("n1", 2, 1), eligibleNode("n2", 2, 1))
	ctx := context.Background()
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	reps, err := fix.store.ListRuntimeReplicas(ctx, "demo", "v1")
	if err != nil || len(reps) != 2 || reps[0].NodeID == reps[1].NodeID {
		t.Fatalf("0->N placement: %+v err=%v", reps, err)
	}
	startsBefore := 0
	for _, client := range fix.clients {
		starts, _, _, _ := client.counts()
		startsBefore += starts
	}
	restarted := *fix.ctrl
	if _, err := restarted.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	startsAfter := 0
	for _, client := range fix.clients {
		starts, _, _, _ := client.counts()
		startsAfter += starts
	}
	if startsAfter != startsBefore {
		t.Fatalf("restart/repeated tick duplicated starts: %d -> %d", startsBefore, startsAfter)
	}
	if err := fix.store.CompareAndSetDesired(ctx, "demo", "v1", 1, registry.ServingDesireRow{DesiredReplicas: 3, Generation: 2, Reason: "scale-up"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	reps, _ = fix.store.ListRuntimeReplicas(ctx, "demo", "v1")
	active := 0
	for _, rep := range reps {
		if rep.State == contract.ReplicaReady {
			active++
		}
	}
	if active != 3 {
		t.Fatalf("1->N active=%d reps=%+v", active, reps)
	}
	if err := fix.store.CompareAndSetDesired(ctx, "demo", "v1", 2, registry.ServingDesireRow{DesiredReplicas: 1, Generation: 3, Reason: "scale-down"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	reps, _ = fix.store.ListRuntimeReplicas(ctx, "demo", "v1")
	active = 0
	for _, rep := range reps {
		if rep.State == contract.ReplicaReady {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("safe scale-down active=%d reps=%+v", active, reps)
	}
}

func TestControllerCordonedNodeAndExpiredLeaseAreNotDispatched(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	cordoned := eligibleNode("cordoned", 1, 1)
	cordoned.Cordoned = true
	cordoned.LeaseExpiry = now.Add(time.Hour)
	expired := eligibleNode("expired", 1, 1)
	expired.LeaseExpiry = now.Add(-time.Second)
	fix := newSchedulerFixture(t, 1, cordoned, expired)
	fix.now = now
	fix.ctrl.Now = func() time.Time { return now }
	if _, err := fix.ctrl.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	reps, err := fix.store.ListRuntimeReplicas(context.Background(), "demo", "v1")
	if err != nil || len(reps) != 0 {
		t.Fatalf("ineligible nodes received assignment: %+v err=%v", reps, err)
	}
	for _, client := range fix.clients {
		starts, probes, drains, stops := client.counts()
		if starts+probes+drains+stops != 0 {
			t.Fatal("ineligible node received lifecycle command")
		}
	}
}

func TestControllerFencesNodeGenerationAndRegistryUncertainty(t *testing.T) {
	fix := newSchedulerFixture(t, 1, eligibleNode("n1", 2, 1))
	ctx := context.Background()
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	node := eligibleNode("n1", 2, 2)
	node.LeaseExpiry = fix.now.Add(2 * time.Hour)
	if err := fix.store.UpsertRuntimeNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	reps, err := fix.store.ListRuntimeReplicas(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	var failed, ready int
	for _, rep := range reps {
		if rep.State == contract.ReplicaFailed {
			failed++
		}
		if rep.State == contract.ReplicaReady && rep.AssignedNodeGeneration == 2 {
			ready++
		}
	}
	if failed != 1 || ready != 1 {
		t.Fatalf("node-generation replacement: %+v", reps)
	}
	fault := &faultStore{Store: RegistryStoreFromServing(fix.store), listNodesErr: errors.New("registry unavailable")}
	ctrl := *fix.ctrl
	ctrl.Store = fault
	startsBefore, _, drainsBefore, stopsBefore := fix.clients["n1"].counts()
	if _, err := ctrl.Tick(ctx); !errors.Is(err, fault.listNodesErr) {
		t.Fatalf("registry error not returned: %v", err)
	}
	startsAfter, _, drainsAfter, stopsAfter := fix.clients["n1"].counts()
	if startsAfter != startsBefore || drainsAfter != drainsBefore || stopsAfter != stopsBefore {
		t.Fatal("registry uncertainty dispatched lifecycle work")
	}
}

type faultStore struct {
	Store
	listNodesErr error
}

func (s *faultStore) ListRuntimeNodes(context.Context) ([]contract.RuntimeNode, error) {
	return nil, s.listNodesErr
}

type losingGuard struct {
	mu    sync.Mutex
	calls int
	lose  int
}

func (g *losingGuard) HoldsWriteLock(context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.calls >= g.lose {
		return ErrGuardLost
	}
	return nil
}

func TestControllerScaleUpZeroToNConverges(t *testing.T) {
	fix := newSchedulerFixture(t, 3, eligibleNode("n1", 2, 1), eligibleNode("n2", 2, 1))
	ctx := context.Background()
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	reps, err := fix.store.ListRuntimeReplicas(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if CountMatchingActive(reps, fix.now) != 3 {
		t.Fatalf("0->N active=%d reps=%+v", CountMatchingActive(reps, fix.now), reps)
	}
}

func TestControllerDesiredZeroMinZeroTerminalsAll(t *testing.T) {
	fix := newSchedulerFixture(t, 1, eligibleNode("n1", 2, 1))
	ctx := context.Background()
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fix.store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 2, MinReplicas: 0, MaxReplicas: 8,
		BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := fix.store.CompareAndSetDesired(ctx, "demo", "v1", 1, registry.ServingDesireRow{
		DesiredReplicas: 0, Generation: 2, Reason: "scale-to-zero",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	reps, err := fix.store.ListRuntimeReplicas(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, rep := range reps {
		if !contract.ReplicaIsTerminal(rep.State) {
			t.Fatalf("expected all terminal: %+v", reps)
		}
	}
}

func TestPlacementDegradedSingleNodeAntiAffinity(t *testing.T) {
	SetPlacementDegraded(false)
	fix := newSchedulerFixture(t, 2, eligibleNode("solo", 4, 1))
	ctx := context.Background()
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if !PlacementDegraded.Load() {
		t.Fatal("expected placement_degraded=true for single-node anti-affinity violation")
	}
	reps, _ := fix.store.ListRuntimeReplicas(ctx, "demo", "v1")
	if len(reps) != 2 || reps[0].NodeID != "solo" || reps[1].NodeID != "solo" {
		t.Fatalf("replicas: %+v", reps)
	}
}

func TestLoadConfigRejectsNonPositiveInterval(t *testing.T) {
	t.Setenv("CELLP_SCHEDULER_INTERVAL", "-1s")
	cfg := LoadConfig()
	if !cfg.Background || cfg.Interval != defaultInterval {
		t.Fatalf("config: %+v", cfg)
	}
}

func TestControllerRequiresGuard(t *testing.T) {
	fix := newSchedulerFixture(t, 1, eligibleNode("n1", 1, 1))
	fix.ctrl.Guard = nil
	if _, err := fix.ctrl.Tick(context.Background()); !errors.Is(err, ErrGuardLost) {
		t.Fatalf("missing guard: %v", err)
	}
	reps, err := fix.store.ListRuntimeReplicas(context.Background(), "demo", "v1")
	if err != nil || len(reps) != 0 {
		t.Fatalf("missing guard wrote assignments: %+v err=%v", reps, err)
	}
}

func TestControllerGuardLossStopsBeforeWritesAndRunQuiesces(t *testing.T) {
	fix := newSchedulerFixture(t, 1, eligibleNode("n1", 1, 1))
	guard := &losingGuard{lose: 2}
	ctrl := *fix.ctrl
	ctrl.Guard = guard
	if _, err := ctrl.Tick(context.Background()); !errors.Is(err, ErrGuardLost) {
		t.Fatalf("guard loss: %v", err)
	}
	reps, err := fix.store.ListRuntimeReplicas(context.Background(), "demo", "v1")
	if err != nil || len(reps) != 0 {
		t.Fatalf("guard loss wrote assignments: %+v err=%v", reps, err)
	}
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := Start(ctx, &ctrl, Config{Background: true, Interval: time.Millisecond}, errCh)
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrGuardLost) {
			t.Fatalf("run guard error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not report guard loss")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not quiesce")
	}
}

func TestControllerRemoteDispatchScopeAndLeaseRenewal(t *testing.T) {
	fix := newSchedulerFixture(t, 1, eligibleNode("remote", 1, 1))
	ctx := context.Background()
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	client := fix.clients["remote"]
	client.mu.Lock()
	if len(client.starts) != 1 || len(client.idems) != 1 {
		client.mu.Unlock()
		t.Fatalf("remote dispatch starts=%d idems=%d", len(client.starts), len(client.idems))
	}
	spec, idem := client.starts[0], client.idems[0]
	client.mu.Unlock()
	if spec.Scope.NodeID != "remote" || spec.Scope.Action != contract.ActionStartReplica || spec.Scope.Nonce == "" ||
		!spec.Scope.LeaseExpiry.After(fix.now) || spec.Bucket != "s3://cellp-celld/demo/v1" || idem == "" {
		t.Fatalf("remote command contract: spec=%+v idem=%q", spec, idem)
	}
	rep, err := fix.store.GetRuntimeReplica(ctx, spec.Scope.ReplicaID)
	if err != nil || rep == nil {
		t.Fatalf("replica: %+v err=%v", rep, err)
	}
	oldExpiry := *rep.ValidUntil
	if err := fix.store.RenewRuntimeNodeLease(ctx, "remote", 1, fix.now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	nearExpiry := oldExpiry.Add(-10 * time.Second)
	fix.ctrl.Now = func() time.Time { return nearExpiry }
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	renewed, _ := fix.store.GetRuntimeReplica(ctx, rep.ReplicaID)
	if renewed.ValidUntil == nil || !renewed.ValidUntil.After(oldExpiry) {
		t.Fatalf("assignment lease not renewed: %+v", renewed)
	}
}

type blockingStartClient struct {
	inner *recordingClient
	delay time.Duration

	mu      sync.Mutex
	active  int
	maxSeen int
}

func (b *blockingStartClient) StartReplica(ctx context.Context, spec contract.StartReplicaSpec, idem string) (contract.RuntimeReplica, error) {
	b.mu.Lock()
	b.active++
	if b.active > b.maxSeen {
		b.maxSeen = b.active
	}
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		b.mu.Lock()
		b.active--
		b.mu.Unlock()
		return contract.RuntimeReplica{}, ctx.Err()
	case <-time.After(b.delay):
	}
	b.mu.Lock()
	b.active--
	b.mu.Unlock()
	return b.inner.StartReplica(ctx, spec, idem)
}

func (b *blockingStartClient) ProbeReplica(ctx context.Context, scope contract.CommandScope) (agent.ProbeResult, error) {
	return b.inner.ProbeReplica(ctx, scope)
}

func (b *blockingStartClient) DrainReplica(ctx context.Context, scope contract.CommandScope, until time.Time) (contract.RuntimeReplica, error) {
	return b.inner.DrainReplica(ctx, scope, until)
}

func (b *blockingStartClient) StopReplica(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	return b.inner.StopReplica(ctx, scope)
}

func TestControllerTickSerializesConcurrently(t *testing.T) {
	fix := newSchedulerFixture(t, 1, eligibleNode("n1", 1, 1))
	blocking := &blockingStartClient{inner: fix.clients["n1"], delay: 80 * time.Millisecond}
	fix.ctrl.Clients = func(node contract.RuntimeNode) (RuntimeNodeClient, error) {
		if node.NodeID == "n1" {
			return blocking, nil
		}
		return nil, errors.New("unexpected node")
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	var tickErr atomic.Value
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := fix.ctrl.Tick(ctx); err != nil {
				tickErr.Store(err)
			}
		}()
	}
	wg.Wait()
	if err := tickErr.Load(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	blocking.mu.Lock()
	peak := blocking.maxSeen
	blocking.mu.Unlock()
	if peak != 1 {
		t.Fatalf("expected serialized ticks, peak concurrent=%d", peak)
	}
}

type schedulerTestBackend struct {
	mu        sync.Mutex
	inventory map[string]agent.BackendReplica
}

func (b *schedulerTestBackend) ExpectedReplicaBucket(scope contract.CommandScope) (string, error) {
	return "s3://cellp-celld/" + scope.ProjectID + "/" + scope.VersionID, nil
}
func (b *schedulerTestBackend) Diagnose(context.Context, contract.StartReplicaSpec) error { return nil }
func (b *schedulerTestBackend) Start(_ context.Context, spec contract.StartReplicaSpec) (string, int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item := agent.BackendReplica{ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID, Host: "127.0.0.1", Port: 9911, Healthy: true}
	b.inventory[item.ReplicaID] = item
	return item.Host, item.Port, nil
}
func (b *schedulerTestBackend) Probe(_ context.Context, scope contract.CommandScope) (agent.BackendReplica, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.inventory[scope.ReplicaID]
	if !ok {
		return agent.BackendReplica{}, errors.New("not running")
	}
	return item, nil
}
func (b *schedulerTestBackend) Drain(_ context.Context, scope contract.CommandScope, _ time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.inventory, scope.ReplicaID)
	return nil
}
func (b *schedulerTestBackend) Stop(_ context.Context, scope contract.CommandScope) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.inventory, scope.ReplicaID)
	return nil
}
func (b *schedulerTestBackend) List(context.Context) ([]agent.BackendReplica, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]agent.BackendReplica, 0, len(b.inventory))
	for _, item := range b.inventory {
		out = append(out, item)
	}
	return out, nil
}

type schedulerTestPKI struct {
	pool       *x509.CertPool
	server     tls.Certificate
	client     tls.Certificate
	nodeURI    string
	controller string
}

func newSchedulerTestPKI(t *testing.T) schedulerTestPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "scheduler-test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	nodeURI := "spiffe://cellp/test/node/remote"
	controllerURI := "spiffe://cellp/test/controller/scheduler"
	return schedulerTestPKI{
		pool: pool, nodeURI: nodeURI, controller: controllerURI,
		server: schedulerLeaf(t, ca, caKey, nodeURI), client: schedulerLeaf(t, ca, caKey, controllerURI),
	}
}

func schedulerLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, identity string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identityURI, err := url.Parse(identity)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "leaf"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, URIs: []*url.URL{identityURI},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	cert.Leaf, _ = x509.ParseCertificate(der)
	return cert
}

func TestControllerDispatchesThroughRealMTLSAgent(t *testing.T) {
	t.Setenv(contract.EnvElasticRuntime, "1")
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/remote-mtls.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{ProjectID: "demo", VersionID: "v1", Revision: 1, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	pki := newSchedulerTestPKI(t)
	backend := &schedulerTestBackend{inventory: make(map[string]agent.BackendReplica)}
	handler := agent.NewLifecycleFromRegistry(true, store, backend)
	server, err := transport.NewServer(transport.ServerConfig{
		Enabled: true, NodeID: "remote", BindAddr: "127.0.0.1:0", NodeIdentityURI: pki.nodeURI,
		AllowedControllerURIs: []string{pki.controller}, TLS: transport.TLSMaterials{Cert: pki.server, RootCAs: pki.pool, ServerName: "127.0.0.1"},
	}, handler)
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, cancelServer := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(serverCtx) }()
	select {
	case <-server.Ready():
	case <-time.After(time.Second):
		t.Fatal("agent server not ready")
	}
	defer func() {
		cancelServer()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serverDone
	}()
	now := time.Now().UTC().Truncate(time.Microsecond)
	node := contract.RuntimeNode{
		NodeID: "remote", CapacityUnits: 1, Generation: 1, LeaseExpiry: now.Add(time.Hour),
		AgentBaseURL: "https://" + server.Addr(), IdentityURI: pki.nodeURI, Zone: "zone-a",
	}
	if err := store.UpsertRuntimeNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	clientMaterials := transport.TLSMaterials{Cert: pki.client, RootCAs: pki.pool, ServerName: "127.0.0.1"}
	ctrl := &Controller{
		Store: RegistryStoreFromServing(store), Guard: NoopGuard{}, Now: func() time.Time { return now },
		Clients: func(node contract.RuntimeNode) (RuntimeNodeClient, error) {
			return transport.NewClient(transport.ClientConfig{
				BaseURL: node.AgentBaseURL, ExpectedNodeURI: node.IdentityURI,
				ControllerIdentityURI: pki.controller, TLS: clientMaterials,
			})
		},
	}
	var reps []contract.RuntimeReplica
	for attempt := 0; attempt < 20; attempt++ {
		if _, err := ctrl.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		reps, err = store.ListRuntimeReplicas(ctx, "demo", "v1")
		if err != nil {
			t.Fatal(err)
		}
		if len(reps) == 1 && reps[0].State == contract.ReplicaReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(reps) != 1 || reps[0].State != contract.ReplicaReady || reps[0].NodeID != "remote" {
		t.Fatalf("mTLS scheduler result: %+v err=%v", reps, err)
	}
}

func TestControllerColdActivationTransitionsVersionAndIsRestartSafe(t *testing.T) {
	fix := newSchedulerFixture(t, 0, eligibleNode("n1", 1, 1))
	ctx := context.Background()
	if err := fix.store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	version, err := fix.store.GetVersion(ctx, "demo", "v1")
	if err != nil || version == nil || version.Status != registry.StatusDeployReady {
		t.Fatalf("cold status: version=%+v err=%v", version, err)
	}
	desire, err := fix.store.GetServingDesire(ctx, "demo", "v1")
	if err != nil || desire == nil {
		t.Fatalf("desire: %+v err=%v", desire, err)
	}
	if err := fix.store.CompareAndSetDesired(ctx, "demo", "v1", desire.Generation, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: desire.Generation + 1, Reason: "activator_ensure",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.ctrl.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	version, err = fix.store.GetVersion(ctx, "demo", "v1")
	if err != nil || version == nil || version.Status != registry.StatusReady {
		t.Fatalf("activated status: version=%+v err=%v", version, err)
	}
	startsBefore, _, _, _ := fix.clients["n1"].counts()
	restarted := *fix.ctrl
	if _, err := restarted.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	startsAfter, _, _, _ := fix.clients["n1"].counts()
	if startsAfter != startsBefore {
		t.Fatalf("restart duplicated start: %d -> %d", startsBefore, startsAfter)
	}
}
