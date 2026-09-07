package contract

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// NodeRegistrationBinding is operator-configured trust for a runtime node (identity + advertised agent URL).
type NodeRegistrationBinding struct {
	IdentityURI  string
	AgentBaseURL string
	// AllowLoopback permits loopback/localhost agent hosts in this binding (test/dev only).
	AllowLoopback bool
}

// CanonicalAgentBaseURL parses and returns the exact https authority form used for trust comparisons.
func CanonicalAgentBaseURL(raw string) (string, error) {
	if len(raw) > MaxRuntimeNodeAgentBaseURLLen {
		return "", fmt.Errorf("agent base url too long")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("https agent base url required")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("agent base url components forbidden")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("agent base url host required")
	}
	if err := validateAgentBaseURLHostAlwaysForbidden(host); err != nil {
		return "", err
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return "https://" + strings.ToLower(host) + ":" + port, nil
}

// ValidateConfiguredAgentBaseURL checks an operator-authorized agent URL at config load time.
func ValidateConfiguredAgentBaseURL(raw string, allowLoopback bool) error {
	canonical, err := CanonicalAgentBaseURL(raw)
	if err != nil {
		return err
	}
	u, err := url.Parse(canonical)
	if err != nil {
		return err
	}
	return validateAgentBaseURLHost(u.Hostname(), allowLoopback)
}

func validateAgentBaseURLHostAlwaysForbidden(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("agent base url host forbidden")
		}
	}
	return nil
}

func validateAgentBaseURLHost(host string, allowLoopback bool) error {
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "" {
		return fmt.Errorf("agent base url host required")
	}
	if !allowLoopback {
		if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
			return fmt.Errorf("agent base url loopback forbidden")
		}
		if ip := net.ParseIP(host); ip != nil {
			if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
				ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				return fmt.Errorf("agent base url host forbidden")
			}
		}
	} else {
		if ip := net.ParseIP(host); ip != nil {
			if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				return fmt.Errorf("agent base url host forbidden")
			}
		}
	}
	return nil
}

// ValidateNodeRegistrationAllowlist ensures node_id↔identity and agent URL bijection with valid bindings.
func ValidateNodeRegistrationAllowlist(allowlist map[string]NodeRegistrationBinding) error {
	if len(allowlist) == 0 {
		return fmt.Errorf("node registration allowlist required")
	}
	seenURI := make(map[string]string, len(allowlist))
	seenAgent := make(map[string]string, len(allowlist))
	for nodeID, binding := range allowlist {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			return fmt.Errorf("node_id required in allowlist")
		}
		if len(nodeID) > MaxCommandFieldLen {
			return fmt.Errorf("node_id too long in allowlist")
		}
		identity := strings.TrimSpace(binding.IdentityURI)
		if err := validateSPIFFEIdentityURI(identity); err != nil {
			return fmt.Errorf("node %q: %w", nodeID, err)
		}
		if other, ok := seenURI[identity]; ok && other != nodeID {
			return fmt.Errorf("duplicate spiffe uri for nodes %q and %q", other, nodeID)
		}
		seenURI[identity] = nodeID
		canonical, err := CanonicalAgentBaseURL(binding.AgentBaseURL)
		if err != nil {
			return fmt.Errorf("node %q agent url: %w", nodeID, err)
		}
		if err := ValidateConfiguredAgentBaseURL(canonical, binding.AllowLoopback); err != nil {
			return fmt.Errorf("node %q agent url: %w", nodeID, err)
		}
		if other, ok := seenAgent[canonical]; ok && other != nodeID {
			return fmt.Errorf("duplicate agent base url for nodes %q and %q", other, nodeID)
		}
		seenAgent[canonical] = nodeID
	}
	return nil
}

// AgentBaseURLPolicyForBinding derives activation URL policy from the configured binding.
func AgentBaseURLPolicyForBinding(binding NodeRegistrationBinding) AgentBaseURLPolicy {
	return AgentBaseURLPolicy{AllowLoopback: binding.AllowLoopback}
}
