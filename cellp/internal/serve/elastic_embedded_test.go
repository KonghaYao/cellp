package serve

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/cellp/cellp/internal/elastic/scheduler"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestValidateServeElasticBeforeListener(t *testing.T) {
	elastic := config.ElasticConfig{Enabled: true, AgentEmbedded: false}
	if err := config.ValidateServeElastic(elastic, config.RemoteControlConfig{}); err == nil {
		t.Fatal("expected fail before listener wiring")
	}
}

func TestLoadElasticTLSRemoteOnlySkipsServerMaterial(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	clientCert, clientKey := writeRemoteWireCertFiles(t, dir, "ctrl-client", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	if err := os.WriteFile(denyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.ElasticConfig{
		Enabled: true, AgentEmbedded: false, ControllerIdentityURI: pki.ControllerURI,
		ClientCertFile: clientCert, ClientKeyFile: clientKey, ClientCAFile: caPath,
		CertificateDenylistFile: denyPath, MaxBodyBytes: 1 << 20,
	}
	tlsMat, err := loadElasticTLS(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(tlsMat.server.Cert.Certificate) != 0 {
		t.Fatal("remote-only must not load agent server tls")
	}
	if len(tlsMat.client.Cert.Certificate) == 0 {
		t.Fatal("expected scheduler client tls")
	}
}

func TestRemoteOnlyControllerRegistryHasRemoteNodeOnly(t *testing.T) {
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
	if err := config.ValidateServeElastic(config.ElasticConfig{
		Enabled: true, AgentEmbedded: false, ControllerIdentityURI: pki.ControllerURI,
	}, remoteCfg); err != nil {
		t.Fatal(err)
	}
	tlsRemote, err := loadRemoteControlTLS(remoteCfg)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey := writeRemoteWireCertFiles(t, dir, "sched-client", pki.ControllerCert)
	elasticCfg := config.ElasticConfig{
		Enabled: true, AgentEmbedded: false, ControllerIdentityURI: pki.ControllerURI,
		ClientCertFile: clientCert, ClientKeyFile: clientKey, ClientCAFile: caPath,
		CertificateDenylistFile: denyPath, MaxBodyBytes: 1 << 20,
	}
	elasticTLS, err := loadElasticTLS(elasticCfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := registry.Open(filepath.Join(dir, "registry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close registry store: %v", err)
		}
	})
	guardID := "remote-only-ctrl"
	if err := store.TryAcquireControllerGuard(context.Background(), guardID, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.ReleaseControllerGuard(context.Background(), guardID); err != nil {
			t.Errorf("release controller guard %q: %v", guardID, err)
		}
	})
	guard := scheduler.NewRegistryGuard(store, guardID)

	ctx, cancel := context.WithCancel(context.Background())
	rt, err := startRemoteControl(ctx, remoteCfg, tlsRemote, store, guard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shCancel()
		_ = rt.quiesce(shCtx)
	})

	schedCtrl := newElasticSchedulerController(store, guard, elasticCfg, elasticTLS.client)
	schedDone := startElasticScheduler(ctx, schedCtrl, nil)

	controllerBase := "https://" + rt.server.Addr()
	writeAgentCertFiles(t, dir, pki)
	standaloneCfg := mustStandaloneElasticConfig(t, dir, pki, agentBind, agentURL, controllerBase)
	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runDone <- agentrun.Run(runCtx, standaloneCfg, config.Config{}, agentrun.Deps{
			LoadTLS: func(cfg config.ElasticConfig) (agentrun.TLSBundle, error) {
				client := agentTLSMaterials(pki)
				client.ServerName = cfg.ResolvedControllerTLSServerName()
				return agentrun.TLSBundle{
					Server: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool},
					Client: client,
				}, nil
			},
			NewBackend: func(*runtime.Manager) agent.LifecycleBackend { return fakeAgentBackend{} },
		})
	}()
	waitForNodeLease(t, store, pki, controllerBase, "n1", runDone)

	nodes, err := store.ListRuntimeNodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected exactly 1 runtime node, got %d: %+v", len(nodes), nodes)
	}
	if nodes[0].NodeID != "n1" || nodes[0].IdentityURI != pki.NodeURI || nodes[0].AgentBaseURL != agentURL {
		t.Fatalf("unexpected node identity: %+v", nodes[0])
	}

	runCancel()
	wg.Wait()
	if runErr := <-runDone; runErr != nil && !errors.Is(runErr, context.Canceled) {
		t.Fatalf("agent shutdown: %v", runErr)
	}
	cancel()
	if schedDone != nil {
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer waitCancel()
		_ = waitBackground(waitCtx, schedDone)
	}
}
