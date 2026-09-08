package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	envAgentCelldBindHost      = "CELLP_AGENT_CELLD_BIND_HOST"
	envAgentCelldAdvertiseHost = "CELLP_AGENT_CELLD_ADVERTISE_HOST"
	defaultCelldBindHost       = "127.0.0.1"
)

func loadCelldUpstreamFields(cfg *ElasticConfig) {
	rawBind := strings.TrimSpace(os.Getenv(envAgentCelldBindHost))
	if rawBind == "" {
		cfg.CelldBindHost = defaultCelldBindHost
	} else {
		cfg.CelldBindHost = rawBind
	}
	cfg.CelldAdvertiseHost = strings.TrimSpace(os.Getenv(envAgentCelldAdvertiseHost))
	if cfg.CelldAdvertiseHost == "" {
		cfg.CelldAdvertiseHost = cfg.CelldBindHost
	}
}

func validateCelldUpstreamResolved(bindHost, advertiseHost string, allowPrivateNAT bool) error {
	bind, err := validateCelldBindHost(bindHost)
	if err != nil {
		return fmt.Errorf("elastic config: celld bind host: %w", err)
	}
	advertiseRaw := strings.TrimSpace(advertiseHost)
	adv := bind
	if advertiseRaw != "" {
		adv, err = validateCelldAdvertiseHost(advertiseRaw, allowPrivateNAT)
		if err != nil {
			return fmt.Errorf("elastic config: celld advertise host: %w", err)
		}
	}
	if adv != bind && !allowPrivateNAT {
		return fmt.Errorf("elastic config: celld advertise host must match bind unless private nat is enabled")
	}
	return nil
}

func validateCelldUpstream(c *ElasticConfig) error {
	if c == nil {
		return fmt.Errorf("elastic config: celld upstream required")
	}
	bindHost, err := validateCelldBindHost(c.CelldBindHost)
	if err != nil {
		return fmt.Errorf("elastic config: celld bind host: %w", err)
	}
	c.CelldBindHost = bindHost

	advertiseRaw := strings.TrimSpace(c.CelldAdvertiseHost)
	if advertiseRaw == "" {
		c.CelldAdvertiseHost = bindHost
		return nil
	}
	advertiseHost, err := validateCelldAdvertiseHost(advertiseRaw, c.AllowPrivateNAT)
	if err != nil {
		return fmt.Errorf("elastic config: celld advertise host: %w", err)
	}
	c.CelldAdvertiseHost = advertiseHost
	if advertiseHost != bindHost && !c.AllowPrivateNAT {
		return fmt.Errorf("elastic config: celld advertise host must match bind unless private nat is enabled")
	}
	return nil
}

func validateCelldBindHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("required")
	}
	if strings.ContainsAny(host, "/@?#") || strings.Contains(host, "*") {
		return "", fmt.Errorf("url components or wildcards forbidden")
	}
	if _, port, err := net.SplitHostPort(host); err == nil && port != "" {
		return "", fmt.Errorf("host:port forbidden")
	}
	if strings.Contains(host, ":") {
		return "", fmt.Errorf("url components forbidden")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("must be private literal ip")
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return "", fmt.Errorf("unspecified or multicast forbidden")
	}
	if !isAllowedCelldLiteralIP(ip) {
		return "", fmt.Errorf("must be loopback or private")
	}
	return ip.String(), nil
}

func validateCelldAdvertiseHost(host string, allowPublicLiteralIP bool) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("required")
	}
	if strings.ContainsAny(host, "/@?#") || strings.Contains(host, "*") {
		return "", fmt.Errorf("url components or wildcards forbidden")
	}
	if _, port, err := net.SplitHostPort(host); err == nil && port != "" {
		return "", fmt.Errorf("host:port forbidden")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() || ip.IsMulticast() {
			return "", fmt.Errorf("unspecified or multicast forbidden")
		}
		if ip.IsLinkLocalMulticast() {
			return "", fmt.Errorf("link-local multicast forbidden")
		}
		if !allowPublicLiteralIP && !isAllowedCelldLiteralIP(ip) {
			return "", fmt.Errorf("public literal ip forbidden")
		}
		return ip.String(), nil
	}
	if err := validateCelldAdvertiseHostname(host); err != nil {
		return "", err
	}
	return strings.ToLower(host), nil
}

func validateCelldAdvertiseHostname(host string) error {
	if len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return fmt.Errorf("invalid hostname")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("invalid hostname label")
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
				continue
			}
			return fmt.Errorf("invalid hostname character")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("invalid hostname label")
		}
	}
	return nil
}

func isAllowedCelldLiteralIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || isCGNAT(ip)
}

func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}
