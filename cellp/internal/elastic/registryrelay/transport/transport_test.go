package transport_test

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
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/elastic/registryrelay/agentstore"
	"github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/registry"
)

type testPKI struct {
	RootPool       *x509.CertPool
	ControllerCert tls.Certificate
	NodeCert       tls.Certificate
	ControllerURI  string
	NodeURI        string
}

func mustPKI(t *testing.T, nodeID string) testPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	ctrlURI := "spiffe://cellp/test/controller/ctrl-1"
	nodeURI := "spiffe://cellp/test/node/" + nodeID
	return testPKI{
		RootPool: pool, ControllerCert: mustLeaf(t, caCert, caKey, ctrlURI), NodeCert: mustLeaf(t, caCert, caKey, nodeURI),
		ControllerURI: ctrlURI, NodeURI: nodeURI,
	}
}

func mustLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, uri string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(uri)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		URIs:         []*url.URL{u},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

type guardOK struct{}

func (guardOK) EnsureActiveController(context.Context) error { return nil }

type relayEnv struct {
	store        *registry.SQLiteStore
	client       *transport.Client
	remote       *agentstore.Store
	relayHandler *registryrelay.Handler
}

// relayValidateFaultStore fails ValidateAgentAssignment only (list-by-node still succeeds).
type relayValidateFaultStore struct {
	*registry.SQLiteStore
	validateErr error
}

func (s *relayValidateFaultStore) ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error) {
	if s.validateErr != nil {
		return nil, s.validateErr
	}
	return s.SQLiteStore.ValidateAgentAssignment(ctx, scope, now)
}

type relayObserveBackend struct {
	mu        sync.Mutex
	inventory map[string]agent.BackendReplica
	stops     int
}

func newRelayObserveBackend() *relayObserveBackend {
	return &relayObserveBackend{inventory: map[string]agent.BackendReplica{}}
}

func (b *relayObserveBackend) ExpectedReplicaBucket(scope contract.CommandScope) (string, error) {
	return "s3://cellp-celld/" + scope.ProjectID + "/" + scope.VersionID, nil
}

func (b *relayObserveBackend) Diagnose(context.Context, contract.StartReplicaSpec) error { return nil }

func (b *relayObserveBackend) Start(_ context.Context, spec contract.StartReplicaSpec) (string, int, error) {
	item := agent.BackendReplica{
		ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID,
		Host: "127.0.0.1", Port: 9101, Healthy: true,
	}
	b.mu.Lock()
	b.inventory[item.ReplicaID] = item
	b.mu.Unlock()
	return item.Host, item.Port, nil
}

func (b *relayObserveBackend) Probe(_ context.Context, scope contract.CommandScope) (agent.BackendReplica, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.inventory[scope.ReplicaID]
	if !ok {
		return agent.BackendReplica{}, errors.New("not running")
	}
	return item, nil
}

func (b *relayObserveBackend) Drain(_ context.Context, scope contract.CommandScope, _ time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.inventory, scope.ReplicaID)
	b.stops++
	return nil
}

func (b *relayObserveBackend) Stop(_ context.Context, scope contract.CommandScope) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.inventory, scope.ReplicaID)
	b.stops++
	return nil
}

func (b *relayObserveBackend) List(context.Context) ([]agent.BackendReplica, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]agent.BackendReplica, 0, len(b.inventory))
	for _, item := range b.inventory {
		out = append(out, item)
	}
	return out, nil
}

func (b *relayObserveBackend) stopCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stops
}

func (b *relayObserveBackend) hasReplica(replicaID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.inventory[replicaID]
	return ok
}

func seedRelayControllerRegistry(t *testing.T, store *registry.SQLiteStore, nodeID, identityURI string) {
	t.Helper()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(2 * time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: nodeID, CapacityUnits: 2, Generation: 1, LeaseExpiry: exp,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: identityURI, Zone: "z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: nodeID,
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: exp,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedRelayReplicaReady(t *testing.T, store *registry.SQLiteStore, nodeID string) {
	t.Helper()
	ctx := context.Background()
	rep, err := store.GetRuntimeReplica(ctx, "r1")
	if err != nil || rep == nil || rep.ValidUntil == nil {
		t.Fatalf("replica: %+v err=%v", rep, err)
	}
	exp := *rep.ValidUntil
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: nodeID, Generation: 1,
		State: contract.ReplicaStarting, AssignmentValidUntil: &exp,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: nodeID, Generation: 1,
		State: contract.ReplicaReady, ListenHost: "127.0.0.1", ListenPort: 9101,
		EndpointState: contract.EndpointReady, AssignmentValidUntil: &exp, EndpointValidUntil: &exp,
	}); err != nil {
		t.Fatal(err)
	}
}

func newRelayLifecycleHandler(remote *agentstore.Store, backend agent.LifecycleBackend) *agent.Handler {
	return agent.NewLifecycleHandler(true, remote, remote, backend)
}

func waitRelayServerReady(t *testing.T, srv *transport.Server) {
	t.Helper()
	select {
	case <-srv.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("relay server not ready")
	}
}

func startRelayEnvWithHandler(t *testing.T, nodeID string, pki testPKI, h *registryrelay.Handler, store *registry.SQLiteStore) relayEnv {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	srv, err := transport.NewServer(transport.ServerConfig{
		BindAddr: addr, ControllerIdentityURI: pki.ControllerURI, AllowedNodeURIs: []string{pki.NodeURI},
		TLS: agenttransport.TLSMaterials{Cert: pki.ControllerCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	}, h)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Run(runCtx)
	}()
	waitRelayServerReady(t, srv)
	t.Cleanup(func() {
		cancel()
		_ = srv.Shutdown(context.Background())
		wg.Wait()
	})

	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL: "https://" + srv.Addr(), NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	remote := agentstore.NewStore(client, nodeID, time.Minute)
	return relayEnv{store: store, client: client, remote: remote, relayHandler: h}
}

func startRelayEnv(t *testing.T, nodeID string) relayEnv {
	t.Helper()
	pki := mustPKI(t, nodeID)
	store, err := registry.Open(t.TempDir() + "/relay.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedRelayControllerRegistry(t, store, nodeID, pki.NodeURI)

	h := &registryrelay.Handler{Store: store, Allowlist: map[string]string{nodeID: pki.NodeURI}, Guard: guardOK{}}
	return startRelayEnvWithHandler(t, nodeID, pki, h, store)
}

func startRelayEnvUnregistered(t *testing.T, nodeID string) relayEnv {
	t.Helper()
	pki := mustPKI(t, nodeID)
	store, err := registry.Open(t.TempDir() + "/relay-unreg.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	h := &registryrelay.Handler{Store: store, Allowlist: map[string]string{nodeID: pki.NodeURI}, Guard: guardOK{}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	srv, err := transport.NewServer(transport.ServerConfig{
		BindAddr: addr, ControllerIdentityURI: pki.ControllerURI, AllowedNodeURIs: []string{pki.NodeURI},
		TLS: agenttransport.TLSMaterials{Cert: pki.ControllerCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	}, h)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Run(runCtx)
	}()
	waitRelayServerReady(t, srv)
	t.Cleanup(func() {
		cancel()
		_ = srv.Shutdown(context.Background())
		wg.Wait()
	})
	client, err := transport.NewClient(transport.ClientConfig{
		BaseURL: "https://" + srv.Addr(), NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	remote := agentstore.NewStore(client, nodeID, time.Minute)
	return relayEnv{store: store, client: client, remote: remote, relayHandler: h}
}

func TestRelayTLSRoundTripCommandClaim(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	node, err := env.remote.GetRuntimeNode(ctx, "n1")
	if err != nil || node == nil {
		t.Fatalf("get node: %+v err=%v", node, err)
	}
	reps, err := env.remote.ListRuntimeReplicasByNode(ctx, "n1")
	if err != nil || len(reps) != 1 {
		t.Fatalf("list: %+v err=%v", reps, err)
	}
	cmd := registry.AgentCommand{
		IdempotencyKey: "k1", Action: contract.ActionStartReplica, NodeID: "n1",
		ProjectID: "demo", VersionID: "v1", ReplicaID: "r1", Generation: 1,
	}
	claim, err := env.remote.ClaimAgentCommand(ctx, cmd)
	if err != nil || !claim.Claimed {
		t.Fatalf("claim: %+v err=%v", claim, err)
	}
}

func TestRelayCrossNodeScopeDenied(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	scope := env.client.NewRelayScope("n2", time.Minute)
	_, err := env.client.GetRuntimeNode(ctx, scope)
	if err == nil {
		t.Fatal("expected cross-node deny")
	}
}

func TestRelayReplayRejected(t *testing.T) {
	env := startRelayEnv(t, "n1")
	ctx := context.Background()
	scope := env.client.NewRelayScope("n1", time.Minute)
	if _, err := env.client.GetRuntimeNode(ctx, scope); err != nil {
		t.Fatal(err)
	}
	_, err := env.client.GetRuntimeNode(ctx, scope)
	if err == nil {
		t.Fatal("expected replay rejection")
	}
	var relay *registryrelay.RelayError
	if !errors.As(err, &relay) || relay.Reason != contract.ReasonReplayRejected {
		t.Fatalf("expected replay rejected, got %v", err)
	}
}

func TestRemoteStoreRegistryUnavailable(t *testing.T) {
	remote := agentstore.NewStore(nil, "n1", time.Minute)
	_, err := remote.GetRuntimeNode(context.Background(), "n1")
	if !errors.Is(err, registryrelay.ErrRegistryUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}
