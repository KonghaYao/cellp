package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

const (
	defaultMaxBodyBytes  = 1 << 20 // 1 MiB
	defaultReadTimeout   = 90 * time.Second
	defaultWriteTimeout  = 90 * time.Second
	defaultIdleTimeout   = 60 * time.Second
	defaultClientTimeout = 90 * time.Second
	maxIdempotencyKeyLen = 128
	headerIdempotencyKey = "Cellp-Idempotency-Key"
)

// TLSMaterials holds in-memory PKIX material (no paths; no secret logging).
type TLSMaterials struct {
	Cert       tls.Certificate
	RootCAs    *x509.CertPool
	ServerName string // DNS or IP for client verification
	// VerifyPeerConnection is an optional hook after PKIX for rotation/revocation policy.
	VerifyPeerConnection func(tls.ConnectionState) error
	// GetCertificate is an optional hook for serving rotated leaf certificates.
	GetCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	// GetClientCertificate is an optional hook for presenting rotated controller certificates.
	GetClientCertificate func(*tls.CertificateRequestInfo) (*tls.Certificate, error)
}

// AuditEvent is bounded lifecycle metadata; request bodies, nonce, idempotency keys, buckets and certificates are excluded.
type AuditEvent struct {
	Action     contract.LifecycleAction
	Reason     contract.ReasonCode
	Principal  string
	NodeID     string
	ReplicaID  string
	Generation int64
}

// ServerConfig configures the Node Agent HTTPS listener.
type ServerConfig struct {
	Enabled               bool
	NodeID                string
	BindAddr              string
	NodeIdentityURI       string
	AllowedControllerURIs []string
	TLS                   TLSMaterials
	MaxBodyBytes          int64
	ReplayMaxEntries      int
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	Audit                 func(AuditEvent)
}

func validateTLSLeaf(leaf *x509.Certificate, identityURI string) error {
	if err := ValidateLeafCertificateValidity(leaf, time.Now().UTC()); err != nil {
		return err
	}
	return LeafContainsURI(leaf, identityURI)
}

func validateControllerAllowlist(uris []string) error {
	if len(uris) == 0 {
		return fmt.Errorf("allowed_controller_uris required")
	}
	seen := make(map[string]struct{}, len(uris))
	for _, u := range uris {
		u = strings.TrimSpace(u)
		if u == "" {
			return fmt.Errorf("empty controller uri in allowlist")
		}
		if err := ValidateIdentityURI(u); err != nil {
			return fmt.Errorf("controller uri: %w", err)
		}
		if _, ok := seen[u]; ok {
			return fmt.Errorf("duplicate controller uri in allowlist")
		}
		seen[u] = struct{}{}
	}
	return nil
}

// ValidateServerConfig checks bind, identity, and TLS prerequisites.
func ValidateServerConfig(cfg ServerConfig) error {
	if strings.TrimSpace(cfg.NodeID) == "" {
		return fmt.Errorf("node_id required")
	}
	if err := ValidateInternalBindAddr(cfg.BindAddr); err != nil {
		return err
	}
	if err := ValidateIdentityURI(cfg.NodeIdentityURI); err != nil {
		return fmt.Errorf("node_identity_uri: %w", err)
	}
	if err := validateControllerAllowlist(cfg.AllowedControllerURIs); err != nil {
		return err
	}
	if len(cfg.TLS.Cert.Certificate) == 0 {
		return errors.New("server tls certificate required")
	}
	leaf, err := x509.ParseCertificate(cfg.TLS.Cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("server leaf: %w", err)
	}
	if err := validateTLSLeaf(leaf, cfg.NodeIdentityURI); err != nil {
		return err
	}
	if cfg.TLS.RootCAs == nil {
		return errors.New("server root cas required")
	}
	return nil
}

// ClientConfig configures the controller-side HTTPS client.
type ClientConfig struct {
	BaseURL               string
	ExpectedNodeURI       string
	ControllerIdentityURI string
	TLS                   TLSMaterials
	Timeout               time.Duration
	MaxBodyBytes          int64
}

// ValidateClientConfig ensures HTTPS-only base URL and TLS settings.
func ValidateClientConfig(cfg ClientConfig) error {
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil {
		return fmt.Errorf("base url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("https required")
	}
	if u.Host == "" {
		return fmt.Errorf("base url host required")
	}
	if u.User != nil {
		return fmt.Errorf("userinfo forbidden in base url")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("base url path forbidden")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("base url query and fragment forbidden")
	}
	if err := ValidateIdentityURI(cfg.ExpectedNodeURI); err != nil {
		return fmt.Errorf("expected_node_uri: %w", err)
	}
	if err := ValidateIdentityURI(cfg.ControllerIdentityURI); err != nil {
		return fmt.Errorf("controller_identity_uri: %w", err)
	}
	if len(cfg.TLS.Cert.Certificate) == 0 {
		return errors.New("client tls certificate required")
	}
	leaf, err := x509.ParseCertificate(cfg.TLS.Cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("client leaf: %w", err)
	}
	if err := validateTLSLeaf(leaf, cfg.ControllerIdentityURI); err != nil {
		return err
	}
	if cfg.TLS.RootCAs == nil {
		return errors.New("client root cas required")
	}
	if strings.TrimSpace(cfg.TLS.ServerName) == "" {
		return fmt.Errorf("tls server name required")
	}
	return nil
}

func normalizeServerConfig(cfg ServerConfig) ServerConfig {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	if cfg.ReplayMaxEntries <= 0 {
		cfg.ReplayMaxEntries = 4096
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = defaultReadTimeout
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defaultIdleTimeout
	}
	return cfg
}

func normalizeClientConfig(cfg ClientConfig) ClientConfig {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultClientTimeout
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	return cfg
}

func buildServerTLSConfig(mat TLSMaterials, identityURI string) *tls.Config {
	return buildServerTLSConfigExported(mat, identityURI)
}

// BuildServerTLSConfig constructs TLS 1.3 server settings for Node Agent or controller internal listeners.
func BuildServerTLSConfig(mat TLSMaterials, identityURI string) *tls.Config {
	return buildServerTLSConfigExported(mat, identityURI)
}

func buildServerTLSConfigExported(mat TLSMaterials, identityURI string) *tls.Config {
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{mat.Cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    mat.RootCAs,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no peer certificate")
			}
			if err := ValidateLeafCertificateValidity(cs.PeerCertificates[0], time.Now().UTC()); err != nil {
				return err
			}
			if mat.VerifyPeerConnection != nil {
				return mat.VerifyPeerConnection(cs)
			}
			return nil
		},
	}
	if mat.GetCertificate != nil {
		provider := mat.GetCertificate
		cfg.Certificates = nil
		cfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert, err := provider(hello)
			if err != nil {
				return nil, err
			}
			if cert == nil {
				return nil, errors.New("server certificate provider returned nil")
			}
			leaf, err := certificateLeaf(*cert)
			if err != nil {
				return nil, err
			}
			if err := validateTLSLeaf(leaf, identityURI); err != nil {
				return nil, err
			}
			return cert, nil
		}
	}
	return cfg
}

func certificateLeaf(cert tls.Certificate) (*x509.Certificate, error) {
	if cert.Leaf != nil {
		return cert.Leaf, nil
	}
	if len(cert.Certificate) == 0 {
		return nil, errors.New("certificate provider returned empty certificate")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("certificate provider leaf: %w", err)
	}
	return leaf, nil
}

func buildClientTLSConfig(mat TLSMaterials, expectedPeerURI, clientIdentityURI string) *tls.Config {
	return buildClientTLSConfigExported(mat, expectedPeerURI, clientIdentityURI)
}

// BuildClientTLSConfig verifies the peer leaf URI and presents clientIdentityURI on the client cert.
func BuildClientTLSConfig(mat TLSMaterials, expectedPeerURI, clientIdentityURI string) *tls.Config {
	return buildClientTLSConfigExported(mat, expectedPeerURI, clientIdentityURI)
}

func buildClientTLSConfigExported(mat TLSMaterials, expectedPeerURI, clientIdentityURI string) *tls.Config {
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      mat.RootCAs,
		Certificates: []tls.Certificate{mat.Cert},
		ServerName:   mat.ServerName,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no peer certificate")
			}
			if err := LeafContainsURI(cs.PeerCertificates[0], expectedPeerURI); err != nil {
				return err
			}
			if mat.VerifyPeerConnection != nil {
				return mat.VerifyPeerConnection(cs)
			}
			return nil
		},
	}
	if mat.GetClientCertificate != nil {
		provider := mat.GetClientCertificate
		cfg.GetClientCertificate = func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert, err := provider(info)
			if err != nil {
				return nil, err
			}
			if cert == nil {
				return nil, errors.New("client certificate provider returned nil")
			}
			leaf, err := certificateLeaf(*cert)
			if err != nil {
				return nil, err
			}
			if err := validateTLSLeaf(leaf, clientIdentityURI); err != nil {
				return nil, err
			}
			return cert, nil
		}
		cfg.Certificates = nil
	}
	return cfg
}
