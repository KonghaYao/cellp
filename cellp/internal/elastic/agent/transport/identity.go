package transport

import (
	"crypto/x509"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ValidateIdentityURI ensures a SPIFFE-style URI for controller/node identity binding.
func ValidateIdentityURI(uri string) error {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return fmt.Errorf("identity uri required")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid identity uri")
	}
	if u.Scheme != "spiffe" || u.Host == "" || u.Path == "" || u.Path == "/" {
		return fmt.Errorf("invalid spiffe identity uri")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid spiffe identity uri")
	}
	return nil
}

// ValidateLeafCertificateValidity fail-closed on expired or not-yet-valid leaf certs.
func ValidateLeafCertificateValidity(leaf *x509.Certificate, now time.Time) error {
	if leaf == nil {
		return fmt.Errorf("missing certificate")
	}
	now = now.UTC()
	if now.Before(leaf.NotBefore.UTC()) {
		return fmt.Errorf("certificate not yet valid")
	}
	if !now.Before(leaf.NotAfter.UTC()) {
		return fmt.Errorf("certificate expired")
	}
	return nil
}

// MatchPeerURIPrincipal returns the single URI SAN that exactly matches allowlist.
// Zero or multiple matches fail closed (no CN fallback).
func MatchPeerURIPrincipal(cert *x509.Certificate, allowlist []string) (string, error) {
	if cert == nil {
		return "", fmt.Errorf("missing certificate")
	}
	allowed := make(map[string]struct{}, len(allowlist))
	for _, u := range allowlist {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		allowed[u] = struct{}{}
	}
	if len(allowed) == 0 {
		return "", fmt.Errorf("empty allowlist")
	}
	var matches []string
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		s := u.String()
		if _, ok := allowed[s]; ok {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no matching uri san")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous uri san")
	}
}

// LeafContainsURI reports whether the leaf certificate includes the expected URI SAN exactly.
func LeafContainsURI(cert *x509.Certificate, expectedURI string) error {
	expectedURI = strings.TrimSpace(expectedURI)
	if err := ValidateIdentityURI(expectedURI); err != nil {
		return err
	}
	if cert == nil {
		return fmt.Errorf("missing certificate")
	}
	count := 0
	for _, u := range cert.URIs {
		if u != nil && u.String() == expectedURI {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("node identity uri san mismatch")
	}
	return nil
}
