package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// ElasticConfig is the internal Node Agent configuration. It contains paths, never PEM contents.
type ElasticConfig struct {
	Enabled bool
	// AgentEmbedded is read for cellp serve only (CELLP_AGENT_EMBEDDED). Standalone cellp agent ignores it.
	AgentEmbedded           bool
	NodeID                  string
	AgentBindAddr           string
	AgentBaseURL            string
	AllowPrivateNAT         bool
	CelldBindHost           string
	CelldAdvertiseHost      string
	NodeIdentityURI         string
	ControllerIdentityURI   string
	AllowedControllerURIs   []string
	ServerCertFile          string
	ServerKeyFile           string
	ServerCAFile            string
	ClientCertFile          string
	ClientKeyFile           string
	ClientCAFile            string
	TLSServerName           string
	CertificateDenylistFile string
	CapacityUnits           int
	Zone                    string
	NodeHeartbeatTTL        time.Duration
	NodeHeartbeatInterval   time.Duration
	AgentReconcileInterval  time.Duration
	MaxBodyBytes            int64
	ReplayMaxEntries        int

	// Standalone remote Node Agent (cellp agent); not used by embedded cellpd agent.
	NodeRegBaseURL          string
	RegistryRelayBaseURL    string
	ControllerTLSServerName string
	RelayScopeTTL           time.Duration
}

// LoadElasticConfig loads elastic settings for cellp serve.
func LoadElasticConfig() (ElasticConfig, error) {
	if _, err := contract.ParseElasticRuntimeEnv(); err != nil {
		return ElasticConfig{}, fmt.Errorf("elastic config: %w", err)
	}
	cfg := ElasticConfig{Enabled: true}
	embedded, err := requiredBool("CELLP_AGENT_EMBEDDED", true)
	if err != nil {
		return ElasticConfig{}, err
	}
	cfg.AgentEmbedded = embedded
	if cfg.AgentEmbedded {
		if err := loadEmbeddedAgentFields(&cfg); err != nil {
			return ElasticConfig{}, err
		}
		if err := cfg.ValidateEmbedded(); err != nil {
			return ElasticConfig{}, err
		}
		if err := validateCelldUpstream(&cfg); err != nil {
			return ElasticConfig{}, err
		}
	} else {
		if err := loadControllerOutboundFields(&cfg); err != nil {
			return ElasticConfig{}, err
		}
		if err := cfg.ValidateControllerOutbound(); err != nil {
			return ElasticConfig{}, err
		}
	}
	if err := cfg.validateStandalonePartial(); err != nil {
		return ElasticConfig{}, err
	}
	return cfg, nil
}

// LoadStandaloneAgentConfig loads agent config for the standalone cellp agent process.
func LoadStandaloneAgentConfig() (ElasticConfig, error) {
	if _, err := contract.ParseElasticRuntimeEnv(); err != nil {
		return ElasticConfig{}, fmt.Errorf("elastic config: %w", err)
	}
	cfg := ElasticConfig{Enabled: true, AgentEmbedded: true}
	if err := loadEmbeddedAgentFields(&cfg); err != nil {
		return ElasticConfig{}, err
	}
	if err := cfg.ValidateEmbedded(); err != nil {
		return ElasticConfig{}, err
	}
	if err := validateCelldUpstream(&cfg); err != nil {
		return ElasticConfig{}, err
	}
	if err := cfg.ValidateStandalone(); err != nil {
		return ElasticConfig{}, err
	}
	return cfg, nil
}

// ValidateServeElastic checks cellp serve elastic + remote-control combinations before listeners start.
func ValidateServeElastic(elastic ElasticConfig, remote RemoteControlConfig) error {
	if !elastic.AgentEmbedded && !remote.Enabled {
		return fmt.Errorf("elastic config: remote control required when embedded agent disabled")
	}
	return elastic.ValidateControllerIdentityMatchesRemote(remote)
}

// ValidateControllerIdentityMatchesRemote ensures scheduler outbound identity matches remote-control.
func (c ElasticConfig) ValidateControllerIdentityMatchesRemote(remote RemoteControlConfig) error {
	if !c.Enabled || !remote.Enabled {
		return nil
	}
	if strings.TrimSpace(c.ControllerIdentityURI) != strings.TrimSpace(remote.ControllerIdentityURI) {
		return fmt.Errorf("elastic config: controller identity must match remote control controller identity")
	}
	return nil
}

// ValidateEmbedded checks every required embedded Node Agent setting without reading TLS material.
func (c ElasticConfig) ValidateEmbedded() error {
	if !c.Enabled {
		return nil
	}
	for name, value := range map[string]string{
		"node_id": c.NodeID, "advertise_url": c.AgentBaseURL, "node_identity_uri": c.NodeIdentityURI,
		"controller_identity_uri": c.ControllerIdentityURI, "server_cert_file": c.ServerCertFile,
		"server_key_file": c.ServerKeyFile, "server_ca_file": c.ServerCAFile, "client_cert_file": c.ClientCertFile,
		"client_key_file": c.ClientKeyFile, "client_ca_file": c.ClientCAFile, "tls_server_name": c.TLSServerName,
		"certificate_denylist_file": c.CertificateDenylistFile, "zone": c.Zone,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("elastic config: %s required", name)
		}
	}
	if err := validatePrivateBindAddr(c.AgentBindAddr); err != nil {
		return fmt.Errorf("elastic config: %w", err)
	}
	if err := validateIdentityURI(c.NodeIdentityURI); err != nil {
		return fmt.Errorf("elastic config: node identity: %w", err)
	}
	if err := validateIdentityURI(c.ControllerIdentityURI); err != nil {
		return fmt.Errorf("elastic config: controller identity: %w", err)
	}
	if len(c.AllowedControllerURIs) == 0 {
		return fmt.Errorf("elastic config: controller allowlist required")
	}
	for _, identity := range c.AllowedControllerURIs {
		if err := validateIdentityURI(identity); err != nil {
			return fmt.Errorf("elastic config: controller allowlist: %w", err)
		}
	}
	allowlisted := false
	for _, identity := range c.AllowedControllerURIs {
		if identity == c.ControllerIdentityURI {
			allowlisted = true
			break
		}
	}
	if !allowlisted {
		return fmt.Errorf("elastic config: controller identity must appear in allowlist")
	}
	if c.CapacityUnits <= 0 || c.MaxBodyBytes <= 0 || c.ReplayMaxEntries <= 0 {
		return fmt.Errorf("elastic config: capacity and limits must be positive")
	}
	if c.NodeHeartbeatTTL <= 0 || c.NodeHeartbeatInterval <= 0 || c.AgentReconcileInterval <= 0 {
		return fmt.Errorf("elastic config: intervals must be positive")
	}
	if c.NodeHeartbeatInterval >= c.NodeHeartbeatTTL {
		return fmt.Errorf("elastic config: heartbeat interval must be less than ttl")
	}
	if err := validateAdvertise(c); err != nil {
		return err
	}
	return validateCelldUpstreamResolved(c.CelldBindHost, c.CelldAdvertiseHost, c.AllowPrivateNAT)
}

// ValidateControllerOutbound checks scheduler→Node Agent mTLS client settings (transport.ClientConfig).
func (c ElasticConfig) ValidateControllerOutbound() error {
	if !c.Enabled {
		return nil
	}
	for name, value := range map[string]string{
		"controller_identity_uri":   c.ControllerIdentityURI,
		"client_cert_file":          c.ClientCertFile,
		"client_key_file":           c.ClientKeyFile,
		"client_ca_file":            c.ClientCAFile,
		"certificate_denylist_file": c.CertificateDenylistFile,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("elastic config: %s required", name)
		}
	}
	if err := validateIdentityURI(c.ControllerIdentityURI); err != nil {
		return fmt.Errorf("elastic config: controller identity: %w", err)
	}
	if c.MaxBodyBytes <= 0 {
		return fmt.Errorf("elastic config: max_body_bytes must be positive")
	}
	return nil
}

// Validate is an alias for ValidateEmbedded (embedded Node Agent surface).
func (c ElasticConfig) Validate() error {
	return c.ValidateEmbedded()
}

// ValidateStandalone checks remote controller endpoints required by cellp agent.
func (c ElasticConfig) ValidateStandalone() error {
	if !c.Enabled {
		return nil
	}
	if err := c.ValidateEmbedded(); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"nodereg_base_url": c.NodeRegBaseURL, "registry_relay_base_url": c.RegistryRelayBaseURL,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("elastic config: %s required for standalone agent", name)
		}
	}
	if c.RelayScopeTTL <= 0 {
		return fmt.Errorf("elastic config: relay_scope_ttl required for standalone agent")
	}
	if c.RelayScopeTTL > contract.MaxControlPlaneScopeTTL {
		return fmt.Errorf("elastic config: relay scope ttl exceeds maximum")
	}
	if err := validateControllerHTTPSURL(c.NodeRegBaseURL); err != nil {
		return fmt.Errorf("elastic config: nodereg base url: %w", err)
	}
	if err := validateControllerHTTPSURL(c.RegistryRelayBaseURL); err != nil {
		return fmt.Errorf("elastic config: registry relay base url: %w", err)
	}
	if c.ResolvedControllerTLSServerName() == "" {
		return fmt.Errorf("elastic config: controller tls server name required")
	}
	return nil
}

func loadControllerOutboundFields(cfg *ElasticConfig) error {
	cfg.ControllerIdentityURI = strings.TrimSpace(os.Getenv("CELLP_AGENT_CONTROLLER_IDENTITY_URI"))
	cfg.ClientCertFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CLIENT_CERT_FILE"))
	cfg.ClientKeyFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CLIENT_KEY_FILE"))
	cfg.ClientCAFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CLIENT_CA_FILE"))
	cfg.CertificateDenylistFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CERT_DENYLIST_FILE"))
	body, err := requiredInt64("CELLP_AGENT_MAX_BODY_BYTES")
	if err != nil {
		return err
	}
	cfg.MaxBodyBytes = body
	return nil
}

func loadEmbeddedAgentFields(cfg *ElasticConfig) error {
	var err error
	cfg.NodeID = strings.TrimSpace(os.Getenv("CELLP_AGENT_NODE_ID"))
	cfg.AgentBindAddr = strings.TrimSpace(os.Getenv("CELLP_AGENT_BIND_ADDR"))
	cfg.AgentBaseURL = strings.TrimSpace(os.Getenv("CELLP_AGENT_ADVERTISE_URL"))
	cfg.AllowPrivateNAT, err = requiredBool("CELLP_AGENT_ALLOW_PRIVATE_NAT", false)
	if err != nil {
		return err
	}
	cfg.NodeIdentityURI = strings.TrimSpace(os.Getenv("CELLP_AGENT_NODE_IDENTITY_URI"))
	cfg.ControllerIdentityURI = strings.TrimSpace(os.Getenv("CELLP_AGENT_CONTROLLER_IDENTITY_URI"))
	cfg.AllowedControllerURIs = splitList(os.Getenv("CELLP_AGENT_ALLOWED_CONTROLLER_URIS"))
	cfg.ServerCertFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_SERVER_CERT_FILE"))
	cfg.ServerKeyFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_SERVER_KEY_FILE"))
	cfg.ServerCAFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_SERVER_CA_FILE"))
	cfg.ClientCertFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CLIENT_CERT_FILE"))
	cfg.ClientKeyFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CLIENT_KEY_FILE"))
	cfg.ClientCAFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CLIENT_CA_FILE"))
	cfg.TLSServerName = strings.TrimSpace(os.Getenv("CELLP_AGENT_TLS_SERVER_NAME"))
	cfg.CertificateDenylistFile = strings.TrimSpace(os.Getenv("CELLP_AGENT_CERT_DENYLIST_FILE"))
	cfg.Zone = strings.TrimSpace(os.Getenv("CELLP_AGENT_ZONE"))
	if cfg.CapacityUnits, err = requiredInt("CELLP_AGENT_CAPACITY_UNITS"); err != nil {
		return err
	}
	if cfg.NodeHeartbeatTTL, err = requiredDuration("CELLP_AGENT_HEARTBEAT_TTL"); err != nil {
		return err
	}
	if cfg.NodeHeartbeatInterval, err = requiredDuration("CELLP_AGENT_HEARTBEAT_INTERVAL"); err != nil {
		return err
	}
	if cfg.AgentReconcileInterval, err = requiredDuration("CELLP_AGENT_RECONCILE_INTERVAL"); err != nil {
		return err
	}
	body, err := requiredInt64("CELLP_AGENT_MAX_BODY_BYTES")
	if err != nil {
		return err
	}
	cfg.MaxBodyBytes = body
	if cfg.ReplayMaxEntries, err = requiredInt("CELLP_AGENT_REPLAY_MAX_ENTRIES"); err != nil {
		return err
	}
	loadCelldUpstreamFields(cfg)
	return loadStandaloneAgentFieldsFromEnv(cfg)
}

// ResolvedControllerTLSServerName returns the TLS SNI for controller HTTPS clients.
func (c ElasticConfig) ResolvedControllerTLSServerName() string {
	if s := strings.TrimSpace(c.ControllerTLSServerName); s != "" {
		return s
	}
	u, err := url.Parse(strings.TrimSpace(c.NodeRegBaseURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

const (
	envAgentNodeRegBaseURL          = "CELLP_AGENT_NODEREG_BASE_URL"
	envAgentRegistryRelayBaseURL    = "CELLP_AGENT_REGISTRY_RELAY_BASE_URL"
	envAgentControllerTLSServerName = "CELLP_AGENT_CONTROLLER_TLS_SERVER_NAME"
	envAgentRelayScopeTTL           = "CELLP_AGENT_RELAY_SCOPE_TTL"
)

func standaloneAgentPresenceEnvKeys() []string {
	return []string{
		envAgentNodeRegBaseURL,
		envAgentRegistryRelayBaseURL,
		envAgentControllerTLSServerName,
		envAgentRelayScopeTTL,
	}
}

func standaloneAgentRequiredEnvKeys() []string {
	return []string{
		envAgentNodeRegBaseURL,
		envAgentRegistryRelayBaseURL,
		envAgentRelayScopeTTL,
	}
}

func loadStandaloneAgentFieldsFromEnv(cfg *ElasticConfig) error {
	cfg.NodeRegBaseURL = strings.TrimSpace(os.Getenv(envAgentNodeRegBaseURL))
	cfg.RegistryRelayBaseURL = strings.TrimSpace(os.Getenv(envAgentRegistryRelayBaseURL))
	cfg.ControllerTLSServerName = strings.TrimSpace(os.Getenv(envAgentControllerTLSServerName))
	raw := strings.TrimSpace(os.Getenv(envAgentRelayScopeTTL))
	if raw == "" {
		cfg.RelayScopeTTL = 0
		return nil
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("elastic config: relay scope ttl invalid")
	}
	cfg.RelayScopeTTL = ttl
	return nil
}

// probeStandaloneAgentEnv scans standalone-only env for presence (not cfg fields).
func probeStandaloneAgentEnv() (anySet bool, complete bool, err error) {
	values := make(map[string]string, len(standaloneAgentPresenceEnvKeys()))
	for _, key := range standaloneAgentPresenceEnvKeys() {
		values[key] = strings.TrimSpace(os.Getenv(key))
		if values[key] != "" {
			anySet = true
		}
	}
	if !anySet {
		return false, false, nil
	}
	for _, key := range standaloneAgentRequiredEnvKeys() {
		if values[key] == "" {
			return true, false, nil
		}
	}
	if _, err := time.ParseDuration(values[envAgentRelayScopeTTL]); err != nil {
		return true, false, fmt.Errorf("elastic config: relay scope ttl invalid")
	}
	return true, true, nil
}

func (c ElasticConfig) validateStandalonePartial() error {
	if !c.Enabled {
		return nil
	}
	anySet, complete, err := probeStandaloneAgentEnv()
	if err != nil {
		return err
	}
	if !anySet {
		return nil
	}
	if !complete {
		return fmt.Errorf("elastic config: incomplete standalone agent settings")
	}
	if !c.AgentEmbedded {
		return fmt.Errorf("elastic config: standalone agent settings must not be set for controller-only cellp serve")
	}
	return c.ValidateStandalone()
}

func validateControllerHTTPSURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("https url required")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("url components forbidden")
	}
	return nil
}

func validatePrivateBindAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return fmt.Errorf("invalid agent bind address")
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() || (!ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast()) {
		return fmt.Errorf("agent bind must be private literal")
	}
	return nil
}

func validateIdentityURI(identity string) error {
	u, err := url.Parse(strings.TrimSpace(identity))
	if err != nil || u.Scheme != "spiffe" || u.Host == "" || u.Path == "" || u.Path == "/" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid spiffe identity uri")
	}
	return nil
}

func validateAdvertise(c ElasticConfig) error {
	u, err := url.Parse(c.AgentBaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("elastic config: https advertise url required")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("elastic config: advertise url components forbidden")
	}
	advertiseHost := net.ParseIP(u.Hostname())
	if advertiseHost == nil || (!advertiseHost.IsLoopback() && !advertiseHost.IsPrivate() && !advertiseHost.IsLinkLocalUnicast()) {
		return fmt.Errorf("elastic config: advertise host must be private literal")
	}
	bindHost, bindPort, err := net.SplitHostPort(c.AgentBindAddr)
	if err != nil {
		return fmt.Errorf("elastic config: invalid bind")
	}
	advertisePort := u.Port()
	if advertisePort == "" {
		advertisePort = "443"
	}
	if advertisePort != bindPort {
		return fmt.Errorf("elastic config: advertise port must match bind")
	}
	if c.TLSServerName != u.Hostname() {
		return fmt.Errorf("elastic config: tls server name must match advertise host")
	}
	if u.Hostname() != bindHost && !c.AllowPrivateNAT {
		return fmt.Errorf("elastic config: advertise host must match bind unless private nat is enabled")
	}
	return nil
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
func requiredInt(key string) (int, error) { v, err := requiredInt64(key); return int(v), err }
func requiredInt64(key string) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, fmt.Errorf("elastic config: %s required", key)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("elastic config: %s invalid", key)
	}
	return v, nil
}
func requiredDuration(key string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, fmt.Errorf("elastic config: %s required", key)
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("elastic config: %s invalid", key)
	}
	return v, nil
}
func requiredBool(key string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("elastic config: %s invalid", key)
	}
	return v, nil
}
