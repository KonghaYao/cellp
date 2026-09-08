package transport_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/scheduler"
)

func startPausedTestEnv(t *testing.T) *testEnv {
	env := startTestEnv(t, true, 64)
	env.srv.SetAccepting(false)
	return env
}

func TestDefaultServerAcceptsLifecycleCommands(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx := context.Background()
	scope := testScope(contract.ActionListReplicas, "default-accepts")
	_, err := env.client.ListReplicas(ctx, scope)
	if err != nil {
		t.Fatalf("default server should accept: %v", err)
	}
}

func TestPausedServerRejectsAfterAuthWithoutConsumingReplay(t *testing.T) {
	env := startPausedTestEnv(t)
	ctx := context.Background()

	scope := testScope(contract.ActionProbeReplica, "paused-probe-nonce")
	_, err := env.client.ProbeReplica(ctx, scope)
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonColdActivating {
		t.Fatalf("expected cold_activating, got %v", err)
	}

	_, err = env.client.ProbeReplica(ctx, scope)
	if errors.As(err, &cmd) && cmd.Reason == contract.ReasonReplayRejected {
		t.Fatal("paused rejection must not consume replay nonce")
	}
}

func TestPausedServerAuthFailureBeforeAcceptingGate(t *testing.T) {
	env := startPausedTestEnv(t)
	pki := env.pki
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{pki.ServerCert},
		RootCAs:      pki.RootPool,
		ServerName:   "127.0.0.1",
	}
	hc := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   3 * time.Second,
	}
	req, err := http.NewRequest(http.MethodPost, env.base+"/v1/internal/elastic-agent/list", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 before accepting gate, got %d", resp.StatusCode)
	}
}

func TestSetAcceptingAfterPausedAllowsSameNonce(t *testing.T) {
	env := startPausedTestEnv(t)
	ctx := context.Background()
	scope := testScope(contract.ActionListReplicas, "accepting-toggle-nonce")

	_, err := env.client.ListReplicas(ctx, scope)
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonColdActivating {
		t.Fatalf("paused: %v", err)
	}

	env.srv.SetAccepting(true)
	_, err = env.client.ListReplicas(ctx, scope)
	if err != nil {
		t.Fatalf("after accepting: %v", err)
	}
}

func TestColdActivatingWireIsSchedulerRetryable(t *testing.T) {
	err := transport.CommandErrorFromResponse(503, []byte(`{"reason":"cold_activating"}`))
	if !scheduler.IsTransientAgentOrRegistry(err) {
		t.Fatalf("cold_activating should be retryable: %v", err)
	}
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonColdActivating {
		t.Fatalf("decode: %v", err)
	}
}

func TestShutdownClearsAccepting(t *testing.T) {
	env := startTestEnv(t, true, 8)
	if !transport.HookAccepting(env.srv) {
		t.Fatal("server should accept by default")
	}
	if err := env.srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !transport.HookClosed(env.srv) {
		t.Fatal("server must be closed after Shutdown")
	}
	if transport.HookAccepting(env.srv) {
		t.Fatal("Shutdown must clear accepting without manual SetAccepting(false)")
	}
}

func TestSetAcceptingTrueIgnoredAfterShutdown(t *testing.T) {
	env := startTestEnv(t, true, 8)
	if err := env.srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !transport.HookClosed(env.srv) {
		t.Fatal("server must be closed after Shutdown")
	}
	if transport.HookAccepting(env.srv) {
		t.Fatal("accepting must be false when shutdown completes")
	}
	env.srv.SetAccepting(true)
	if transport.HookAccepting(env.srv) {
		t.Fatal("SetAccepting(true) after shutdown must not re-enable accepting")
	}
	if !transport.HookClosed(env.srv) {
		t.Fatal("closed state must remain after ignored SetAccepting(true)")
	}
}

func TestSetAcceptingTrueIgnoredAfterShutdownBeforeRun(t *testing.T) {
	pki := mustTestPKI(t)
	mem := newMemStores()
	h := agent.NewHandler(true, mem, mem)
	cfg := transport.ServerConfig{
		Enabled:               true,
		NodeID:                "node-a",
		BindAddr:              "127.0.0.1:0",
		NodeIdentityURI:       pki.NodeURI,
		AllowedControllerURIs: []string{pki.ControllerURI},
		TLS: transport.TLSMaterials{
			Cert: pki.ServerCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1",
		},
		ReplayMaxEntries: 8,
	}
	srv, err := transport.NewServer(cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	if !transport.HookAccepting(srv) {
		t.Fatal("NewServer default accepting")
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv.SetAccepting(true)
	if transport.HookAccepting(srv) {
		t.Fatal("SetAccepting(true) after shutdown-before-run must not enable")
	}
	if !transport.HookClosed(srv) {
		t.Fatal("expected closed")
	}
}

func TestSetAcceptingShutdownConcurrentNeverReopens(t *testing.T) {
	env := startTestEnv(t, true, 8)
	const workers = 8
	const rounds = 200
	start := make(chan struct{})
	var shutdownDone atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < rounds; j++ {
				env.srv.SetAccepting(true)
				env.srv.SetAccepting(false)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if err := env.srv.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
		shutdownDone.Store(true)
	}()
	close(start)
	wg.Wait()
	if !shutdownDone.Load() {
		t.Fatal("shutdown goroutine did not complete")
	}
	if !transport.HookClosed(env.srv) {
		t.Fatal("server must be closed after Shutdown returns")
	}
	if transport.HookAccepting(env.srv) {
		t.Fatal("accepting must be false after Shutdown returns")
	}
	for i := 0; i < workers; i++ {
		env.srv.SetAccepting(true)
	}
	if transport.HookAccepting(env.srv) {
		t.Fatal("concurrent SetAccepting(true) after shutdown must not reopen accepting")
	}
}
