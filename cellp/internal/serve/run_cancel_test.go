package serve

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setEmbeddedServeElasticEnv(t *testing.T, dir string, pki remoteWirePKI, agentBind string) {
	t.Helper()
	_, _ = writeRemoteWireCertFiles(t, dir, "agent-server", pki.NodeCert)
	_, _ = writeRemoteWireCertFiles(t, dir, "agent-client", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	if err := os.WriteFile(denyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	advertise := "https://" + agentBind
	host, _, err := net.SplitHostPort(agentBind)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CELLP_AGENT_EMBEDDED", "1")
	t.Setenv("CELLP_AGENT_NODE_ID", "serve-run-test")
	t.Setenv("CELLP_AGENT_BIND_ADDR", agentBind)
	t.Setenv("CELLP_AGENT_ADVERTISE_URL", advertise)
	t.Setenv("CELLP_AGENT_NODE_IDENTITY_URI", pki.NodeURI)
	t.Setenv("CELLP_AGENT_CONTROLLER_IDENTITY_URI", pki.ControllerURI)
	t.Setenv("CELLP_AGENT_ALLOWED_CONTROLLER_URIS", pki.ControllerURI)
	t.Setenv("CELLP_AGENT_SERVER_CERT_FILE", filepath.Join(dir, "agent-server.pem"))
	t.Setenv("CELLP_AGENT_SERVER_KEY_FILE", filepath.Join(dir, "agent-server-key.pem"))
	t.Setenv("CELLP_AGENT_SERVER_CA_FILE", caPath)
	t.Setenv("CELLP_AGENT_CLIENT_CERT_FILE", filepath.Join(dir, "agent-client.pem"))
	t.Setenv("CELLP_AGENT_CLIENT_KEY_FILE", filepath.Join(dir, "agent-client-key.pem"))
	t.Setenv("CELLP_AGENT_CLIENT_CA_FILE", caPath)
	t.Setenv("CELLP_AGENT_TLS_SERVER_NAME", host)
	t.Setenv("CELLP_AGENT_CERT_DENYLIST_FILE", denyPath)
	t.Setenv("CELLP_AGENT_CAPACITY_UNITS", "2")
	t.Setenv("CELLP_AGENT_ZONE", "local")
	t.Setenv("CELLP_AGENT_HEARTBEAT_TTL", "30s")
	t.Setenv("CELLP_AGENT_HEARTBEAT_INTERVAL", "10s")
	t.Setenv("CELLP_AGENT_RECONCILE_INTERVAL", "5s")
	t.Setenv("CELLP_AGENT_MAX_BODY_BYTES", "1048576")
	t.Setenv("CELLP_AGENT_REPLAY_MAX_ENTRIES", "4096")
	t.Setenv("CELLP_AUTOSCALER_INTERVAL", "0")
}

func TestRunStopsOnCancel(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	agentBind := freeTCPAddr(t)
	setMinimalServeProcessEnv(t, dir, "")
	setEmbeddedServeElasticEnv(t, dir, pki, agentBind)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return
	case <-time.After(500 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for Run to exit")
	}
}
