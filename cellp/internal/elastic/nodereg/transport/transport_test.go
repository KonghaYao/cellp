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
	"math/big"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/nodereg"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	"github.com/cellp/cellp/internal/registry"
)

type guardOK struct{}

func (guardOK) EnsureActiveController(context.Context) error { return nil }

func TestNoderegTLSActivateRoundTrip(t *testing.T) {
	pki := mustNoderegPKI(t)
	store, err := registry.Open(t.TempDir() + "/nodereg-tls.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := pki.NodeURI
	h := &nodereg.Handler{
		Store: store, Allowlist: map[string]contract.NodeRegistrationBinding{
			"n1": {IdentityURI: uri, AgentBaseURL: "https://n1.agent.example"},
		}, Guard: guardOK{},
		HeartbeatTTL: 30 * time.Second,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	srv, err := noderegtransport.NewServer(noderegtransport.ServerConfig{
		BindAddr: addr, ControllerIdentityURI: pki.ControllerURI, AllowedNodeURIs: []string{uri},
		TLS: agenttransport.TLSMaterials{Cert: pki.ControllerCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	}, h)
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
	time.Sleep(30 * time.Millisecond)
	t.Cleanup(func() {
		cancel()
		_ = srv.Shutdown(context.Background())
		wg.Wait()
	})
	client, err := noderegtransport.NewClient(noderegtransport.ClientConfig{
		BaseURL: "https://" + srv.Addr(), NodeIdentityURI: uri, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.NodeCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "tls1"}
	out, err := client.Activate(context.Background(), contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: "https://n1.agent.example",
		IdentityURI: uri, Zone: "z", ExpectedGeneration: 0,
	})
	if err != nil || out.Generation != 1 {
		t.Fatalf("activate: %+v err=%v", out, err)
	}
}

type noderegPKI struct {
	RootPool       *x509.CertPool
	ControllerCert tls.Certificate
	NodeCert       tls.Certificate
	ControllerURI  string
	NodeURI        string
}

func mustNoderegPKI(t *testing.T) noderegPKI {
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
	nodeURI := "spiffe://cellp/test/node/n1"
	return noderegPKI{
		RootPool: pool, ControllerCert: mustNoderegLeaf(t, caCert, caKey, ctrlURI), NodeCert: mustNoderegLeaf(t, caCert, caKey, nodeURI),
		ControllerURI: ctrlURI, NodeURI: nodeURI,
	}
}

func mustNoderegLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, uri string) tls.Certificate {
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
