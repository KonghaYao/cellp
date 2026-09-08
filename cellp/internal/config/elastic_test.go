package config

import (
	"strings"
	"testing"
)

func setValidElasticEnv(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"CELLP_ELASTIC_RUNTIME": "1", "CELLP_AGENT_NODE_ID": "node-a",
		"CELLP_AGENT_BIND_ADDR": "127.0.0.1:19443", "CELLP_AGENT_ADVERTISE_URL": "https://127.0.0.1:19443",
		"CELLP_AGENT_NODE_IDENTITY_URI":       "spiffe://cellp/local/node/node-a",
		"CELLP_AGENT_CONTROLLER_IDENTITY_URI": "spiffe://cellp/local/controller/main",
		"CELLP_AGENT_ALLOWED_CONTROLLER_URIS": "spiffe://cellp/local/controller/main",
		"CELLP_AGENT_SERVER_CERT_FILE":        "/cert", "CELLP_AGENT_SERVER_KEY_FILE": "/key", "CELLP_AGENT_SERVER_CA_FILE": "/ca",
		"CELLP_AGENT_CLIENT_CERT_FILE": "/client-cert", "CELLP_AGENT_CLIENT_KEY_FILE": "/client-key", "CELLP_AGENT_CLIENT_CA_FILE": "/client-ca",
		"CELLP_AGENT_TLS_SERVER_NAME": "127.0.0.1", "CELLP_AGENT_CERT_DENYLIST_FILE": "/denylist",
		"CELLP_AGENT_CAPACITY_UNITS": "4", "CELLP_AGENT_ZONE": "local-a", "CELLP_AGENT_HEARTBEAT_TTL": "30s",
		"CELLP_AGENT_HEARTBEAT_INTERVAL": "10s", "CELLP_AGENT_RECONCILE_INTERVAL": "5s",
		"CELLP_AGENT_MAX_BODY_BYTES": "1048576", "CELLP_AGENT_REPLAY_MAX_ENTRIES": "4096",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func TestLoadElasticConfigExplicitOffErrors(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "0")
	t.Setenv("CELLP_AGENT_CAPACITY_UNITS", "invalid")
	_, err := LoadElasticConfig()
	if err == nil {
		t.Fatal("expected elastic runtime required error")
	}
}

func TestLoadElasticConfigValid(t *testing.T) {
	setValidElasticEnv(t)
	cfg, err := LoadElasticConfig()
	if err != nil || !cfg.Enabled || cfg.NodeID != "node-a" || !cfg.AgentEmbedded {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadElasticConfigEmbeddedDefaultsTrue(t *testing.T) {
	setValidElasticEnv(t)
	t.Setenv("CELLP_AGENT_EMBEDDED", "")
	cfg, err := LoadElasticConfig()
	if err != nil || !cfg.AgentEmbedded {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadElasticConfigRejectsInvalidEmbeddedBool(t *testing.T) {
	setValidElasticEnv(t)
	t.Setenv("CELLP_AGENT_EMBEDDED", "maybe")
	_, err := LoadElasticConfig()
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestLoadElasticConfigExplicitOffErrorsBeforeEmbeddedParse(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "0")
	t.Setenv("CELLP_AGENT_EMBEDDED", "not-a-bool")
	_, err := LoadElasticConfig()
	if err == nil {
		t.Fatal("expected elastic runtime required error")
	}
}

func setControllerOutboundElasticEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CELLP_ELASTIC_RUNTIME", "1")
	t.Setenv("CELLP_AGENT_EMBEDDED", "0")
	t.Setenv("CELLP_AGENT_CONTROLLER_IDENTITY_URI", "spiffe://cellp/local/controller/main")
	t.Setenv("CELLP_AGENT_CLIENT_CERT_FILE", "/client-cert")
	t.Setenv("CELLP_AGENT_CLIENT_KEY_FILE", "/client-key")
	t.Setenv("CELLP_AGENT_CLIENT_CA_FILE", "/client-ca")
	t.Setenv("CELLP_AGENT_CERT_DENYLIST_FILE", "/denylist")
	t.Setenv("CELLP_AGENT_MAX_BODY_BYTES", "1048576")
}

func TestLoadElasticConfigControllerOutboundMinimal(t *testing.T) {
	setControllerOutboundElasticEnv(t)
	cfg, err := LoadElasticConfig()
	if err != nil || !cfg.Enabled || cfg.AgentEmbedded || cfg.NodeID != "" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadElasticConfigControllerOutboundMissingField(t *testing.T) {
	cases := map[string]string{
		"controller_identity": "CELLP_AGENT_CONTROLLER_IDENTITY_URI",
		"client_cert":         "CELLP_AGENT_CLIENT_CERT_FILE",
		"client_key":          "CELLP_AGENT_CLIENT_KEY_FILE",
		"client_ca":           "CELLP_AGENT_CLIENT_CA_FILE",
		"denylist":            "CELLP_AGENT_CERT_DENYLIST_FILE",
		"max_body":            "CELLP_AGENT_MAX_BODY_BYTES",
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			setControllerOutboundElasticEnv(t)
			t.Setenv(key, "")
			if _, err := LoadElasticConfig(); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestLoadStandaloneIgnoresEmbeddedFalse(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	t.Setenv("CELLP_AGENT_EMBEDDED", "0")
	cfg, err := LoadStandaloneAgentConfig()
	if err != nil || !cfg.Enabled || cfg.NodeID != "node-a" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestValidateServeElasticMatrix(t *testing.T) {
	elasticOn := ElasticConfig{Enabled: true, AgentEmbedded: false, ControllerIdentityURI: "spiffe://cellp/a/c"}
	remoteOff := RemoteControlConfig{}
	if err := ValidateServeElastic(elasticOn, remoteOff); err == nil {
		t.Fatal("expected remote required")
	}
	remoteOn := RemoteControlConfig{Enabled: true, ControllerIdentityURI: "spiffe://cellp/b/c"}
	if err := ValidateServeElastic(elasticOn, remoteOn); err == nil {
		t.Fatal("expected identity mismatch")
	}
	remoteOn.ControllerIdentityURI = elasticOn.ControllerIdentityURI
	if err := ValidateServeElastic(elasticOn, remoteOn); err != nil {
		t.Fatal(err)
	}
	mixed := ElasticConfig{Enabled: true, AgentEmbedded: true, ControllerIdentityURI: "spiffe://cellp/a/c"}
	if err := ValidateServeElastic(mixed, remoteOff); err != nil {
		t.Fatal(err)
	}
}

func TestLoadElasticConfigRejectsUnknownRuntimeFlag(t *testing.T) {
	t.Setenv("CELLP_ELASTIC_RUNTIME", "treu")
	_, err := LoadElasticConfig()
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestLoadElasticConfigRejectsControllerNotInAllowlist(t *testing.T) {
	setValidElasticEnv(t)
	t.Setenv("CELLP_AGENT_ALLOWED_CONTROLLER_URIS", "spiffe://cellp/local/controller/other")
	_, err := LoadElasticConfig()
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestLoadElasticConfigRejectsInvalidClasses(t *testing.T) {
	cases := map[string][2]string{
		"missing node": {"CELLP_AGENT_NODE_ID", ""}, "public bind": {"CELLP_AGENT_BIND_ADDR", "8.8.8.8:19443"},
		"http advertise":  {"CELLP_AGENT_ADVERTISE_URL", "http://127.0.0.1:19443"},
		"advertise path":  {"CELLP_AGENT_ADVERTISE_URL", "https://127.0.0.1:19443/x"},
		"wrong node uri":  {"CELLP_AGENT_NODE_IDENTITY_URI", "https://cellp/node"},
		"empty allowlist": {"CELLP_AGENT_ALLOWED_CONTROLLER_URIS", ""}, "invalid duration": {"CELLP_AGENT_HEARTBEAT_TTL", "soon"},
		"invalid integer": {"CELLP_AGENT_CAPACITY_UNITS", "many"}, "nonpositive limit": {"CELLP_AGENT_MAX_BODY_BYTES", "0"},
		"ttl ordering": {"CELLP_AGENT_HEARTBEAT_INTERVAL", "30s"}, "host mismatch": {"CELLP_AGENT_ADVERTISE_URL", "https://127.0.0.2:19443"},
		"server name mismatch": {"CELLP_AGENT_TLS_SERVER_NAME", "localhost"},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			setValidElasticEnv(t)
			t.Setenv(change[0], change[1])
			if _, err := LoadElasticConfig(); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestLoadStandaloneAgentConfigRequiresRemoteFields(t *testing.T) {
	setValidElasticEnv(t)
	cfg, err := LoadStandaloneAgentConfig()
	if err == nil {
		t.Fatalf("expected rejection without remote urls cfg=%+v", cfg)
	}
}

func TestLoadStandaloneAgentConfigValidRemote(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	cfg, err := LoadStandaloneAgentConfig()
	if err != nil || !cfg.Enabled || cfg.NodeRegBaseURL == "" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestValidateStandaloneRejectsPartialRemote(t *testing.T) {
	setControllerOutboundElasticEnv(t)
	t.Setenv("CELLP_AGENT_NODEREG_BASE_URL", "https://127.0.0.1:19450")
	_, err := LoadElasticConfig()
	if err == nil {
		t.Fatal("expected partial remote rejection")
	}
}

const incompleteStandaloneAgentSettings = "incomplete standalone agent settings"

func assertIncompleteStandaloneConfigErr(t *testing.T, err error, envValue string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected rejection for partial standalone env")
	}
	msg := err.Error()
	if !strings.Contains(msg, incompleteStandaloneAgentSettings) {
		t.Fatalf("expected error category %q, got %v", incompleteStandaloneAgentSettings, err)
	}
	if envValue != "" && strings.Contains(msg, envValue) {
		t.Fatalf("error must not contain env value, got %v", err)
	}
}

func TestLoadElasticConfigControllerOutboundRejectsPartialStandaloneEnv(t *testing.T) {
	cases := map[string]struct {
		key   string
		value string
	}{
		"nodereg_base_url":           {envAgentNodeRegBaseURL, "https://nodereg-sensitive-probe.invalid"},
		"registry_relay_base_url":    {envAgentRegistryRelayBaseURL, "https://registry-relay-sensitive-probe.invalid"},
		"relay_scope_ttl":            {envAgentRelayScopeTTL, "3721s"},
		"controller_tls_server_name": {envAgentControllerTLSServerName, "sni-sensitive-probe.invalid"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			setControllerOutboundElasticEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := LoadElasticConfig()
			assertIncompleteStandaloneConfigErr(t, err, tc.value)
		})
	}
}

func TestLoadElasticConfigControllerOutboundRejectsFullStandaloneEnv(t *testing.T) {
	setControllerOutboundElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	_, err := LoadElasticConfig()
	if err == nil {
		t.Fatal("expected rejection when standalone agent env set on controller-only serve")
	}
}

func setValidStandaloneRemoteEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CELLP_AGENT_NODEREG_BASE_URL", "https://127.0.0.1:19450")
	t.Setenv("CELLP_AGENT_REGISTRY_RELAY_BASE_URL", "https://127.0.0.1:19450")
	t.Setenv("CELLP_AGENT_RELAY_SCOPE_TTL", "90s")
}

func TestElasticConfigErrorsDoNotContainMaterial(t *testing.T) {
	setValidElasticEnv(t)
	t.Setenv("CELLP_AGENT_NODE_ID", "")
	_, err := LoadElasticConfig()
	if err == nil || strings.Contains(err.Error(), "BEGIN") {
		t.Fatalf("unsafe error: %v", err)
	}
}
