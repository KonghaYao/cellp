package transport_test

import (
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
	"testing"
	"time"
)

type testPKI struct {
	CA       *x509.Certificate
	CAKey    *ecdsa.PrivateKey
	RootPool *x509.CertPool

	ServerCert tls.Certificate
	ClientCert tls.Certificate

	NodeURI        string
	ControllerURI  string
	ControllerURI2 string
}

func mustTestPKI(t *testing.T) testPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
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

	nodeURI := "spiffe://cellp/test/node/node-a"
	ctrlURI := "spiffe://cellp/test/controller/ctrl-1"
	ctrlURI2 := "spiffe://cellp/test/controller/ctrl-2"

	server := mustLeafCert(t, caCert, caKey, nodeURI, nil, time.Now().Add(24*time.Hour))
	client := mustLeafCert(t, caCert, caKey, ctrlURI, nil, time.Now().Add(24*time.Hour))

	return testPKI{
		CA:             caCert,
		CAKey:          caKey,
		RootPool:       pool,
		ServerCert:     server,
		ClientCert:     client,
		NodeURI:        nodeURI,
		ControllerURI:  ctrlURI,
		ControllerURI2: ctrlURI2,
	}
}

func mustLeafCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, uri string, extraURIs []string, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var uris []*url.URL
	if uri != "" {
		u, err := url.Parse(uri)
		if err != nil {
			t.Fatal(err)
		}
		uris = append(uris, u)
	}
	for _, s := range extraURIs {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		uris = append(uris, u)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		URIs:         uris,
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
	cert.Leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
