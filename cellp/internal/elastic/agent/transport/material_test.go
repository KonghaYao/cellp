package transport_test

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent/transport"
)

func writeCertificateFiles(t *testing.T, dir, prefix string, cert tls.Certificate) (string, string) {
	t.Helper()
	certPath := filepath.Join(dir, prefix+".pem")
	keyPath := filepath.Join(dir, prefix+"-key.pem")
	var certPEM []byte
	for _, der := range cert.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	key, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("unexpected key type")
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

func writeCAFile(t *testing.T, dir string, cert *x509.Certificate) string {
	t.Helper()
	path := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFileTLSMaterialsRotationAndFailClosed(t *testing.T) {
	pki := mustTestPKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeCertificateFiles(t, dir, "server", pki.ServerCert)
	caPath := writeCAFile(t, dir, pki.CA)
	denyPath := filepath.Join(dir, "denylist")
	if err := os.WriteFile(denyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	materials, err := transport.LoadServerTLSMaterials(certPath, keyPath, caPath, pki.NodeURI, denyPath)
	if err != nil {
		t.Fatal(err)
	}
	rotated := mustLeafCert(t, pki.CA, pki.CAKey, pki.NodeURI, nil, time.Now().Add(24*time.Hour))
	writeCertificateFiles(t, dir, "rotated", rotated)
	rotatedCertPath := filepath.Join(dir, "rotated.pem")
	rotatedKeyPath := filepath.Join(dir, "rotated-key.pem")
	rotatedCert, _ := os.ReadFile(rotatedCertPath)
	rotatedKey, _ := os.ReadFile(rotatedKeyPath)
	if err := os.WriteFile(certPath, rotatedCert, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, rotatedKey, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := materials.GetCertificate(nil)
	if err != nil || got.Leaf.SerialNumber.Cmp(rotated.Leaf.SerialNumber) != 0 {
		t.Fatalf("rotation got=%v err=%v", got, err)
	}
	if err := os.WriteFile(certPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := materials.GetCertificate(nil); err == nil {
		t.Fatal("partial rotation must fail closed")
	}
}

func TestClientCertificateRotationAndWrongURI(t *testing.T) {
	pki := mustTestPKI(t)
	dir := t.TempDir()
	certPath, keyPath := writeCertificateFiles(t, dir, "client", pki.ClientCert)
	caPath := writeCAFile(t, dir, pki.CA)
	denyPath := filepath.Join(dir, "denylist")
	_ = os.WriteFile(denyPath, nil, 0o600)
	materials, err := transport.LoadClientTLSMaterials(certPath, keyPath, caPath, pki.ControllerURI, "127.0.0.1", denyPath)
	if err != nil {
		t.Fatal(err)
	}
	wrong := mustLeafCert(t, pki.CA, pki.CAKey, pki.ControllerURI2, nil, time.Now().Add(time.Hour))
	wrongCert, wrongKey := writeCertificateFiles(t, dir, "wrong", wrong)
	data, _ := os.ReadFile(wrongCert)
	keyData, _ := os.ReadFile(wrongKey)
	_ = os.WriteFile(certPath, data, 0o600)
	_ = os.WriteFile(keyPath, keyData, 0o600)
	if _, err := materials.GetClientCertificate(nil); err == nil {
		t.Fatal("wrong controller URI accepted")
	}
}

func TestDynamicDenylistRevokedUnreadableAndInvalid(t *testing.T) {
	pki := mustTestPKI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "denylist")
	fingerprint := sha256.Sum256(pki.ClientCert.Leaf.Raw)
	if err := os.WriteFile(path, []byte("sha256:"+hex.EncodeToString(fingerprint[:])+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	verify, err := transport.NewDynamicDenylistVerifier(path)
	if err != nil {
		t.Fatal(err)
	}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{pki.ClientCert.Leaf}}
	if err := verify(state); err == nil {
		t.Fatal("revoked certificate accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := verify(state); err == nil {
		t.Fatal("unreadable denylist accepted")
	}
	if err := os.WriteFile(path, []byte("bad-entry"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.NewDynamicDenylistVerifier(path); err == nil {
		t.Fatal("invalid denylist accepted")
	}
}

func TestLoadCertPoolRejectsNoCertificate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.LoadCertPool(path); err == nil {
		t.Fatal("invalid CA accepted")
	}
}
