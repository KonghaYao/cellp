package serve

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/agentrun"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	"github.com/cellp/cellp/internal/elastic/scheduler"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestStandaloneAgentRunRegistersAndReleases(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeRemoteWireCertFiles(t, dir, "server", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	if err := os.WriteFile(denyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	agentBind := freeTCPAddr(t)
	agentURL := "https://" + agentBind
	allowPath := filepath.Join(dir, "allow.json")
	payload, _ := json.Marshal([]map[string]interface{}{
		{"node_id": "n1", "identity_uri": pki.NodeURI, "agent_base_url": agentURL, "allow_loopback": true},
	})
	if err := os.WriteFile(allowPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	ctrlBind := freeTCPAddr(t)
	remoteCfg := config.RemoteControlConfig{
		Enabled: true, BindAddr: ctrlBind, ControllerIdentityURI: pki.ControllerURI,
		ServerCertFile: certPath, ServerKeyFile: keyPath, ServerCAFile: caPath,
		CertificateDenylistFile: denyPath, NodeHeartbeatTTL: 30 * time.Second,
		MaxBodyBytes: 1 << 20, ReplayMaxEntries: 4096,
		NodeAllowlist: map[string]contract.NodeRegistrationBinding{
			"n1": {IdentityURI: pki.NodeURI, AgentBaseURL: agentURL, AllowLoopback: true},
		},
	}
	tlsMat, err := loadRemoteControlTLS(remoteCfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := registry.Open(filepath.Join(dir, "registry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	guardID := "standalone-agent-test"
	if err := store.TryAcquireControllerGuard(context.Background(), guardID, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseControllerGuard(context.Background(), guardID)
	guard := scheduler.NewRegistryGuard(store, guardID)

	ctx, cancel := context.WithCancel(context.Background())
	rt, err := startRemoteControl(ctx, remoteCfg, tlsMat, store, guard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shCancel()
		_ = rt.quiesce(shCtx)
	})

	controllerBase := "https://" + rt.server.Addr()
	writeAgentCertFiles(t, dir, pki)
	elasticCfg := mustStandaloneElasticConfig(t, dir, pki, agentBind, agentURL, controllerBase)

	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runDone <- agentrun.Run(runCtx, elasticCfg, config.Config{}, agentrun.Deps{
			LoadTLS: func(cfg config.ElasticConfig) (agentrun.TLSBundle, error) {
				client := agentTLSMaterials(pki)
				client.ServerName = cfg.ResolvedControllerTLSServerName()
				return agentrun.TLSBundle{
					Server: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool},
					Client: client,
				}, nil
			},
			NewBackend: func(*runtime.Manager) agent.LifecycleBackend {
				return fakeAgentBackend{}
			},
		})
	}()
	waitForNodeLease(t, store, pki, controllerBase, "n1", runDone)
	runCancel()
	wg.Wait()
	runErr := <-runDone
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		t.Fatalf("expected nil or context.Canceled from agent shutdown, got %v", runErr)
	}

	node, err := store.GetRuntimeNode(context.Background(), "n1")
	if err != nil {
		t.Fatal(err)
	}
	if node != nil && node.LeaseExpiry.After(time.Now().UTC()) {
		t.Fatal("expected lease released after shutdown")
	}
}

type fakeAgentBackend struct{}

func (fakeAgentBackend) ExpectedReplicaBucket(contract.CommandScope) (string, error) {
	return "s3://bucket/p/v", nil
}
func (fakeAgentBackend) Diagnose(context.Context, contract.StartReplicaSpec) error { return nil }
func (fakeAgentBackend) Start(context.Context, contract.StartReplicaSpec) (string, int, error) {
	return "127.0.0.1", 1, nil
}
func (fakeAgentBackend) Probe(context.Context, contract.CommandScope) (agent.BackendReplica, error) {
	return agent.BackendReplica{}, nil
}
func (fakeAgentBackend) Drain(context.Context, contract.CommandScope, time.Time) error { return nil }
func (fakeAgentBackend) Stop(context.Context, contract.CommandScope) error             { return nil }
func (fakeAgentBackend) List(context.Context) ([]agent.BackendReplica, error)          { return nil, nil }

func waitForNodeLease(t *testing.T, store *registry.SQLiteStore, pki remoteWirePKI, base, nodeID string, runDone <-chan error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastStatusErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("agent run failed: %v", err)
			}
			t.Fatal("agent run exited before lease observed")
		default:
		}
			node, err := store.GetRuntimeNode(context.Background(), nodeID)
			if err != nil {
				if registry.IsSQLiteBusy(err) {
					time.Sleep(registryBusyPollInterval)
					continue
				}
				t.Fatal(err)
			}
		if node != nil && node.LeaseExpiry.After(time.Now().UTC()) {
			return
		}
		nd, err := noderegtransport.NewClient(noderegtransport.ClientConfig{
			BaseURL: base, NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
			TLS: agentTLSMaterials(pki),
		})
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		st, err := nd.Status(context.Background(), contract.StatusNodeRequest{
			Scope: contract.NodeRegistrationScope{
				NodeID: nodeID, IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "probe",
			},
		})
		lastStatusErr = err
		if err == nil && st.Found && st.Generation > 0 && st.LeaseExpiry.After(now) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("node lease not observed (last status err=%v)", lastStatusErr)
}

func freeTCPAddr(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func mustStandaloneElasticConfig(t *testing.T, dir string, pki remoteWirePKI, bind, advertise, controllerBase string) config.ElasticConfig {
	t.Helper()
	env := map[string]string{
		"CELLP_ELASTIC_RUNTIME": "1", "CELLP_AGENT_NODE_ID": "n1",
		"CELLP_AGENT_BIND_ADDR": bind, "CELLP_AGENT_ADVERTISE_URL": advertise,
		"CELLP_AGENT_NODE_IDENTITY_URI": pki.NodeURI, "CELLP_AGENT_CONTROLLER_IDENTITY_URI": pki.ControllerURI,
		"CELLP_AGENT_ALLOWED_CONTROLLER_URIS": pki.ControllerURI,
		"CELLP_AGENT_SERVER_CERT_FILE": filepath.Join(dir, "agent-server.pem"),
		"CELLP_AGENT_SERVER_KEY_FILE":  filepath.Join(dir, "agent-server-key.pem"),
		"CELLP_AGENT_SERVER_CA_FILE": filepath.Join(dir, "agent-ca.pem"),
		"CELLP_AGENT_CLIENT_CERT_FILE": filepath.Join(dir, "agent-client.pem"),
		"CELLP_AGENT_CLIENT_KEY_FILE":  filepath.Join(dir, "agent-client-key.pem"),
		"CELLP_AGENT_CLIENT_CA_FILE":   filepath.Join(dir, "agent-ca.pem"),
		"CELLP_AGENT_TLS_SERVER_NAME":  "127.0.0.1",
		"CELLP_AGENT_CERT_DENYLIST_FILE": filepath.Join(dir, "denylist"),
		"CELLP_AGENT_CAPACITY_UNITS": "2", "CELLP_AGENT_ZONE": "z",
		"CELLP_AGENT_HEARTBEAT_TTL": "30s", "CELLP_AGENT_HEARTBEAT_INTERVAL": "10s",
		"CELLP_AGENT_RECONCILE_INTERVAL": "200ms", "CELLP_AGENT_MAX_BODY_BYTES": "1048576",
		"CELLP_AGENT_REPLAY_MAX_ENTRIES": "4096",
		"CELLP_AGENT_NODEREG_BASE_URL": controllerBase, "CELLP_AGENT_REGISTRY_RELAY_BASE_URL": controllerBase,
		"CELLP_AGENT_RELAY_SCOPE_TTL": "90s", "CELLP_AGENT_CONTROLLER_TLS_SERVER_NAME": "127.0.0.1",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := config.LoadStandaloneAgentConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeAgentCertFiles(t *testing.T, dir string, pki remoteWirePKI) {
	t.Helper()
	_, _ = writeRemoteWireCertFiles(t, dir, "agent-server", pki.NodeCert)
	_, _ = writeRemoteWireCertFiles(t, dir, "agent-client", pki.NodeCert)
	caPath := filepath.Join(dir, "agent-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pki.CACert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func agentTLSMaterials(pki remoteWirePKI) agenttransport.TLSMaterials {
	return agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"}
}
