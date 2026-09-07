package controllermtls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/nodereg"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/registry"
)

type shutdownGuardOK struct{}

func (shutdownGuardOK) EnsureActiveController(context.Context) error { return nil }

func mustShutdownTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := registry.Open(t.TempDir() + "/shutdown.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	mat := mustShutdownTLS(t)
	ctrlURI := "spiffe://cellp/test/controller/shutdown"
	nodeURI := "spiffe://cellp/test/node/shutdown"
	ndHandler := &nodereg.Handler{
		Store: store,
		Allowlist: map[string]contract.NodeRegistrationBinding{
			"n1": {IdentityURI: nodeURI, AgentBaseURL: "https://127.0.0.1:1", AllowLoopback: true},
		},
		Guard: shutdownGuardOK{},
	}
	ndSrv, err := noderegtransport.NewServer(noderegtransport.ServerConfig{}, ndHandler)
	if err != nil {
		t.Fatal(err)
	}
	relayHandler := &registryrelay.Handler{
		Store:     store,
		Allowlist: map[string]string{"n1": nodeURI},
		Guard:     shutdownGuardOK{},
	}
	rlSrv, err := registryrelaytransport.NewServer(registryrelaytransport.ServerConfig{}, relayHandler)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(Config{
		BindAddr:              "127.0.0.1:0",
		ControllerIdentityURI: ctrlURI,
		TLS:                   mat,
		Nodereg:               ndSrv,
		Relay:                 rlSrv,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func mustShutdownTLS(t *testing.T) agenttransport.TLSMaterials {
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
	leaf := mustShutdownLeaf(t, caCert, caKey, "spiffe://cellp/test/controller/shutdown")
	return agenttransport.TLSMaterials{Cert: leaf, RootCAs: pool, ServerName: "127.0.0.1"}
}

func mustShutdownLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, uri string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "leaf"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		URIs: []*url.URL{u}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func waitTCPClosed(t *testing.T, addr string, deadline time.Time) {
	t.Helper()
	d := net.Dialer{Timeout: 50 * time.Millisecond}
	for time.Now().Before(deadline) {
		c, err := d.Dial("tcp", addr)
		if err != nil {
			return
		}
		_ = c.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("listener %s still accepting after %v", addr, deadline)
}

func waitServerReady(t *testing.T, srv *Server, timeout time.Duration) {
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

func TestRunContextCancelStopsListener(t *testing.T) {
	srv := mustShutdownTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	waitServerReady(t, srv, 3*time.Second)
	addr := srv.Addr()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
	waitTCPClosed(t, addr, time.Now().Add(3*time.Second))
}

func TestExplicitShutdownStopsListener(t *testing.T) {
	srv := mustShutdownTestServer(t)
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	waitServerReady(t, srv, 3*time.Second)
	addr := srv.Addr()
	shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shCancel()
	if err := srv.Shutdown(shCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
	waitTCPClosed(t, addr, time.Now().Add(3*time.Second))
}

func TestConcurrentCancelAndShutdown(t *testing.T) {
	srv := mustShutdownTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	waitServerReady(t, srv, 3*time.Second)
	shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shCancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cancel()
	}()
	go func() {
		defer wg.Done()
		if err := srv.Shutdown(shCtx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()
	wg.Wait()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestShutdownBeforeRunStartupWaitReturns(t *testing.T) {
	srv := mustShutdownTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	done := make(chan struct{})
	runErr := make(chan error, 1)
	go func() {
		defer close(done)
		runErr <- srv.Run(context.Background())
	}()
	if err := WaitForStartup(ctx, srv.Ready, done, runErr); err == nil {
		t.Fatal("expected startup failure after shutdown-before-run")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit")
	}
}

func TestConcurrentRunShutdownRounds(t *testing.T) {
	const rounds = 8
	deadline := time.Now().Add(20 * time.Second)
	for i := 0; i < rounds; i++ {
		if time.Now().After(deadline) {
			t.Fatal("deadline exceeded in concurrent run/shutdown rounds")
		}
		srv := mustShutdownTestServer(t)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		runErr := make(chan error, 1)
		go func() {
			defer close(done)
			runErr <- srv.Run(ctx)
		}()
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_ = WaitForStartup(waitCtx, srv.Ready, done, runErr)
		waitCancel()
		shCtx, shCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(shCtx)
		shCancel()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("round %d: Run hung", i)
		}
	}
}

func TestShutdownRacesRunBeforeReady(t *testing.T) {
	srv := mustShutdownTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	runErr := make(chan error, 1)
	go func() {
		defer close(done)
		runErr <- srv.Run(ctx)
	}()
	shCtx, shCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = srv.Shutdown(shCtx)
	shCancel()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	if err := WaitForStartup(waitCtx, srv.Ready, done, runErr); err == nil {
		t.Fatal("expected startup failure when shutdown races Run")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run hung after shutdown race")
	}
}

func TestShutdownCallerDeadlineThenSuccess(t *testing.T) {
	done := make(chan struct{})
	srv := &Server{shutdownDone: done}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer shortCancel()
	if err := srv.waitShutdown(shortCtx, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	close(done)
	longCtx, longCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer longCancel()
	if err := srv.waitShutdown(longCtx, done); err != nil {
		t.Fatalf("expected nil shared shutdown err after completion, got %v", err)
	}

	live := mustShutdownTestServer(t)
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go func() { _ = live.Run(runCtx) }()
	waitServerReady(t, live, 3*time.Second)
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

func TestShutdownBeforeRunIdempotent(t *testing.T) {
	srv := mustShutdownTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	if err := srv.Run(context.Background()); err == nil || err.Error() != "server closed" {
		t.Fatalf("Run after shutdown: %v", err)
	}
}

func TestRepeatedShutdownWaitsSameDone(t *testing.T) {
	srv := mustShutdownTestServer(t)
	ctx := context.Background()
	go func() { _ = srv.Run(ctx) }()
	waitServerReady(t, srv, 3*time.Second)
	shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shCancel()
	var firstErr, secondErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		firstErr = srv.Shutdown(shCtx)
	}()
	go func() {
		defer wg.Done()
		secondErr = srv.Shutdown(shCtx)
	}()
	wg.Wait()
	if firstErr != nil || secondErr != nil {
		t.Fatalf("shutdown errors: %v / %v", firstErr, secondErr)
	}
}

func TestRunReturnsRealListenError(t *testing.T) {
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	addr := holder.Addr().String()

	store, err := registry.Open(t.TempDir() + "/bind.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mat := mustShutdownTLS(t)
	nodeURI := "spiffe://cellp/test/node/bind"
	ndSrv, err := noderegtransport.NewServer(noderegtransport.ServerConfig{}, &nodereg.Handler{
		Store: store,
		Allowlist: map[string]contract.NodeRegistrationBinding{
			"n1": {IdentityURI: nodeURI, AgentBaseURL: "https://127.0.0.1:1", AllowLoopback: true},
		},
		Guard: shutdownGuardOK{},
	})
	if err != nil {
		t.Fatal(err)
	}
	rlSrv, err := registryrelaytransport.NewServer(registryrelaytransport.ServerConfig{}, &registryrelay.Handler{
		Store: store, Allowlist: map[string]string{"n1": nodeURI}, Guard: shutdownGuardOK{},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(Config{
		BindAddr: addr, ControllerIdentityURI: "spiffe://cellp/test/controller/bind",
		TLS: mat, Nodereg: ndSrv, Relay: rlSrv,
	})
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

func TestNormalizeServeErrClosedListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	hs := &http.Server{Handler: mux}
	errCh := make(chan error, 1)
	go func() { errCh <- hs.Serve(ln) }()
	deadline := time.Now().Add(3 * time.Second)
	d := net.Dialer{Timeout: 50 * time.Millisecond}
	for time.Now().Before(deadline) {
		c, dialErr := d.Dial("tcp", addr)
		if dialErr == nil {
			_ = c.Close()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = ln.Close()
	err = <-errCh
	if norm := normalizeServeErr(err, true); norm != nil {
		t.Fatalf("expected nil normalized serve err during shutdown, got %v", norm)
	}
}
