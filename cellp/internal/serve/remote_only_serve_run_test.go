package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/agentrun"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestRemoteOnlyServeRunRejectsInvalidElasticRemote(t *testing.T) {
	dir := t.TempDir()
	reservedRemote := freeTCPAddr(t)
	setMinimalServeProcessEnv(t, dir, reservedRemote)
	t.Setenv("CELLP_ELASTIC_RUNTIME", "1")
	t.Setenv("CELLP_AGENT_EMBEDDED", "0")
	t.Setenv("CELLP_AGENT_CONTROLLER_IDENTITY_URI", "spiffe://cellp/test/controller/reject")
	t.Setenv("CELLP_AGENT_CLIENT_CERT_FILE", filepath.Join(dir, "client.pem"))
	t.Setenv("CELLP_AGENT_CLIENT_KEY_FILE", filepath.Join(dir, "client-key.pem"))
	t.Setenv("CELLP_AGENT_CLIENT_CA_FILE", filepath.Join(dir, "ca.pem"))
	t.Setenv("CELLP_AGENT_CERT_DENYLIST_FILE", filepath.Join(dir, "denylist"))
	t.Setenv("CELLP_AGENT_MAX_BODY_BYTES", "1048576")
	if err := os.WriteFile(filepath.Join(dir, "denylist"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	err := Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "remote control required") {
		t.Fatalf("expected remote control validation error before serve side effects, got %v", err)
	}
	waitRemoteWireTCPClosed(t, reservedRemote, time.Now().Add(500*time.Millisecond))
}

func TestRemoteOnlyServeRunRegistersAllowlistedNodeOnly(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeRemoteWireCertFiles(t, dir, "server", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	if err := os.WriteFile(denyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey := writeRemoteWireCertFiles(t, dir, "sched-client", pki.ControllerCert)

	agentBind := freeTCPAddr(t)
	agentURL := "https://" + agentBind
	ctrlBind := freeTCPAddr(t)
	decoyEmbeddedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	embeddedAgentBind := decoyEmbeddedListener.Addr().String()
	embeddedDecoyURL := "https://" + embeddedAgentBind
	decoyProbe := startEmbeddedAgentDecoyAcceptProbe(t, decoyEmbeddedListener)
	t.Cleanup(decoyProbe.stop)
	allowPath := writeRemoteAllowlistForAgent(t, dir, pki, agentURL)
	registryPath := filepath.Join(dir, "registry.sqlite")

	setMinimalServeProcessEnv(t, dir, ctrlBind)
	setRemoteOnlyServeElasticEnv(t, dir, pki, ctrlBind, allowPath, certPath, keyPath, caPath, denyPath, clientCert, clientKey)
	setRemoteOnlyEmbeddedAgentDecoyEnv(t, dir, pki, embeddedAgentBind)

	serveCtx, serveCancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- Run(serveCtx) }()

	waitTCPAccepting(t, ctrlBind, 15*time.Second)
	assertServeRunStillActive(t, serveDone, "controller remote listener up")
	controllerBase := "https://" + ctrlBind

	writeAgentCertFiles(t, dir, pki)
	standaloneCfg := mustStandaloneElasticConfig(t, dir, pki, agentBind, agentURL, controllerBase)

	agentCtx, agentCancel := context.WithCancel(context.Background())
	agentDone := make(chan error, 1)
	var agentWG sync.WaitGroup
	agentWG.Add(1)
	go func() {
		defer agentWG.Done()
		agentDone <- agentrun.Run(agentCtx, standaloneCfg, config.Config{}, agentrun.Deps{
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

	store, err := registry.Open(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	waitForNodeLease(t, store, pki, controllerBase, "n1", agentDone)

	var nodes []contract.RuntimeNode
	listDeadline := time.Now().Add(5 * time.Second)
	if err := registryCallEventually(t, listDeadline, func() error {
		var listErr error
		nodes, listErr = store.ListRuntimeNodes(context.Background())
		return listErr
	}); err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected exactly 1 runtime node, got %d: %+v", len(nodes), nodes)
	}
	if nodes[0].NodeID != "n1" || nodes[0].IdentityURI != pki.NodeURI || nodes[0].AgentBaseURL != agentURL {
		t.Fatalf("unexpected node identity: %+v", nodes[0])
	}
	if nodes[0].AgentBaseURL == embeddedDecoyURL {
		t.Fatalf("runtime node must not use decoy embedded agent URL %q", embeddedDecoyURL)
	}

	assertServeRunStillActive(t, serveDone, "remote node registered")
	assertEmbeddedAgentDecoyStillOwnsPort(t, embeddedAgentBind, decoyProbe)
	waitTCPAccepting(t, agentBind, 5*time.Second)

	agentCancel()
	agentWG.Wait()
	if runErr := <-agentDone; runErr != nil && !errors.Is(runErr, context.Canceled) {
		t.Fatalf("agent shutdown: %v", runErr)
	}

	serveCancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve shutdown: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for serve.Run to exit")
	}
	waitControllerGuardReleased(t, store, time.Now().Add(10*time.Second))
	waitRemoteWireTCPClosed(t, ctrlBind, time.Now().Add(5*time.Second))
}

var embeddedAgentDecoyProbeToken = []byte("cellp-embedded-decoy-probe-v1")

type embeddedAgentDecoyOwnershipProbe struct {
	ln            net.Listener
	tokenReceived chan struct{}
	acceptErr     chan error
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	stopOnce      sync.Once
}

func startEmbeddedAgentDecoyAcceptProbe(t *testing.T, ln net.Listener) *embeddedAgentDecoyOwnershipProbe {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := &embeddedAgentDecoyOwnershipProbe{
		ln:            ln,
		tokenReceived: make(chan struct{}, 1),
		acceptErr:     make(chan error, 1),
		cancel:        cancel,
	}
	expected := embeddedAgentDecoyProbeToken
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			if ctx.Err() != nil {
				return
			}
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					return
				}
				select {
				case p.acceptErr <- err:
				default:
				}
				continue
			}
			buf := make([]byte, len(expected))
			_, readErr := io.ReadFull(conn, buf)
			closeErr := conn.Close()
			if readErr == nil && bytes.Equal(buf, expected) {
				select {
				case p.tokenReceived <- struct{}{}:
				default:
				}
				return
			}
			if readErr != nil {
				select {
				case p.acceptErr <- readErr:
				default:
				}
			} else if closeErr != nil {
				select {
				case p.acceptErr <- closeErr:
				default:
				}
			}
		}
	}()
	return p
}

func (p *embeddedAgentDecoyOwnershipProbe) stop() {
	p.stopOnce.Do(func() {
		closeEmbeddedAgentDecoyListener(p.ln)
		p.cancel()
		p.wg.Wait()
	})
}

func closeEmbeddedAgentDecoyListener(ln net.Listener) {
	if ln == nil {
		return
	}
	if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		panic("close embedded-agent decoy listener: " + err.Error())
	}
}

func TestEmbeddedAgentDecoyAcceptProbeCleanupWithoutToken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	probe := startEmbeddedAgentDecoyAcceptProbe(t, ln)
	t.Cleanup(probe.stop)

	done := make(chan struct{})
	go func() {
		probe.stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("decoy probe cleanup hung without ownership token")
	}
	probe.stop()
}

func assertEmbeddedAgentDecoyStillOwnsPort(t *testing.T, bind string, probe *embeddedAgentDecoyOwnershipProbe) {
	t.Helper()
	select {
	case err := <-probe.acceptErr:
		t.Fatalf("decoy accept loop failed before ownership probe: %v", err)
	default:
	}
	conn, err := net.DialTimeout("tcp", bind, 2*time.Second)
	if err != nil {
		t.Fatalf("dial embedded-agent decoy bind %q: %v", bind, err)
	}
	if _, err := conn.Write(embeddedAgentDecoyProbeToken); err != nil {
		_ = conn.Close()
		t.Fatalf("write decoy ownership probe token: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close decoy probe dial conn: %v", err)
	}
	select {
	case <-probe.tokenReceived:
	case err := <-probe.acceptErr:
		t.Fatalf("decoy did not receive ownership probe token: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for decoy to receive ownership probe token")
	}
}

func setMinimalServeProcessEnv(t *testing.T, dir string, _ string) {
	t.Helper()
	t.Setenv("CELLP_REGISTRY_DB", filepath.Join(dir, "registry.sqlite"))
	t.Setenv("PLATFORM_PORT", strconv.Itoa(freeServePort(t)))
	t.Setenv("GATEWAY_PORT", strconv.Itoa(freeServePort(t)))
	t.Setenv("OFFSHOOT_STORE", filepath.Join(dir, "offshoot"))
	t.Setenv("ARTIFACTS_DIR", filepath.Join(dir, "artifacts"))
	t.Setenv("CELLP_GC_INTERVAL", "0")
}

func setRemoteOnlyEmbeddedAgentDecoyEnv(t *testing.T, dir string, pki remoteWirePKI, embeddedBind string) {
	t.Helper()
	embeddedURL := "https://" + embeddedBind
	serverCert, serverKey := writeRemoteWireCertFiles(t, dir, "embedded-agent-server", pki.NodeCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	t.Setenv("CELLP_AGENT_BIND_ADDR", embeddedBind)
	t.Setenv("CELLP_AGENT_ADVERTISE_URL", embeddedURL)
	t.Setenv("CELLP_AGENT_SERVER_CERT_FILE", serverCert)
	t.Setenv("CELLP_AGENT_SERVER_KEY_FILE", serverKey)
	t.Setenv("CELLP_AGENT_SERVER_CA_FILE", caPath)
	if host, _, err := net.SplitHostPort(embeddedBind); err == nil {
		t.Setenv("CELLP_AGENT_TLS_SERVER_NAME", host)
	}
}

func setRemoteOnlyServeElasticEnv(t *testing.T, dir string, pki remoteWirePKI, ctrlBind, allowPath, certPath, keyPath, caPath, denyPath, clientCert, clientKey string) {
	t.Helper()
	t.Setenv("CELLP_ELASTIC_RUNTIME", "1")
	t.Setenv("CELLP_AGENT_EMBEDDED", "0")
	t.Setenv("CELLP_AGENT_CONTROLLER_IDENTITY_URI", pki.ControllerURI)
	t.Setenv("CELLP_AGENT_CLIENT_CERT_FILE", clientCert)
	t.Setenv("CELLP_AGENT_CLIENT_KEY_FILE", clientKey)
	t.Setenv("CELLP_AGENT_CLIENT_CA_FILE", caPath)
	t.Setenv("CELLP_AGENT_CERT_DENYLIST_FILE", denyPath)
	t.Setenv("CELLP_AGENT_MAX_BODY_BYTES", "1048576")
	t.Setenv("CELLP_CONTROLLER_IDENTITY_URI", pki.ControllerURI)
	t.Setenv("CELLP_CONTROLLER_REMOTE_BIND_ADDR", ctrlBind)
	t.Setenv("CELLP_CONTROLLER_REMOTE_SERVER_CERT_FILE", certPath)
	t.Setenv("CELLP_CONTROLLER_REMOTE_SERVER_KEY_FILE", keyPath)
	t.Setenv("CELLP_CONTROLLER_REMOTE_SERVER_CA_FILE", caPath)
	t.Setenv("CELLP_CONTROLLER_REMOTE_CERT_DENYLIST_FILE", denyPath)
	t.Setenv("CELLP_CONTROLLER_NODE_ALLOWLIST_FILE", allowPath)
	t.Setenv("CELLP_CONTROLLER_REMOTE_NODE_HEARTBEAT_TTL", "30s")
	t.Setenv("CELLP_CONTROLLER_REMOTE_MAX_BODY_BYTES", "1048576")
	t.Setenv("CELLP_CONTROLLER_REMOTE_REPLAY_MAX_ENTRIES", "4096")
}

func writeRemoteAllowlistForAgent(t *testing.T, dir string, pki remoteWirePKI, agentURL string) string {
	t.Helper()
	path := filepath.Join(dir, "allow.json")
	payload, err := json.Marshal([]map[string]interface{}{
		{"node_id": "n1", "identity_uri": pki.NodeURI, "agent_base_url": agentURL, "allow_loopback": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func freeServePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func waitTCPAccepting(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	d := net.Dialer{Timeout: 100 * time.Millisecond}
	for time.Now().Before(deadline) {
		c, err := d.Dial("tcp", addr)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tcp %s not accepting within %s", addr, timeout)
}

func assertServeRunStillActive(t *testing.T, serveDone <-chan error, phase string) {
	t.Helper()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve.Run exited before %s: %v", phase, err)
		}
		t.Fatalf("serve.Run exited before %s", phase)
	default:
	}
}

func waitControllerGuardReleased(t *testing.T, store *registry.SQLiteStore, deadline time.Time) {
	t.Helper()
	ctx := context.Background()
	probePID := os.Getpid() + 1
	for time.Now().Before(deadline) {
		st, err := store.GetControllerGuard(ctx)
		if err != nil {
			if registry.IsSQLiteBusy(err) {
				time.Sleep(registryBusyPollInterval)
				continue
			}
			t.Fatal(err)
		}
		if st == nil || st.HolderID == "" {
			return
		}
		if err := store.TryAcquireControllerGuard(ctx, "post-shutdown-probe", probePID); err == nil {
			if relErr := store.ReleaseControllerGuard(ctx, "post-shutdown-probe"); relErr != nil {
				t.Fatalf("release post-shutdown probe controller guard: %v", relErr)
			}
			return
		}
		if registry.IsSQLiteBusy(err) {
			time.Sleep(registryBusyPollInterval)
			continue
		}
		if !errors.Is(err, registry.ErrControllerGuardHeld) {
			t.Fatal(err)
		}
		time.Sleep(registryBusyPollInterval)
	}
	st, err := store.GetControllerGuard(ctx)
	if err != nil {
		if registry.IsSQLiteBusy(err) {
			t.Fatalf("controller guard still held after serve shutdown (last get busy): %v", err)
		}
		t.Fatal(err)
	}
	t.Fatalf("controller guard still held after serve shutdown: %+v", st)
}
