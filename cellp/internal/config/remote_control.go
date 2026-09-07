package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// RemoteControlConfig is controller-side mTLS surface for remote Node Agents (nodereg + registry relay).
// Paths only; never PEM contents.
type RemoteControlConfig struct {
	Enabled                 bool
	BindAddr                string
	ControllerIdentityURI   string
	ServerCertFile          string
	ServerKeyFile           string
	ServerCAFile            string
	CertificateDenylistFile string
	NodeAllowlist           map[string]contract.NodeRegistrationBinding
	NodeHeartbeatTTL        time.Duration
	MaxBodyBytes            int64
	ReplayMaxEntries        int
}

type remoteNodeAllowlistEntry struct {
	NodeID        string `json:"node_id"`
	IdentityURI   string `json:"identity_uri"`
	AgentBaseURL  string `json:"agent_base_url"`
	AllowLoopback bool   `json:"allow_loopback"`
}

// LoadRemoteControlConfig loads remote-control settings when elastic runtime is enabled.
// When elasticEnabled is false (legacy callers only), returns disabled config without reading env.
// When no remote-control env is set, returns disabled (quiescent).
// Partial remote-control env while elastic is on fails closed.
func LoadRemoteControlConfig(elasticEnabled bool) (RemoteControlConfig, error) {
	cfg := RemoteControlConfig{}
	if !elasticEnabled {
		return cfg, nil
	}
	keys := remoteControlEnvKeys()
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		values[key] = strings.TrimSpace(os.Getenv(key))
	}
	anySet := false
	for _, key := range keys {
		if key == envRemoteCertDenylist && values[key] == "" {
			continue
		}
		if values[key] != "" {
			anySet = true
			break
		}
	}
	if !anySet {
		return cfg, nil
	}
	for _, key := range keys {
		if key == envRemoteCertDenylist {
			continue
		}
		if values[key] == "" {
			return RemoteControlConfig{}, fmt.Errorf("remote control config: %s required", remoteControlFieldName(key))
		}
	}
	cfg.BindAddr = values[envRemoteBind]
	cfg.ControllerIdentityURI = values[envRemoteControllerURI]
	cfg.ServerCertFile = values[envRemoteServerCert]
	cfg.ServerKeyFile = values[envRemoteServerKey]
	cfg.ServerCAFile = values[envRemoteServerCA]
	cfg.CertificateDenylistFile = values[envRemoteCertDenylist]
	allowlist, err := loadNodeAllowlistFile(values[envRemoteAllowlistFile])
	if err != nil {
		return RemoteControlConfig{}, fmt.Errorf("remote control config: %w", err)
	}
	cfg.NodeAllowlist = allowlist
	if cfg.NodeHeartbeatTTL, err = time.ParseDuration(values[envRemoteHeartbeatTTL]); err != nil {
		return RemoteControlConfig{}, fmt.Errorf("remote control config: node_heartbeat_ttl invalid")
	}
	body, err := parsePositiveInt64(values[envRemoteMaxBody])
	if err != nil {
		return RemoteControlConfig{}, fmt.Errorf("remote control config: max_body_bytes invalid")
	}
	cfg.MaxBodyBytes = body
	replay, err := parsePositiveInt(values[envRemoteReplayMax])
	if err != nil {
		return RemoteControlConfig{}, fmt.Errorf("remote control config: replay_max_entries invalid")
	}
	cfg.ReplayMaxEntries = replay
	cfg.Enabled = true
	if err := cfg.Validate(); err != nil {
		return RemoteControlConfig{}, err
	}
	return cfg, nil
}

// Validate checks remote-control settings without reading TLS material.
func (c RemoteControlConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	for name, value := range map[string]string{
		"bind_addr": c.BindAddr, "controller_identity_uri": c.ControllerIdentityURI,
		"server_cert_file": c.ServerCertFile, "server_key_file": c.ServerKeyFile, "server_ca_file": c.ServerCAFile,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("remote control config: %s required", name)
		}
	}
	if err := validatePrivateBindAddr(c.BindAddr); err != nil {
		return fmt.Errorf("remote control config: %w", err)
	}
	if err := validateIdentityURI(c.ControllerIdentityURI); err != nil {
		return fmt.Errorf("remote control config: controller identity: %w", err)
	}
	if err := contract.ValidateNodeRegistrationAllowlist(c.NodeAllowlist); err != nil {
		return fmt.Errorf("remote control config: node allowlist: %w", err)
	}
	if c.NodeHeartbeatTTL <= 0 || c.MaxBodyBytes <= 0 || c.ReplayMaxEntries <= 0 {
		return fmt.Errorf("remote control config: limits must be positive")
	}
	return nil
}

// NodeIdentityAllowlist returns node_id -> identity URI for registry relay mTLS.
func (c RemoteControlConfig) NodeIdentityAllowlist() map[string]string {
	out := make(map[string]string, len(c.NodeAllowlist))
	for nodeID, binding := range c.NodeAllowlist {
		out[nodeID] = strings.TrimSpace(binding.IdentityURI)
	}
	return out
}

// AllowedNodeURIs returns distinct SPIFFE URIs trusted as runtime node clients.
func (c RemoteControlConfig) AllowedNodeURIs() []string {
	seen := make(map[string]struct{}, len(c.NodeAllowlist))
	var out []string
	for _, binding := range c.NodeAllowlist {
		uri := strings.TrimSpace(binding.IdentityURI)
		if uri == "" {
			continue
		}
		if _, ok := seen[uri]; ok {
			continue
		}
		seen[uri] = struct{}{}
		out = append(out, uri)
	}
	return out
}

func loadNodeAllowlistFile(path string) (map[string]contract.NodeRegistrationBinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("node allowlist file unreadable")
	}
	var entries []remoteNodeAllowlistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("node allowlist file invalid")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("node allowlist file empty")
	}
	out := make(map[string]contract.NodeRegistrationBinding, len(entries))
	for _, e := range entries {
		nodeID := strings.TrimSpace(e.NodeID)
		if nodeID == "" {
			return nil, fmt.Errorf("node_id required in allowlist file")
		}
		if _, exists := out[nodeID]; exists {
			return nil, fmt.Errorf("duplicate node_id in allowlist file")
		}
		out[nodeID] = contract.NodeRegistrationBinding{
			IdentityURI:   strings.TrimSpace(e.IdentityURI),
			AgentBaseURL:  strings.TrimSpace(e.AgentBaseURL),
			AllowLoopback: e.AllowLoopback,
		}
	}
	return out, nil
}

func parsePositiveInt64(raw string) (int64, error) {
	v, err := parsePositiveInt(raw)
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

func parsePositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid")
	}
	return int(v), nil
}

const (
	envRemoteBind           = "CELLP_CONTROLLER_REMOTE_BIND_ADDR"
	envRemoteControllerURI  = "CELLP_CONTROLLER_IDENTITY_URI"
	envRemoteServerCert     = "CELLP_CONTROLLER_REMOTE_SERVER_CERT_FILE"
	envRemoteServerKey      = "CELLP_CONTROLLER_REMOTE_SERVER_KEY_FILE"
	envRemoteServerCA       = "CELLP_CONTROLLER_REMOTE_SERVER_CA_FILE"
	envRemoteCertDenylist   = "CELLP_CONTROLLER_REMOTE_CERT_DENYLIST_FILE"
	envRemoteAllowlistFile  = "CELLP_CONTROLLER_NODE_ALLOWLIST_FILE"
	envRemoteHeartbeatTTL   = "CELLP_CONTROLLER_REMOTE_NODE_HEARTBEAT_TTL"
	envRemoteMaxBody        = "CELLP_CONTROLLER_REMOTE_MAX_BODY_BYTES"
	envRemoteReplayMax      = "CELLP_CONTROLLER_REMOTE_REPLAY_MAX_ENTRIES"
)

func remoteControlEnvKeys() []string {
	return []string{
		envRemoteBind, envRemoteControllerURI, envRemoteServerCert, envRemoteServerKey, envRemoteServerCA,
		envRemoteCertDenylist, envRemoteAllowlistFile, envRemoteHeartbeatTTL, envRemoteMaxBody, envRemoteReplayMax,
	}
}

func remoteControlFieldName(key string) string {
	switch key {
	case envRemoteBind:
		return "bind_addr"
	case envRemoteControllerURI:
		return "controller_identity_uri"
	case envRemoteServerCert:
		return "server_cert_file"
	case envRemoteServerKey:
		return "server_key_file"
	case envRemoteServerCA:
		return "server_ca_file"
	case envRemoteAllowlistFile:
		return "node_allowlist_file"
	case envRemoteHeartbeatTTL:
		return "node_heartbeat_ttl"
	case envRemoteMaxBody:
		return "max_body_bytes"
	case envRemoteReplayMax:
		return "replay_max_entries"
	default:
		return key
	}
}
