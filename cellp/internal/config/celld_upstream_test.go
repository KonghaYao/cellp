package config

import (
	"testing"
)

func TestCelldUpstreamDefaultsLoopback(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	cfg, err := LoadStandaloneAgentConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CelldBindHost != "127.0.0.1" || cfg.CelldAdvertiseHost != "127.0.0.1" {
		t.Fatalf("defaults: bind=%q advertise=%q", cfg.CelldBindHost, cfg.CelldAdvertiseHost)
	}
}

func TestCelldUpstreamPrivateBindAndAdvertiseHostname(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	t.Setenv("CELLP_AGENT_ALLOW_PRIVATE_NAT", "true")
	t.Setenv("CELLP_AGENT_CELLD_BIND_HOST", "10.0.0.5")
	t.Setenv("CELLP_AGENT_CELLD_ADVERTISE_HOST", "node-a.internal")
	cfg, err := LoadStandaloneAgentConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CelldBindHost != "10.0.0.5" || cfg.CelldAdvertiseHost != "node-a.internal" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestCelldUpstreamRejectsInvalidBind(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	for _, value := range []string{
		"https://10.0.0.1",
		"10.0.0.1:8787",
		"10.*.0.1",
		"0.0.0.0",
		"celld.local",
	} {
		t.Run(value, func(t *testing.T) {
			setValidElasticEnv(t)
			setValidStandaloneRemoteEnv(t)
			t.Setenv(envAgentCelldBindHost, value)
			_, err := LoadStandaloneAgentConfig()
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestCelldUpstreamRejectsInvalidAdvertise(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	t.Setenv("CELLP_AGENT_ALLOW_PRIVATE_NAT", "true")
	t.Setenv(envAgentCelldBindHost, "10.0.0.5")
	for _, value := range []string{
		"https://node.internal",
		"node.internal:8787",
		"0.0.0.0",
	} {
		t.Run(value, func(t *testing.T) {
			setValidElasticEnv(t)
			setValidStandaloneRemoteEnv(t)
			t.Setenv("CELLP_AGENT_ALLOW_PRIVATE_NAT", "true")
			t.Setenv(envAgentCelldBindHost, "10.0.0.5")
			t.Setenv(envAgentCelldAdvertiseHost, value)
			_, err := LoadStandaloneAgentConfig()
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestCelldUpstreamAdvertiseMismatchRequiresNAT(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	t.Setenv(envAgentCelldBindHost, "10.0.0.5")
	t.Setenv(envAgentCelldAdvertiseHost, "10.0.0.6")
	_, err := LoadStandaloneAgentConfig()
	if err == nil {
		t.Fatal("expected rejection without private nat")
	}
}
