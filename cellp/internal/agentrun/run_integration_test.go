package agentrun

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

// --- test doubles (orchestration only; production uses real HTTPS+mTLS) ---

type scriptedNodeReg struct {
	mu sync.Mutex

	activateErr error
	status      contract.StatusNodeResponse
	generation  int64

	heartbeatCalls atomic.Int32
	heartbeatFn    func(call int) error

	releaseCalls atomic.Int32
	releaseErr     error
}

func (s *scriptedNodeReg) Status(context.Context, contract.StatusNodeRequest) (contract.StatusNodeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, nil
}

func (s *scriptedNodeReg) Activate(context.Context, contract.ActivateNodeRequest) (contract.ActivateNodeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activateErr != nil {
		return contract.ActivateNodeResponse{}, s.activateErr
	}
	if s.generation == 0 {
		s.generation = 1
	}
	return contract.ActivateNodeResponse{Generation: s.generation}, nil
}

func (s *scriptedNodeReg) Heartbeat(context.Context, contract.HeartbeatNodeRequest) error {
	call := int(s.heartbeatCalls.Add(1))
	s.mu.Lock()
	fn := s.heartbeatFn
	s.mu.Unlock()
	if fn != nil {
		return fn(call)
	}
	return nil
}

func (s *scriptedNodeReg) Release(context.Context, contract.ReleaseNodeRequest) error {
	s.releaseCalls.Add(1)
	s.mu.Lock()
	err := s.releaseErr
	s.mu.Unlock()
	return err
}

type staticLifecycleStores struct {
	nodeID string
}

func (s staticLifecycleStores) UpsertRuntimeNode(context.Context, contract.RuntimeNode) error { return nil }
func (s staticLifecycleStores) ListRuntimeNodes(context.Context) ([]contract.RuntimeNode, error) {
	return nil, nil
}

func (s staticLifecycleStores) GetRuntimeNode(_ context.Context, nodeID string) (*contract.RuntimeNode, error) {
	if nodeID != s.nodeID {
		return nil, nil
	}
	now := time.Now().UTC()
	return &contract.RuntimeNode{
		NodeID: s.nodeID, LeaseExpiry: now.Add(time.Hour), CapacityUnits: 1,
	}, nil
}

func (s staticLifecycleStores) GetRuntimeReplica(context.Context, string) (*contract.RuntimeReplica, error) {
	return nil, nil
}
func (s staticLifecycleStores) ValidateAgentAssignment(context.Context, contract.CommandScope, time.Time) (*contract.RuntimeReplica, error) {
	return nil, nil
}
func (s staticLifecycleStores) ValidateAgentCleanupAssignment(context.Context, contract.CommandScope) (*contract.RuntimeReplica, error) {
	return nil, nil
}
func (s staticLifecycleStores) ListRuntimeReplicasByNode(context.Context, string) ([]contract.RuntimeReplica, error) {
	return nil, nil
}
func (s staticLifecycleStores) RecordObservation(context.Context, registry.ReplicaObservation) error { return nil }
func (s staticLifecycleStores) ClaimAgentCommand(context.Context, registry.AgentCommand) (registry.AgentCommandClaim, error) {
	return registry.AgentCommandClaim{}, nil
}
func (s staticLifecycleStores) RenewAgentCommandLease(context.Context, registry.AgentCommand, time.Time) error {
	return nil
}
func (s staticLifecycleStores) CompleteAgentCommand(context.Context, registry.AgentCommand) error { return nil }
func (s staticLifecycleStores) RecordObservationAndCompleteAgentCommand(context.Context, registry.ReplicaObservation, registry.AgentCommand) error {
	return nil
}
func (s staticLifecycleStores) WithdrawReplica(context.Context, string, string, string, string, int64) error {
	return nil
}
func (s staticLifecycleStores) TerminalizeReplica(context.Context, string, string, int64, contract.ReplicaState) error {
	return nil
}

type spyBackend struct {
	agent.LifecycleBackend

	listFn func(call int) ([]agent.BackendReplica, error)
	listN  atomic.Int32

	startN atomic.Int32
	probeN atomic.Int32
	stopN  atomic.Int32
	drainN atomic.Int32

	bootReconcile chan struct{}
}

func (b *spyBackend) ExpectedReplicaBucket(contract.CommandScope) (string, error) { return "s3://b/p/v", nil }
func (b *spyBackend) Diagnose(context.Context, contract.StartReplicaSpec) error   { return nil }
func (b *spyBackend) Start(context.Context, contract.StartReplicaSpec) (string, int, error) {
	b.startN.Add(1)
	return "127.0.0.1", 1, nil
}
func (b *spyBackend) Probe(context.Context, contract.CommandScope) (agent.BackendReplica, error) {
	b.probeN.Add(1)
	return agent.BackendReplica{}, nil
}
func (b *spyBackend) Drain(context.Context, contract.CommandScope, time.Time) error {
	b.drainN.Add(1)
	return nil
}
func (b *spyBackend) Stop(context.Context, contract.CommandScope) error {
	b.stopN.Add(1)
	return nil
}

func (b *spyBackend) List(context.Context) ([]agent.BackendReplica, error) {
	call := int(b.listN.Add(1))
	if call == 1 && b.bootReconcile != nil {
		close(b.bootReconcile)
	}
	if b.listFn != nil {
		return b.listFn(call)
	}
	return nil, nil
}

func freeTCPBind(t *testing.T) (bind string, advertise string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr, "https://" + addr
}

func waitListenerClosed(t *testing.T, bind string, deadline time.Time) {
	t.Helper()
	d := net.Dialer{Timeout: 50 * time.Millisecond}
	for time.Now().Before(deadline) {
		c, err := d.Dial("tcp", bind)
		if err != nil {
			return
		}
		_ = c.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("agent listener still accepting connections")
}

func runAgentUntil(t *testing.T, ctx context.Context, cfg config.ElasticConfig, deps Deps) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, config.Config{}, deps) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		err := <-done
		if err == nil {
			return ctx.Err()
		}
		return err
	}
}

func integrationElasticCfg(bind string, advertise string, pki integrationPKI) config.ElasticConfig {
	return config.ElasticConfig{
		Enabled: true, NodeID: "n-test", AgentBindAddr: bind, AgentBaseURL: advertise,
		NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		AllowedControllerURIs: []string{pki.ControllerURI},
		CapacityUnits: 1, Zone: "z",
		NodeHeartbeatTTL: 30 * time.Second, NodeHeartbeatInterval: 20 * time.Millisecond,
		AgentReconcileInterval: 20 * time.Millisecond,
		MaxBodyBytes: 1 << 20, ReplayMaxEntries: 128, RelayScopeTTL: time.Minute,
		NodeRegBaseURL: "https://unused", RegistryRelayBaseURL: "https://unused",
	}
}

func integrationDeps(pki integrationPKI, nd *scriptedNodeReg, backend agent.LifecycleBackend) Deps {
	return Deps{
		LoadTLS: func(cfg config.ElasticConfig) (TLSBundle, error) {
			client := pki.clientTLS()
			client.ServerName = cfg.ResolvedControllerTLSServerName()
			return TLSBundle{
				Server: agenttransport.TLSMaterials{Cert: pki.ServerCert, RootCAs: pki.RootPool},
				Client: client,
			}, nil
		},
		DialNodeReg: func(_ config.ElasticConfig, _ agenttransport.TLSMaterials) (nodeRegistrationClient, error) {
			return nd, nil
		},
		LifecycleStores: func(_ *registryrelaytransport.Client, elasticCfg config.ElasticConfig) (agent.NodeStore, agent.LifecycleStore) {
			st := staticLifecycleStores{nodeID: elasticCfg.NodeID}
			return st, st
		},
		NewBackend: func(*runtime.Manager) agent.LifecycleBackend { return backend },
	}
}

func TestRunActivateFailureClosesListenerNoRelease(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{activateErr: errors.New("activate denied")}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, &spyBackend{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runAgentUntil(t, ctx, cfg, deps)
	if err == nil || !strings.Contains(err.Error(), "nodereg activate") {
		t.Fatalf("expected activate error, got %v", err)
	}
	if nd.releaseCalls.Load() != 0 {
		t.Fatalf("must not release lease when activate never succeeded, releases=%d", nd.releaseCalls.Load())
	}
	waitListenerClosed(t, bind, time.Now().Add(3*time.Second))
}

func TestRunBootReconcileObservedAndFatalRollsBackLease(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{}
	boot := make(chan struct{})
	backend := &spyBackend{
		bootReconcile: boot,
		listFn: func(call int) ([]agent.BackendReplica, error) {
			if call == 1 {
				return nil, errors.New("boot reconcile inventory failed")
			}
			return nil, nil
		},
	}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, backend)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runAgentUntil(t, ctx, cfg, deps)
	if err == nil || !strings.Contains(err.Error(), "agent boot reconcile") {
		t.Fatalf("expected boot reconcile error, got %v", err)
	}
	select {
	case <-boot:
	case <-ctx.Done():
		t.Fatal("boot ReconcileNode did not run before failure")
	}
	if nd.releaseCalls.Load() != 1 {
		t.Fatalf("expected lease release after boot fatal with generation, got %d", nd.releaseCalls.Load())
	}
	waitListenerClosed(t, bind, time.Now().Add(3*time.Second))
}

func TestRunHeartbeatAuthoritativeFatalStopsRunner(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{
		heartbeatFn: func(call int) error {
			if call >= 2 {
				return registry.ErrNodeLeaseCASConflict
			}
			return nil
		},
	}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, &spyBackend{bootReconcile: make(chan struct{})})

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, cfg, config.Config{}, deps) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err, ok := tryRecv(done); ok {
			cancel()
			_ = waitRunDone(done, 2*time.Second)
			if err == nil || !strings.Contains(err.Error(), "agent heartbeat") {
				t.Fatalf("expected heartbeat fatal, got %v", err)
			}
			waitListenerClosed(t, bind, time.Now().Add(2*time.Second))
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	t.Fatal("runner did not exit on authoritative heartbeat")
}

func TestRunHeartbeatTransientThenRecovers(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	var transient atomic.Bool
	transient.Store(true)
	nd := &scriptedNodeReg{
		heartbeatFn: func(call int) error {
			if transient.Load() && call == 2 {
				transient.Store(false)
				return errors.New("temporary nodereg outage")
			}
			return nil
		},
	}
	backend := &spyBackend{bootReconcile: make(chan struct{})}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, backend)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, cfg, config.Config{}, deps) }()

	waitUntil(t, 2*time.Second, func() bool { return nd.heartbeatCalls.Load() >= 3 })
	cancel()
	err := waitRunDone(done, 3*time.Second)
	if err != nil && !errors.Is(err, context.Canceled) && !isOnlyContextCancellation(err) {
		t.Fatalf("expected clean cancel after transient heartbeat, got %v", err)
	}
	if nd.heartbeatCalls.Load() < 3 {
		t.Fatalf("expected heartbeats to continue after transient error")
	}
}

func TestRunReconcileTransientKeepsRunner(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{}
	var transient atomic.Bool
	transient.Store(true)
	backend := &spyBackend{
		bootReconcile: make(chan struct{}),
		listFn: func(call int) ([]agent.BackendReplica, error) {
			if call > 1 && transient.Load() {
				transient.Store(false)
				return nil, registryrelay.ErrRegistryUnavailable
			}
			return nil, nil
		},
	}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, backend)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, cfg, config.Config{}, deps) }()

	waitUntil(t, 2*time.Second, func() bool { return backend.listN.Load() >= 3 })
	cancel()
	err := waitRunDone(done, 3*time.Second)
	if err != nil && !isOnlyContextCancellation(err) {
		t.Fatalf("expected cancel-only shutdown, got %v", err)
	}
}

func TestRunReconcileFatalExitsRunner(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{}
	backend := &spyBackend{
		bootReconcile: make(chan struct{}),
		listFn: func(call int) ([]agent.BackendReplica, error) {
			if call > 1 {
				return nil, registry.ErrLeaseExpired
			}
			return nil, nil
		},
	}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, backend)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, cfg, config.Config{}, deps) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err, ok := tryRecv(done); ok {
			cancel()
			_ = waitRunDone(done, 2*time.Second)
			if err == nil || !strings.Contains(err.Error(), "agent reconcile") {
				t.Fatalf("expected reconcile fatal, got %v", err)
			}
			waitListenerClosed(t, bind, time.Now().Add(2*time.Second))
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	t.Fatal("runner did not exit on fatal reconcile")
}

func TestRunShutdownReleaseFailureJoinedInError(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{releaseErr: errors.New("nodereg release failed")}
	backend := &spyBackend{bootReconcile: make(chan struct{})}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, backend)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, cfg, config.Config{}, deps) }()
	waitUntil(t, 2*time.Second, func() bool { return backend.listN.Load() >= 1 })
	cancel()
	err := waitRunDone(done, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "agent lease release") {
		t.Fatalf("expected release failure in Run error, got %v", err)
	}
	if nd.releaseCalls.Load() == 0 {
		t.Fatal("expected release attempt on quiesced shutdown")
	}
}

func TestRunBootReconcileExecutesBeforeServing(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{}
	boot := make(chan struct{})
	backend := &spyBackend{bootReconcile: boot}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, backend)

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, cfg, config.Config{}, deps) }()

	select {
	case <-boot:
	case <-time.After(2 * time.Second):
		t.Fatal("boot ReconcileNode did not run")
	}
	cancel()
	if err := waitRunDone(done, 5*time.Second); err != nil && !isOnlyContextCancellation(err) {
		t.Fatalf("expected cancel shutdown, got %v", err)
	}
}

func TestRunBlockedBootReconcileRejectsLifecycleUntilReady(t *testing.T) {
	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{}
	bootStarted := make(chan struct{})
	bootRelease := make(chan struct{})
	backend := &spyBackend{
		listFn: func(call int) ([]agent.BackendReplica, error) {
			if call == 1 {
				close(bootStarted)
				<-bootRelease
			}
			return nil, nil
		},
	}
	cfg := integrationElasticCfg(bind, advertise, pki)
	deps := integrationDeps(pki, nd, backend)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, cfg, config.Config{}, deps) }()

	waitUntil(t, 3*time.Second, func() bool {
		select {
		case <-bootStarted:
			return true
		default:
			return false
		}
	})

	client, err := agenttransport.NewClient(agenttransport.ClientConfig{
		BaseURL: advertise, ExpectedNodeURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.ClientCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	listScope := contract.CommandScope{
		NodeID: "n-test", ProjectID: "demo", VersionID: "v1", Generation: 1,
		LeaseExpiry: time.Now().UTC().Add(time.Hour), Nonce: "boot-block-list", Action: contract.ActionListReplicas,
	}
	_, err = client.ListReplicas(context.Background(), listScope)
	expectColdActivating(t, err)

	probeScope := contract.CommandScope{
		NodeID: "n-test", ProjectID: "demo", VersionID: "v1", ReplicaID: "rep-1", Generation: 1,
		LeaseExpiry: time.Now().UTC().Add(time.Hour), Nonce: "boot-block-probe", Action: contract.ActionProbeReplica,
	}
	_, err = client.ProbeReplica(context.Background(), probeScope)
	expectColdActivating(t, err)

	stopScope := contract.CommandScope{
		NodeID: "n-test", ProjectID: "demo", VersionID: "v1", ReplicaID: "rep-1", Generation: 1,
		LeaseExpiry: time.Now().UTC().Add(time.Hour), Nonce: "boot-block-stop", Action: contract.ActionStopReplica,
	}
	_, err = client.StopReplica(context.Background(), stopScope)
	expectColdActivating(t, err)

	drainScope := contract.CommandScope{
		NodeID: "n-test", ProjectID: "demo", VersionID: "v1", ReplicaID: "rep-1", Generation: 1,
		LeaseExpiry: time.Now().UTC().Add(time.Hour), Nonce: "boot-block-drain", Action: contract.ActionDrainReplica,
	}
	_, err = client.DrainReplica(context.Background(), drainScope, time.Now().UTC().Add(time.Hour))
	expectColdActivating(t, err)

	startScope := contract.CommandScope{
		NodeID: "n-test", ProjectID: "demo", VersionID: "v1", ReplicaID: "rep-1", Generation: 1,
		LeaseExpiry: time.Now().UTC().Add(time.Hour), Nonce: "boot-block-start", Action: contract.ActionStartReplica,
	}
	_, err = client.StartReplica(context.Background(), contract.StartReplicaSpec{Scope: startScope}, "idem-boot-block")
	expectColdActivating(t, err)

	if backend.startN.Load() != 0 {
		t.Fatalf("backend Start must not run during boot gate, calls=%d", backend.startN.Load())
	}
	if backend.probeN.Load() != 0 {
		t.Fatalf("backend Probe must not run during boot gate, calls=%d", backend.probeN.Load())
	}
	if backend.stopN.Load() != 0 {
		t.Fatalf("backend Stop must not run during boot gate, calls=%d", backend.stopN.Load())
	}
	if backend.drainN.Load() != 0 {
		t.Fatalf("backend Drain must not run during boot gate, calls=%d", backend.drainN.Load())
	}
	if backend.listN.Load() != 1 {
		t.Fatalf("paused HTTP List must not call backend; only boot reconcile List (got %d)", backend.listN.Load())
	}
	if nd.heartbeatCalls.Load() != 0 {
		t.Fatalf("heartbeat must not run before boot reconcile completes, calls=%d", nd.heartbeatCalls.Load())
	}

	close(bootRelease)
	waitUntil(t, 3*time.Second, func() bool { return nd.heartbeatCalls.Load() >= 1 })
	_, err = client.ListReplicas(context.Background(), listScope)
	if err != nil {
		t.Fatalf("list after boot: %v", err)
	}

	cancel()
	if err := waitRunDone(done, 5*time.Second); err != nil && !isOnlyContextCancellation(err) {
		t.Fatalf("shutdown: %v", err)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func expectColdActivating(t *testing.T, err error) {
	t.Helper()
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonColdActivating {
		t.Fatalf("expected cold_activating, got %v", err)
	}
}

func tryRecv(done <-chan error) (error, bool) {
	select {
	case err := <-done:
		return err, true
	default:
		return nil, false
	}
}

func waitRunDone(done <-chan error, timeout time.Duration) error {
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("run did not finish")
	}
}
