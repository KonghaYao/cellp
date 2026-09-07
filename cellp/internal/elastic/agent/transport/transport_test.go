package transport_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
)

type memStores struct {
	nodes map[string]contract.RuntimeNode
	reps  map[string]contract.RuntimeReplica
}

func newMemStores() *memStores {
	return &memStores{nodes: map[string]contract.RuntimeNode{}, reps: map[string]contract.RuntimeReplica{}}
}

func (m *memStores) UpsertRuntimeNode(_ context.Context, node contract.RuntimeNode) error {
	m.nodes[node.NodeID] = node
	return nil
}

func (m *memStores) GetRuntimeNode(_ context.Context, nodeID string) (*contract.RuntimeNode, error) {
	n, ok := m.nodes[nodeID]
	if !ok {
		return nil, nil
	}
	return &n, nil
}

func (m *memStores) ListRuntimeNodes(context.Context) ([]contract.RuntimeNode, error) {
	out := make([]contract.RuntimeNode, 0, len(m.nodes))
	for _, n := range m.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (m *memStores) UpsertRuntimeReplica(_ context.Context, rep contract.RuntimeReplica) error {
	m.reps[rep.ReplicaID] = rep
	return nil
}

func (m *memStores) ListRuntimeReplicas(_ context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error) {
	var out []contract.RuntimeReplica
	for _, r := range m.reps {
		if r.ProjectID == projectID && r.VersionID == versionID {
			out = append(out, r)
		}
	}
	return out, nil
}

type testEnv struct {
	pki     testPKI
	mem     *memStores
	handler *agent.Handler
	srv     *transport.Server
	client  *transport.Client
	base    string
}

func startTestEnv(t *testing.T, enabled bool, replayMax int) *testEnv {
	t.Helper()
	pki := mustTestPKI(t)
	mem := newMemStores()
	lease := time.Now().UTC().Add(2 * time.Hour)
	h := agent.NewHandler(enabled, mem, mem)
	_ = mem.UpsertRuntimeNode(context.Background(), contract.RuntimeNode{
		NodeID:        "node-a",
		CapacityUnits: 4,
		Generation:    1,
		LeaseExpiry:   lease,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cfg := transport.ServerConfig{
		Enabled:               true,
		NodeID:                "node-a",
		BindAddr:              addr,
		NodeIdentityURI:       pki.NodeURI,
		AllowedControllerURIs: []string{pki.ControllerURI},
		TLS: transport.TLSMaterials{
			Cert:       pki.ServerCert,
			RootCAs:    pki.RootPool,
			ServerName: "127.0.0.1",
		},
		ReplayMaxEntries: replayMax,
	}
	srv, err := transport.NewServer(cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	base := "https://" + srv.Addr()

	cCfg := transport.ClientConfig{
		BaseURL:               base,
		ExpectedNodeURI:       pki.NodeURI,
		ControllerIdentityURI: pki.ControllerURI,
		TLS: transport.TLSMaterials{
			Cert:       pki.ClientCert,
			RootCAs:    pki.RootPool,
			ServerName: "127.0.0.1",
		},
	}
	client, err := transport.NewClient(cCfg)
	if err != nil {
		cancel()
		_ = srv.Shutdown(context.Background())
		t.Fatal(err)
	}
	env := &testEnv{pki: pki, mem: mem, handler: h, srv: srv, client: client, base: base}
	t.Cleanup(func() {
		cancel()
		_ = srv.Shutdown(context.Background())
		wg.Wait()
	})
	return env
}

func testScope(action contract.LifecycleAction, nonce string) contract.CommandScope {
	return contract.CommandScope{
		NodeID:      "node-a",
		ProjectID:   "demo",
		VersionID:   "v1",
		ReplicaID:   "rep-1",
		Generation:  1,
		LeaseExpiry: time.Now().UTC().Add(time.Hour),
		Nonce:       nonce,
		Action:      action,
	}
}

func TestMTLSRoundtripLifecycle(t *testing.T) {
	env := startTestEnv(t, true, 1024)
	ctx := context.Background()
	scope := testScope(contract.ActionStartReplica, "nonce-start-1")
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: "b-demo-v1", SecretRefs: []contract.SecretRef{{Name: "ref-a"}}}
	rep, err := env.client.StartReplica(ctx, spec, "idem-1")
	if err != nil {
		t.Fatal(err)
	}
	if rep.State != contract.ReplicaStarting {
		t.Fatalf("start: %s", rep.State)
	}
	probeScope := testScope(contract.ActionProbeReplica, "nonce-probe-1")
	pr, err := env.client.ProbeReplica(ctx, probeScope)
	if err != nil {
		t.Fatal(err)
	}
	if pr.State != contract.ReplicaStarting {
		t.Fatalf("probe: %s", pr.State)
	}
	drainScope := testScope(contract.ActionDrainReplica, "nonce-drain-1")
	drained, err := env.client.DrainReplica(ctx, drainScope, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if drained.State != contract.ReplicaDraining {
		t.Fatalf("drain: %s", drained.State)
	}
	listScope := testScope(contract.ActionListReplicas, "nonce-list-1")
	listScope.ReplicaID = ""
	reps, err := env.client.ListReplicas(ctx, listScope)
	if err != nil || len(reps) != 1 {
		t.Fatalf("list: %v len=%d", err, len(reps))
	}
	stopScope := testScope(contract.ActionStopReplica, "nonce-stop-1")
	stopped, err := env.client.StopReplica(ctx, stopScope)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != contract.ReplicaStopped {
		t.Fatalf("stop: %s", stopped.State)
	}
}

func TestBindRejectsPublicAndWildcard(t *testing.T) {
	if err := transport.ValidateInternalBindAddr("0.0.0.0:8443"); err == nil {
		t.Fatal("wildcard")
	}
	if err := transport.ValidateInternalBindAddr("8.8.8.8:8443"); err == nil {
		t.Fatal("public")
	}
	if err := transport.ValidateInternalBindAddr("localhost:8443"); err == nil {
		t.Fatal("hostname")
	}
	if err := transport.ValidateInternalBindAddr("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
}

func TestReplaySameNonceRejected(t *testing.T) {
	env := startTestEnv(t, true, 1024)
	ctx := context.Background()
	startScope := testScope(contract.ActionStartReplica, "nonce-start-replay")
	_, err := env.client.StartReplica(ctx, contract.StartReplicaSpec{
		Scope: startScope, Bucket: "b-demo-v1",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	scope := testScope(contract.ActionProbeReplica, "fixed-nonce")
	_, err = env.client.ProbeReplica(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.client.ProbeReplica(ctx, scope)
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonReplayRejected {
		t.Fatalf("replay: %v", err)
	}
}

func TestConcurrentReplaySingleWinner(t *testing.T) {
	env := startTestEnv(t, true, 1024)
	ctx := context.Background()
	startScope := testScope(contract.ActionStartReplica, "nonce-start-race")
	_, err := env.client.StartReplica(ctx, contract.StartReplicaSpec{
		Scope: startScope, Bucket: "b-demo-v1",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	scope := testScope(contract.ActionProbeReplica, "race-nonce")
	var okCount atomic.Int32
	var replayCount atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := env.client.ProbeReplica(ctx, scope)
			var cmd *agent.CommandError
			if err == nil {
				okCount.Add(1)
				return
			}
			if errors.As(err, &cmd) && cmd.Reason == contract.ReasonReplayRejected {
				replayCount.Add(1)
				return
			}
			t.Errorf("unexpected: %v", err)
		}()
	}
	wg.Wait()
	if okCount.Load() != 1 {
		t.Fatalf("winners: %d", okCount.Load())
	}
	if replayCount.Load() != 15 {
		t.Fatalf("replays: %d", replayCount.Load())
	}
}

func TestReplayCacheCapacityFailClosed(t *testing.T) {
	cache := transport.NewReplayCache(2)
	exp := time.Now().Add(time.Hour)
	_, err := cache.Consume("a", exp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cache.Consume("b", exp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cache.Consume("c", exp)
	if err == nil {
		t.Fatal("expected capacity fail-closed")
	}
}

func TestReplayExpiryAllowsReuse(t *testing.T) {
	cache := transport.NewReplayCache(8)
	short := time.Now().Add(10 * time.Millisecond)
	replay, err := cache.Consume("k", short)
	if err != nil || replay {
		t.Fatalf("first: replay=%v err=%v", replay, err)
	}
	time.Sleep(20 * time.Millisecond)
	replay, err = cache.Consume("k", time.Now().Add(time.Hour))
	if err != nil || replay {
		t.Fatalf("reuse: replay=%v err=%v", replay, err)
	}
}

func TestElasticDisabledOnWire(t *testing.T) {
	env := startTestEnv(t, false, 64)
	ctx := context.Background()
	scope := testScope(contract.ActionStartReplica, "n-disabled")
	_, err := env.client.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "b"}, "")
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonElasticDisabled {
		t.Fatalf("got %v", err)
	}
}

func TestWireErrorNoSensitiveBody(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx := context.Background()
	scope := testScope(contract.ActionStartReplica, "n-scope")
	scope.NodeID = "wrong-node"
	_, err := env.client.StartReplica(ctx, contract.StartReplicaSpec{Scope: scope, Bucket: "b"}, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "wrong-node") {
		t.Fatal("leaked scope in error")
	}
}

func TestHTTPSOnlyBaseURL(t *testing.T) {
	if _, err := transport.NewClient(transport.ClientConfig{
		BaseURL:               "http://127.0.0.1:1",
		ExpectedNodeURI:       "spiffe://cellp/test/node/node-a",
		ControllerIdentityURI: "spiffe://cellp/test/controller/ctrl-1",
		TLS: transport.TLSMaterials{
			Cert:       mustTestPKI(t).ClientCert,
			RootCAs:    mustTestPKI(t).RootPool,
			ServerName: "127.0.0.1",
		},
	}); err == nil {
		t.Fatal("http base url")
	}
}

func TestJSONStrictRejects(t *testing.T) {
	env := startTestEnv(t, true, 64)
	tests := []struct {
		name string
		body string
		ct   string
	}{
		{"unknown field", `{"scope":{"node_id":"node-a","project_id":"demo","version_id":"v1","generation":1,"lease_expiry":"2030-01-01T00:00:00Z","nonce":"n","action":"probe_replica","replica_id":"rep-1"},"extra":true}`, "application/json"},
		{"trailing", `{"scope":{"node_id":"node-a","project_id":"demo","version_id":"v1","generation":1,"lease_expiry":"2030-01-01T00:00:00Z","nonce":"n","action":"probe_replica","replica_id":"rep-1"}}{}`, "application/json"},
		{"wrong ct", `{"scope":{"node_id":"node-a"}}`, "text/plain"},
	}
	for _, tc := range tests {
		status, reason := postRaw(t, env, tc.body, tc.ct)
		if status == 200 {
			t.Fatalf("%s: unexpected ok", tc.name)
		}
		if reason == "" {
			t.Fatalf("%s: missing reason", tc.name)
		}
		if strings.Contains(reason, "demo") || strings.Contains(reason, "node-a") {
			t.Fatalf("leaked reason: %s", reason)
		}
	}
}

func postRaw(t *testing.T, env *testEnv, body string, ct string) (int, string) {
	t.Helper()
	pki := env.pki
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{pki.ClientCert},
		RootCAs:      pki.RootPool,
		ServerName:   "127.0.0.1",
	}
	tr := &http.Transport{TLSClientConfig: tlsCfg}
	hc := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	u := env.base + "/v1/internal/elastic-agent/probe"
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", ct)
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var we struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(data, &we)
	return resp.StatusCode, we.Reason
}

func TestControllerURIAmbiguousRejected(t *testing.T) {
	pki := mustTestPKI(t)
	ambig := mustLeafCert(t, pki.CA, pki.CAKey, pki.ControllerURI, []string{pki.ControllerURI2}, time.Now().Add(time.Hour))
	_, err := transport.MatchPeerURIPrincipal(ambig.Leaf, []string{pki.ControllerURI, pki.ControllerURI2})
	if err == nil {
		t.Fatal("ambiguous uri should fail")
	}
}

func TestServerNodeURIMismatchRejected(t *testing.T) {
	pki := mustTestPKI(t)
	badServer := mustLeafCert(t, pki.CA, pki.CAKey, "spiffe://cellp/test/node/other", nil, time.Now().Add(time.Hour))
	mem := newMemStores()
	h := agent.NewHandler(true, mem, mem)
	cfg := transport.ServerConfig{
		Enabled:               true,
		NodeID:                "node-a",
		BindAddr:              "127.0.0.1:0",
		NodeIdentityURI:       pki.NodeURI,
		AllowedControllerURIs: []string{pki.ControllerURI},
		TLS:                   transport.TLSMaterials{Cert: badServer, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	}
	_, err := transport.NewServer(cfg, h)
	if err == nil {
		t.Fatal("expected node uri mismatch at startup")
	}
}

func TestMTLSWrongCAFails(t *testing.T) {
	env := startTestEnv(t, true, 64)
	other := mustTestPKI(t)
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{other.ClientCert},
		RootCAs:      other.RootPool,
		ServerName:   "127.0.0.1",
	}
	hc := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   3 * time.Second,
	}
	req, _ := http.NewRequest(http.MethodPost, env.base+"/v1/internal/elastic-agent/probe", bytes.NewBufferString(`{}`))
	resp, err := hc.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected tls handshake failure")
	}
}

func TestActionScopeMismatch(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx := context.Background()
	scope := testScope(contract.ActionStopReplica, "nonce-action")
	_, err := env.client.ProbeReplica(ctx, scope)
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) {
		t.Fatalf("%v", err)
	}
}

func TestLeaseExpiredScope(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx := context.Background()
	scope := testScope(contract.ActionProbeReplica, "nonce-lease")
	scope.LeaseExpiry = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := env.client.ProbeReplica(ctx, scope)
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonGenerationStale {
		t.Fatalf("%v", err)
	}
}

func TestClientCommandErrorReason(t *testing.T) {
	err := transport.CommandErrorFromResponse(401, []byte(`{"reason":"auth_failed"}`))
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonAuthFailed {
		t.Fatal(err)
	}
}

func TestExpiredClientCertHandshakeFails(t *testing.T) {
	env := startTestEnv(t, true, 64)
	pki := mustTestPKI(t)
	expired := mustLeafCert(t, pki.CA, pki.CAKey, pki.ControllerURI, nil, time.Now().Add(-time.Hour))
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{expired},
		RootCAs:      pki.RootPool,
		ServerName:   "127.0.0.1",
	}
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: 3 * time.Second}
	req, _ := http.NewRequest(http.MethodPost, env.base+"/v1/internal/elastic-agent/probe", bytes.NewBufferString(`{}`))
	_, err := hc.Do(req)
	if err == nil {
		t.Fatal("expected expired cert handshake failure")
	}
}

func TestNoClientCertHandshakeFails(t *testing.T) {
	env := startTestEnv(t, true, 64)
	pki := env.pki
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    pki.RootPool,
		ServerName: "127.0.0.1",
	}
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: 3 * time.Second}
	req, _ := http.NewRequest(http.MethodPost, env.base+"/v1/internal/elastic-agent/probe", bytes.NewBufferString(`{}`))
	_, err := hc.Do(req)
	if err == nil {
		t.Fatal("expected missing client cert failure")
	}
}

func TestRequestBodyTooLarge(t *testing.T) {
	env := startTestEnv(t, true, 64)
	pki := env.pki
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{pki.ClientCert},
		RootCAs:      pki.RootPool,
		ServerName:   "127.0.0.1",
	}
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: 15 * time.Second}
	u := env.base + "/v1/internal/elastic-agent/probe"
	pad := strings.Repeat("a", 2*1024*1024)
	body := fmt.Sprintf(`{"scope":{"node_id":"node-a","project_id":"demo","version_id":"v1","generation":1,"lease_expiry":"2030-01-01T00:00:00Z","nonce":"%s","action":"probe_replica","replica_id":"rep-1"}}`, pad)
	req, _ := http.NewRequest(http.MethodPost, u, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var we struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&we)
	if we.Reason != string(contract.ReasonRequestTooLarge) {
		t.Fatalf("reason %q", we.Reason)
	}
}

func TestClientRejectsRedirect(t *testing.T) {
	pki := mustTestPKI(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{pki.ServerCert},
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://evil.example/", http.StatusFound)
		}),
	}
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	t.Cleanup(func() {
		_ = srv.Close()
	})
	base := "https://" + ln.Addr().String()
	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL:               base,
		ExpectedNodeURI:       pki.NodeURI,
		ControllerIdentityURI: pki.ControllerURI,
		TLS: transport.TLSMaterials{
			Cert:       pki.ClientCert,
			RootCAs:    pki.RootPool,
			ServerName: "127.0.0.1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := testScope(contract.ActionProbeReplica, "redirect-nonce")
	_, err = client.ProbeReplica(context.Background(), scope)
	if err == nil {
		t.Fatal("expected redirect error")
	}
}

func TestControllerURINotInAllowlistFails(t *testing.T) {
	pki := mustTestPKI(t)
	other := mustLeafCert(t, pki.CA, pki.CAKey, "spiffe://cellp/test/controller/other", nil, time.Now().Add(time.Hour))
	_, err := transport.MatchPeerURIPrincipal(other.Leaf, []string{pki.ControllerURI})
	if err == nil {
		t.Fatal("expected no matching uri")
	}
}

func TestClientTransportDisablesProxy(t *testing.T) {
	env := startTestEnv(t, true, 64)
	tr := env.client.ClientHTTPTransport()
	if tr == nil || tr.Proxy != nil {
		t.Fatal("expected internal client transport without environment proxy")
	}
}

func TestClientUnknownWireReasonNormalized(t *testing.T) {
	err := transport.CommandErrorFromResponse(500, []byte(`{"reason":"internal_sql_error_detail"}`))
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonAuthFailed {
		t.Fatalf("got %v", err)
	}
}

func TestClientRejectsOversizeResponseBody(t *testing.T) {
	pki := mustTestPKI(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pki.ServerCert}}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"replica_id":"r","state":"starting","generation":1}}` + strings.Repeat(" ", 2*1024*1024)))
		}),
	}
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	t.Cleanup(func() { _ = srv.Close() })
	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL:               "https://" + ln.Addr().String(),
		ExpectedNodeURI:       pki.NodeURI,
		ControllerIdentityURI: pki.ControllerURI,
		TLS:                   transport.TLSMaterials{Cert: pki.ClientCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
		MaxBodyBytes:          512,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ProbeReplica(context.Background(), testScope(contract.ActionProbeReplica, "oversize-body"))
	var cmd *agent.CommandError
	if !errors.As(err, &cmd) || cmd.Reason != contract.ReasonRequestTooLarge {
		t.Fatalf("got %v", err)
	}
}

func TestClientRejectsTrailingJSONOnSuccess(t *testing.T) {
	pki := mustTestPKI(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pki.ServerCert}}
	body := `{"result":{"replica_id":"r","state":"starting","generation":1}}{}`
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}),
	}
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	t.Cleanup(func() { _ = srv.Close() })
	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL:               "https://" + ln.Addr().String(),
		ExpectedNodeURI:       pki.NodeURI,
		ControllerIdentityURI: pki.ControllerURI,
		TLS:                   transport.TLSMaterials{Cert: pki.ClientCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ProbeReplica(context.Background(), testScope(contract.ActionProbeReplica, "trail-json"))
	if err == nil {
		t.Fatal("expected trailing json rejection")
	}
}

func TestDrainDeadlineInvalidRejected(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx := context.Background()
	startScope := testScope(contract.ActionStartReplica, "nonce-start-drain")
	_, err := env.client.StartReplica(ctx, contract.StartReplicaSpec{Scope: startScope, Bucket: "b-demo-v1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	scope := testScope(contract.ActionDrainReplica, "nonce-drain-bad")
	body := fmt.Sprintf(`{"scope":{"node_id":"node-a","project_id":"demo","version_id":"v1","generation":1,"lease_expiry":"%s","nonce":"nonce-drain-bad","action":"drain_replica","replica_id":"rep-1"},"deadline":"not-rfc3339"}`, scope.LeaseExpiry.UTC().Format(time.RFC3339))
	status, reason := postRawDrain(t, env, body)
	if status == 200 || reason != string(contract.ReasonAuthFailed) {
		t.Fatalf("status=%d reason=%q", status, reason)
	}
}

func TestDrainDeadlinePastRejected(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx := context.Background()
	startScope := testScope(contract.ActionStartReplica, "nonce-start-drain2")
	_, err := env.client.StartReplica(ctx, contract.StartReplicaSpec{Scope: startScope, Bucket: "b-demo-v1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	scope := testScope(contract.ActionDrainReplica, "nonce-drain-past")
	body := fmt.Sprintf(`{"scope":{"node_id":"node-a","project_id":"demo","version_id":"v1","generation":1,"lease_expiry":"%s","nonce":"nonce-drain-past","action":"drain_replica","replica_id":"rep-1"},"deadline":"2020-01-01T00:00:00Z"}`, scope.LeaseExpiry.UTC().Format(time.RFC3339))
	status, reason := postRawDrain(t, env, body)
	if status == 200 || reason != string(contract.ReasonAuthFailed) {
		t.Fatalf("status=%d reason=%q", status, reason)
	}
}

func postRawDrain(t *testing.T, env *testEnv, body string) (int, string) {
	t.Helper()
	pki := env.pki
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{pki.ClientCert},
		RootCAs:      pki.RootPool,
		ServerName:   "127.0.0.1",
	}
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, env.base+"/v1/internal/elastic-agent/drain", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var we struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(data, &we)
	return resp.StatusCode, we.Reason
}

func TestNewServerNilHandlerRejected(t *testing.T) {
	pki := mustTestPKI(t)
	cfg := transport.ServerConfig{
		Enabled: true, NodeID: "node-a", BindAddr: "127.0.0.1:0",
		NodeIdentityURI: pki.NodeURI, AllowedControllerURIs: []string{pki.ControllerURI},
		TLS: transport.TLSMaterials{Cert: pki.ServerCert, RootCAs: pki.RootPool},
	}
	_, err := transport.NewServer(cfg, nil)
	if err == nil {
		t.Fatal("expected nil handler error")
	}
}

func TestServerDoubleRunRejected(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = env.srv.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)
	err := env.srv.Run(context.Background())
	if err == nil {
		t.Fatal("expected already running error")
	}
	cancel()
	wg.Wait()
}

func TestDuplicateControllerURIInAllowlistRejected(t *testing.T) {
	pki := mustTestPKI(t)
	mem := newMemStores()
	h := agent.NewHandler(true, mem, mem)
	cfg := transport.ServerConfig{
		Enabled: true, NodeID: "node-a", BindAddr: "127.0.0.1:0",
		NodeIdentityURI:       pki.NodeURI,
		AllowedControllerURIs: []string{pki.ControllerURI, pki.ControllerURI},
		TLS:                   transport.TLSMaterials{Cert: pki.ServerCert, RootCAs: pki.RootPool},
	}
	_, err := transport.NewServer(cfg, h)
	if err == nil {
		t.Fatal("expected duplicate uri error")
	}
}

func TestInvalidIdentityURIRejected(t *testing.T) {
	pki := mustTestPKI(t)
	mem := newMemStores()
	h := agent.NewHandler(true, mem, mem)
	cfg := transport.ServerConfig{
		Enabled: true, NodeID: "node-a", BindAddr: "127.0.0.1:0",
		NodeIdentityURI:       "http://not-spiffe/node",
		AllowedControllerURIs: []string{pki.ControllerURI},
		TLS:                   transport.TLSMaterials{Cert: pki.ServerCert, RootCAs: pki.RootPool},
	}
	_, err := transport.NewServer(cfg, h)
	if err == nil {
		t.Fatal("expected invalid identity uri")
	}
}

func TestExpiredLeafRejectedAtConfig(t *testing.T) {
	pki := mustTestPKI(t)
	expired := mustLeafCert(t, pki.CA, pki.CAKey, pki.ControllerURI, nil, time.Now().Add(-time.Hour))
	_, err := transport.NewClient(transport.ClientConfig{
		BaseURL:               "https://127.0.0.1:1",
		ExpectedNodeURI:       pki.NodeURI,
		ControllerIdentityURI: pki.ControllerURI,
		TLS: transport.TLSMaterials{
			Cert: expired, RootCAs: pki.RootPool, ServerName: "127.0.0.1",
		},
	})
	if err == nil {
		t.Fatal("expected expired cert rejection at config")
	}
}

func TestReplayNotConsumedOnInvalidScope(t *testing.T) {
	env := startTestEnv(t, true, 64)
	ctx := context.Background()
	scope := testScope(contract.ActionProbeReplica, "shared-bad-node")
	scope.NodeID = "wrong-node"
	_, err := env.client.ProbeReplica(ctx, scope)
	if err == nil {
		t.Fatal("expected auth error")
	}
	_, err = env.client.ProbeReplica(ctx, scope)
	var cmd *agent.CommandError
	if errors.As(err, &cmd) && cmd.Reason == contract.ReasonReplayRejected {
		t.Fatal("replay consumed before valid scope")
	}
}

func TestVerifyPeerConnectionHook(t *testing.T) {
	pki := mustTestPKI(t)
	mem := newMemStores()
	h := agent.NewHandler(true, mem, mem)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	cfg := transport.ServerConfig{
		Enabled: true, NodeID: "node-a", BindAddr: addr,
		NodeIdentityURI: pki.NodeURI, AllowedControllerURIs: []string{pki.ControllerURI},
		TLS: transport.TLSMaterials{
			Cert: pki.ServerCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1",
			VerifyPeerConnection: func(cs tls.ConnectionState) error {
				return errors.New("revoked")
			},
		},
	}
	srv, err := transport.NewServer(cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	t.Cleanup(func() {
		cancel()
		_ = srv.Shutdown(context.Background())
	})
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pki.ClientCert},
		RootCAs: pki.RootPool, ServerName: "127.0.0.1",
	}
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: 3 * time.Second}
	req, _ := http.NewRequest(http.MethodPost, "https://"+srv.Addr()+"/v1/internal/elastic-agent/probe", bytes.NewBufferString(`{}`))
	_, err = hc.Do(req)
	if err == nil || !strings.Contains(err.Error(), "bad certificate") {
		t.Fatalf("revoked peer should fail handshake: %v", err)
	}
}
