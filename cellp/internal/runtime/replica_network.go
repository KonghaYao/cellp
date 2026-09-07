package runtime

import (
	"net"
	"strconv"
)

// ReplicaHostConfig controls elastic replica celld bind vs controller-visible advertise host.
// Values must be validated by config before use.
type ReplicaHostConfig struct {
	BindHost      string
	AdvertiseHost string
}

// DefaultReplicaHostConfig is loopback-only (safe default listen surface).
func DefaultReplicaHostConfig() ReplicaHostConfig {
	return ReplicaHostConfig{BindHost: "127.0.0.1", AdvertiseHost: "127.0.0.1"}
}

// SetReplicaHostConfig applies validated elastic replica network settings.
func (m *Manager) SetReplicaHostConfig(c ReplicaHostConfig) {
	m.replicaHosts = c
}

func (m *Manager) replicaBindAdvertise() (bind string, advertise string) {
	c := m.replicaHosts
	if c.BindHost == "" && c.AdvertiseHost == "" {
		c = DefaultReplicaHostConfig()
	}
	bind = c.BindHost
	if bind == "" {
		bind = "127.0.0.1"
	}
	advertise = c.AdvertiseHost
	if advertise == "" {
		advertise = bind
	}
	return bind, advertise
}

func externalReplicaHost(bindHost, advertiseHost string) string {
	if advertiseHost != "" {
		return advertiseHost
	}
	return bindHost
}

func (p *celldProc) bindHostOrDefault() string {
	if p == nil || p.bindHost == "" {
		return "127.0.0.1"
	}
	return p.bindHost
}

func (p *celldProc) externalHost() string {
	if p == nil {
		return "127.0.0.1"
	}
	return externalReplicaHost(p.bindHost, p.advertiseHost)
}

func celldListenFlag(bindHost string, port int) string {
	return net.JoinHostPort(bindHost, strconv.Itoa(port))
}
