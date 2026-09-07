// genelasticdevcerts writes dev mTLS material for embedded Node Agent (AD-15 local stack).
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func main() {
	out := flag.String("dir", "", "output directory for PEM files")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: genelasticdevcerts --dir <path>")
		os.Exit(2)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	nodeURI := "spiffe://cellp/local/node/local-dev-node"
	ctrlURI := "spiffe://cellp/local/controller/main"
	caKey, caCert, err := genCA()
	if err != nil {
		fatal(err)
	}
	caPath := filepath.Join(*out, "ca.pem")
	if err := writePEM(caPath, "CERTIFICATE", caCert); err != nil {
		fatal(err)
	}
	nodeCert, nodeKey, err := leaf(caCert, caKey, nodeURI)
	if err != nil {
		fatal(err)
	}
	ctrlCert, ctrlKey, err := leaf(caCert, caKey, ctrlURI)
	if err != nil {
		fatal(err)
	}
	if err := writeKeyPair(*out, "agent-server", nodeCert, nodeKey); err != nil {
		fatal(err)
	}
	if err := writeKeyPair(*out, "agent-client", ctrlCert, ctrlKey); err != nil {
		fatal(err)
	}
	deny := filepath.Join(*out, "denylist.pem")
	if err := os.WriteFile(deny, nil, 0o600); err != nil {
		fatal(err)
	}
	fmt.Println(*out)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func genCA() (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "cellp-dev-elastic-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:         true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return key, der, nil
}

func leaf(caDER []byte, caKey *ecdsa.PrivateKey, uri string) (tls.Certificate, *ecdsa.PrivateKey, error) {
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	u, _ := url.Parse(uri)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "cellp-dev-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		URIs:         []*url.URL{u},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return pair, key, nil
}

func writeKeyPair(dir, prefix string, cert tls.Certificate, key *ecdsa.PrivateKey) error {
	var certPEM []byte
	for _, der := range cert.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, prefix+".pem"), certPEM, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, prefix+"-key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
}

func writePEM(path, typ string, der []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o644)
}
