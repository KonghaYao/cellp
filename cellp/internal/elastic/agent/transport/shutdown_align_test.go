package transport_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/agent/transport"
)

func TestShutdownCallerDeadlineThenSuccess(t *testing.T) {
	live := startMinimalTransportServer(t)
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go func() { _ = live.Run(runCtx) }()
	waitTransportReady(t, live, 3*time.Second)

	expiredCtx, expiredCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer expiredCancel()
	if err := live.Shutdown(expiredCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired caller ctx expected deadline, got %v", err)
	}
	okCtx, okCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer okCancel()
	if err := live.Shutdown(okCtx); err != nil {
		t.Fatalf("follow-up shutdown with longer ctx: %v", err)
	}
}

func TestRunReturnsRealListenError(t *testing.T) {
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	addr := holder.Addr().String()

	pki := mustTestPKI(t)
	mem := newMemStores()
	h := agent.NewHandler(true, mem, mem)
	cfg := transport.ServerConfig{
		Enabled:               true,
		NodeID:                "node-a",
		BindAddr:              addr,
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
	err = srv.Run(context.Background())
	if err == nil {
		t.Fatal("expected bind error")
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected net.OpError, got %v", err)
	}
}

func startMinimalTransportServer(t *testing.T) *transport.Server {
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
	return srv
}

func waitTransportReady(t *testing.T, srv *transport.Server, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-srv.Ready():
			return
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
	t.Fatal("server not ready")
}
