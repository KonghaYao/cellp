package transport

import (
	"fmt"
	"net"
	"strings"
)

// ValidateInternalBindAddr accepts only literal internal IPs (loopback, private, link-local).
// Wildcards, hostnames, and public routable addresses are rejected.
func ValidateInternalBindAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bind address: %w", err)
	}
	if strings.TrimSpace(port) == "" {
		return fmt.Errorf("bind port required")
	}
	if host == "" || host == "*" || host == "0.0.0.0" || host == "::" {
		return fmt.Errorf("wildcard bind forbidden")
	}
	if strings.Contains(host, "%") {
		// Scoped literals are not accepted for agent listener binding.
		return fmt.Errorf("scoped address bind forbidden")
	}
	if strings.ContainsAny(host, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return fmt.Errorf("hostname bind forbidden")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("invalid bind ip")
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("unspecified bind forbidden")
	}
	if ip.IsGlobalUnicast() && !isPrivateOrSpecial(ip) {
		return fmt.Errorf("public bind forbidden")
	}
	if !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !isPrivateOrSpecial(ip) {
		return fmt.Errorf("bind not internal")
	}
	return nil
}

func isPrivateOrSpecial(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	// CGNAT 100.64.0.0/10
	if v4 := ip.To4(); v4 != nil {
		return v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
	}
	return false
}
