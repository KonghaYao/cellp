package elasticdevcerts

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Generate writes dev mTLS material for embedded Node Agent (AD-15 local stack).
func Generate(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	nodeURI := "spiffe://cellp/local/node/local-dev-node"
	ctrlURI := "spiffe://cellp/local/controller/main"
	caKey, caCert, err := genCA()
	if err != nil {
		return err
	}
	caPath := filepath.Join(dir, "ca.pem")
	if err := writePEM(caPath, "CERTIFICATE", caCert); err != nil {
		return err
	}
	nodeCert, nodeKey, err := leaf(caCert, caKey, nodeURI)
	if err != nil {
		return err
	}
	ctrlCert, ctrlKey, err := leaf(caCert, caKey, ctrlURI)
	if err != nil {
		return err
	}
	if err := writeKeyPair(dir, "agent-server", nodeCert, nodeKey); err != nil {
		return err
	}
	if err := writeKeyPair(dir, "agent-client", ctrlCert, ctrlKey); err != nil {
		return err
	}
	deny := filepath.Join(dir, "denylist.pem")
	if err := os.WriteFile(deny, nil, 0o600); err != nil {
		return err
	}
	return nil
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

// ApplyEmbeddedAgentEnv sets CELLP_AGENT_* for embedded elastic in cellp dev / local stack.
func ApplyEmbeddedAgentEnv(certDir string) error {
	certDir = filepath.Clean(certDir)
	serverCert := filepath.Join(certDir, "agent-server.pem")
	if _, err := os.Stat(serverCert); err != nil {
		if err := Generate(certDir); err != nil {
			return fmt.Errorf("elastic dev certs: %w", err)
		}
	}
	nodeID := "local-dev-node"
	ctrlURI := "spiffe://cellp/local/controller/main"
	nodeURI := "spiffe://cellp/local/node/" + nodeID
	agentBind := "127.0.0.1:19443"
	set := func(k, v string) {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	set("CELLP_AGENT_EMBEDDED", "1")
	set("CELLP_AGENT_NODE_ID", nodeID)
	set("CELLP_AGENT_BIND_ADDR", agentBind)
	set("CELLP_AGENT_ADVERTISE_URL", "https://"+agentBind)
	set("CELLP_AGENT_ALLOW_PRIVATE_NAT", "false")
	set("CELLP_AGENT_NODE_IDENTITY_URI", nodeURI)
	set("CELLP_AGENT_CONTROLLER_IDENTITY_URI", ctrlURI)
	set("CELLP_AGENT_ALLOWED_CONTROLLER_URIS", ctrlURI)
	set("CELLP_AGENT_SERVER_CERT_FILE", filepath.Join(certDir, "agent-server.pem"))
	set("CELLP_AGENT_SERVER_KEY_FILE", filepath.Join(certDir, "agent-server-key.pem"))
	set("CELLP_AGENT_SERVER_CA_FILE", filepath.Join(certDir, "ca.pem"))
	set("CELLP_AGENT_CLIENT_CERT_FILE", filepath.Join(certDir, "agent-client.pem"))
	set("CELLP_AGENT_CLIENT_KEY_FILE", filepath.Join(certDir, "agent-client-key.pem"))
	set("CELLP_AGENT_CLIENT_CA_FILE", filepath.Join(certDir, "ca.pem"))
	set("CELLP_AGENT_TLS_SERVER_NAME", "127.0.0.1")
	set("CELLP_AGENT_CERT_DENYLIST_FILE", filepath.Join(certDir, "denylist.pem"))
	set("CELLP_AGENT_CAPACITY_UNITS", "8")
	set("CELLP_AGENT_ZONE", "local-dev")
	set("CELLP_AGENT_HEARTBEAT_TTL", "30s")
	set("CELLP_AGENT_HEARTBEAT_INTERVAL", "10s")
	set("CELLP_AGENT_RECONCILE_INTERVAL", "5s")
	set("CELLP_AGENT_MAX_BODY_BYTES", "1048576")
	set("CELLP_AGENT_REPLAY_MAX_ENTRIES", "4096")
	return nil
}
