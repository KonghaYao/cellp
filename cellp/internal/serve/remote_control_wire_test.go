package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/config"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/elastic/scheduler"
	"github.com/cellp/cellp/internal/registry"
)

func TestRemoteControlFeatureOffNoListener(t *testing.T) {
	cfg, err := config.LoadRemoteControlConfig(false)
	if err != nil || cfg.Enabled {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestRemoteControlMTLSNoderegAndRelay(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeRemoteWireCertFiles(t, dir, "server", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	if err := os.WriteFile(denyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	allowPath := filepath.Join(dir, "allow.json")
	agentURL := "https://127.0.0.1:19446"
	payload, _ := json.Marshal([]map[string]interface{}{
		{"node_id": "n1", "identity_uri": pki.NodeURI, "agent_base_url": agentURL, "allow_loopback": true},
	})
	if err := os.WriteFile(allowPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bind := ln.Addr().String()
	_ = ln.Close()

	remoteCfg := config.RemoteControlConfig{
		Enabled: true, BindAddr: bind, ControllerIdentityURI: pki.ControllerURI,
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
	guardID := "test-guard"
	if err := store.TryAcquireControllerGuard(context.Background(), guardID, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseControllerGuard(context.Background(), guardID)
	guard := scheduler.NewRegistryGuard(store, guardID)

	ctx := context.Background()
	rt, err := startRemoteControl(ctx, remoteCfg, tlsMat, store, guard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shCancel()
		_ = rt.quiesce(shCtx)
	})

	base := "https://" + rt.server.Addr()
	ndClient, err := noderegtransport.NewClient(noderegtransport.ClientConfig{
		BaseURL: base, NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "wire1"}
	out, err := ndClient.Activate(context.Background(), contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: agentURL, IdentityURI: pki.NodeURI, Zone: "z", ExpectedGeneration: 0,
	})
	if err != nil || out.Generation != 1 {
		t.Fatalf("activate: %+v err=%v", out, err)
	}

	relayClient, err := registryrelaytransport.NewClient(registryrelaytransport.ClientConfig{
		BaseURL: base, NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	node, err := relayClient.GetRuntimeNode(context.Background(), contract.RegistryRelayScope{
		NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "relay1",
	})
	if err != nil || node == nil || node.NodeID != "n1" {
		t.Fatalf("relay get node: %+v err=%v", node, err)
	}
}

func TestRemoteControlRejectsAgentBaseURLMismatch(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeRemoteWireCertFiles(t, dir, "server", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	_ = os.WriteFile(denyPath, nil, 0o600)
	authorized := "https://127.0.0.1:19449"
	remoteCfg := config.RemoteControlConfig{
		Enabled: true, BindAddr: "127.0.0.1:0", ControllerIdentityURI: pki.ControllerURI,
		ServerCertFile: certPath, ServerKeyFile: keyPath, ServerCAFile: caPath,
		CertificateDenylistFile: denyPath, NodeHeartbeatTTL: 30 * time.Second,
		MaxBodyBytes: 1 << 20, ReplayMaxEntries: 4096,
		NodeAllowlist: map[string]contract.NodeRegistrationBinding{
			"n1": {IdentityURI: pki.NodeURI, AgentBaseURL: authorized, AllowLoopback: true},
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
	guardID := "test-guard-4"
	_ = store.TryAcquireControllerGuard(context.Background(), guardID, os.Getpid())
	guard := scheduler.NewRegistryGuard(store, guardID)
	ctx := context.Background()
	rt, err := startRemoteControl(ctx, remoteCfg, tlsMat, store, guard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = rt.quiesce(context.Background())
	})
	ndClient, err := noderegtransport.NewClient(noderegtransport.ClientConfig{
		BaseURL: "https://" + rt.server.Addr(), NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "url1"}
	_, err = ndClient.Activate(context.Background(), contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 1, AgentBaseURL: "https://127.0.0.1:19999", IdentityURI: pki.NodeURI, ExpectedGeneration: 0,
	})
	if err == nil {
		t.Fatal("expected agent base url mismatch rejection")
	}
}

func TestRemoteControlRejectsUntrustedClientCert(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	other := mustRemoteWirePKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeRemoteWireCertFiles(t, dir, "server", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	_ = os.WriteFile(denyPath, nil, 0o600)
	agentURL := "https://127.0.0.1:19447"
	remoteCfg := config.RemoteControlConfig{
		Enabled: true, BindAddr: "127.0.0.1:0", ControllerIdentityURI: pki.ControllerURI,
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
	guardID := "test-guard-2"
	_ = store.TryAcquireControllerGuard(context.Background(), guardID, os.Getpid())
	guard := scheduler.NewRegistryGuard(store, guardID)
	ctx := context.Background()
	rt, err := startRemoteControl(ctx, remoteCfg, tlsMat, store, guard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = rt.quiesce(context.Background())
	})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	// re-bind: startRemoteControl already bound; use rt.server.Addr()
	base := "https://" + rt.server.Addr()
	ndClient, err := noderegtransport.NewClient(noderegtransport.ClientConfig{
		BaseURL: base, NodeIdentityURI: other.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: other.NodeCert, RootCAs: other.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "bad1"}
	_, err = ndClient.Activate(context.Background(), contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 1, AgentBaseURL: agentURL, IdentityURI: other.NodeURI, ExpectedGeneration: 0,
	})
	if err == nil {
		t.Fatal("expected auth failure for untrusted cert")
	}
}

func TestRemoteControlRejectsTrustedCertWrongScopeNodeID(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeRemoteWireCertFiles(t, dir, "server", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	_ = os.WriteFile(denyPath, nil, 0o600)
	agentURL := "https://127.0.0.1:19450"
	remoteCfg := config.RemoteControlConfig{
		Enabled: true, BindAddr: "127.0.0.1:0", ControllerIdentityURI: pki.ControllerURI,
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
	guardID := "test-guard-scope"
	_ = store.TryAcquireControllerGuard(context.Background(), guardID, os.Getpid())
	guard := scheduler.NewRegistryGuard(store, guardID)
	ctx := context.Background()
	rt, err := startRemoteControl(ctx, remoteCfg, tlsMat, store, guard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = rt.quiesce(context.Background())
	})
	ndClient, err := noderegtransport.NewClient(noderegtransport.ClientConfig{
		BaseURL: "https://" + rt.server.Addr(), NodeIdentityURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n-not-allowed", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "scope1"}
	_, err = ndClient.Activate(context.Background(), contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 1, AgentBaseURL: agentURL, IdentityURI: pki.NodeURI, ExpectedGeneration: 0,
	})
	if err == nil {
		t.Fatal("expected rejection for trusted cert with wrong scope node_id")
	}
}

func TestRemoteControlShutdownClosesListener(t *testing.T) {
	pki := mustRemoteWirePKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeRemoteWireCertFiles(t, dir, "server", pki.ControllerCert)
	caPath := writeRemoteWireCA(t, dir, pki.CACert)
	denyPath := filepath.Join(dir, "denylist")
	_ = os.WriteFile(denyPath, nil, 0o600)
	agentURL := "https://127.0.0.1:19448"
	remoteCfg := config.RemoteControlConfig{
		Enabled: true, BindAddr: "127.0.0.1:0", ControllerIdentityURI: pki.ControllerURI,
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
	guardID := "test-guard-3"
	_ = store.TryAcquireControllerGuard(context.Background(), guardID, os.Getpid())
	guard := scheduler.NewRegistryGuard(store, guardID)
	ctx := context.Background()
	rt, err := startRemoteControl(ctx, remoteCfg, tlsMat, store, guard)
	if err != nil {
		t.Fatal(err)
	}
	addr := rt.server.Addr()
	shCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shCancel()
	if err := rt.quiesce(shCtx); err != nil {
		t.Fatal(err)
	}
	waitRemoteWireTCPClosed(t, addr, time.Now().Add(3*time.Second))
}

func waitRemoteWireTCPClosed(t *testing.T, addr string, deadline time.Time) {
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
	t.Fatal("listener still accepting after shutdown")
}

type remoteWirePKI struct {
	RootPool       *x509.CertPool
	CACert         *x509.Certificate
	ControllerCert tls.Certificate
	NodeCert       tls.Certificate
	ControllerURI  string
	NodeURI        string
}

func mustRemoteWirePKI(t *testing.T) remoteWirePKI {
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
	ctrlURI := "spiffe://cellp/test/controller/" + randomSuffix()
	nodeURI := "spiffe://cellp/test/node/" + randomSuffix()
	return remoteWirePKI{
		RootPool: pool, CACert: caCert,
		ControllerCert: mustRemoteWireLeaf(t, caCert, caKey, ctrlURI),
		NodeCert:       mustRemoteWireLeaf(t, caCert, caKey, nodeURI),
		ControllerURI:  ctrlURI, NodeURI: nodeURI,
	}
}

func randomSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func mustRemoteWireLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, uri string) tls.Certificate {
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

func writeRemoteWireCertFiles(t *testing.T, dir, prefix string, cert tls.Certificate) (string, string) {
	t.Helper()
	certPath := filepath.Join(dir, prefix+".pem")
	keyPath := filepath.Join(dir, prefix+"-key.pem")
	var certPEM []byte
	for _, der := range cert.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	key, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("key type")
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func writeRemoteWireCA(t *testing.T, dir string, cert *x509.Certificate) string {
	t.Helper()
	path := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
