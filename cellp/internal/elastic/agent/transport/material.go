package transport

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"os"
	"strings"
)

// LoadCertPool loads a PEM CA bundle and rejects empty or malformed bundles.
func LoadCertPool(path string) (*x509.CertPool, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("ca_file_unreadable")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("ca_file_invalid")
	}
	return pool, nil
}

// LoadServerTLSMaterials loads initial material and reloads the leaf on every handshake.
func LoadServerTLSMaterials(certFile, keyFile, caFile, identityURI, denylistFile string) (TLSMaterials, error) {
	cert, err := loadIdentityKeyPair(certFile, keyFile, identityURI)
	if err != nil {
		return TLSMaterials{}, err
	}
	roots, err := LoadCertPool(caFile)
	if err != nil {
		return TLSMaterials{}, err
	}
	verify, err := NewDynamicDenylistVerifier(denylistFile)
	if err != nil {
		return TLSMaterials{}, err
	}
	return TLSMaterials{
		Cert: cert, RootCAs: roots, VerifyPeerConnection: verify,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			loaded, err := loadIdentityKeyPair(certFile, keyFile, identityURI)
			if err != nil {
				return nil, err
			}
			return &loaded, nil
		},
	}, nil
}

// LoadClientTLSMaterials loads initial material and reloads the controller leaf on every handshake.
func LoadClientTLSMaterials(certFile, keyFile, caFile, identityURI, serverName, denylistFile string) (TLSMaterials, error) {
	cert, err := loadIdentityKeyPair(certFile, keyFile, identityURI)
	if err != nil {
		return TLSMaterials{}, err
	}
	roots, err := LoadCertPool(caFile)
	if err != nil {
		return TLSMaterials{}, err
	}
	verify, err := NewDynamicDenylistVerifier(denylistFile)
	if err != nil {
		return TLSMaterials{}, err
	}
	return TLSMaterials{
		Cert: cert, RootCAs: roots, ServerName: serverName, VerifyPeerConnection: verify,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			loaded, err := loadIdentityKeyPair(certFile, keyFile, identityURI)
			if err != nil {
				return nil, err
			}
			return &loaded, nil
		},
	}, nil
}

func loadIdentityKeyPair(certFile, keyFile, identityURI string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return tls.Certificate{}, errors.New("certificate_keypair_invalid")
	}
	leaf, err := certificateLeaf(cert)
	if err != nil {
		return tls.Certificate{}, errors.New("certificate_leaf_invalid")
	}
	if err := validateTLSLeaf(leaf, identityURI); err != nil {
		return tls.Certificate{}, err
	}
	cert.Leaf = leaf
	return cert, nil
}

// NewDynamicDenylistVerifier validates a serial/SHA-256 denylist on every handshake.
// Lines are "serial:<hex>" or "sha256:<64 lowercase-or-uppercase hex>"; blank lines and # comments are ignored.
func NewDynamicDenylistVerifier(path string) (func(tls.ConnectionState) error, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("certificate_denylist_required")
	}
	if _, _, err := loadDenylist(path); err != nil {
		return nil, err
	}
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("peer_certificate_missing")
		}
		serials, fingerprints, err := loadDenylist(path)
		if err != nil {
			return err
		}
		leaf := state.PeerCertificates[0]
		serial := strings.ToLower(leaf.SerialNumber.Text(16))
		fingerprint := sha256.Sum256(leaf.Raw)
		if _, ok := serials[serial]; ok {
			return errors.New("peer_certificate_revoked")
		}
		if _, ok := fingerprints[hex.EncodeToString(fingerprint[:])]; ok {
			return errors.New("peer_certificate_revoked")
		}
		return nil
	}, nil
}

func loadDenylist(path string) (map[string]struct{}, map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, errors.New("certificate_denylist_unreadable")
	}
	serials := make(map[string]struct{})
	fingerprints := make(map[string]struct{})
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, value, ok := strings.Cut(line, ":")
		value = strings.ToLower(strings.TrimSpace(value))
		decoded, decodeErr := hex.DecodeString(value)
		if !ok || decodeErr != nil || len(decoded) == 0 {
			return nil, nil, errors.New("certificate_denylist_invalid")
		}
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "serial":
			serials[strings.TrimLeft(value, "0")] = struct{}{}
		case "sha256":
			if len(decoded) != sha256.Size {
				return nil, nil, errors.New("certificate_denylist_invalid")
			}
			fingerprints[value] = struct{}{}
		default:
			return nil, nil, errors.New("certificate_denylist_invalid")
		}
	}
	return serials, fingerprints, nil
}
