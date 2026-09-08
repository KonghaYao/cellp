package config

import "testing"

func TestCelldUpstreamRejectsPublicAdvertiseLiteralWithoutNAT(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	t.Setenv(envAgentCelldBindHost, "10.0.0.5")
	t.Setenv(envAgentCelldAdvertiseHost, "8.8.8.8")
	_, err := LoadStandaloneAgentConfig()
	if err == nil {
		t.Fatal("expected rejection without private nat")
	}
}

func TestCelldUpstreamAllowsPublicAdvertiseLiteralWithNAT(t *testing.T) {
	setValidElasticEnv(t)
	setValidStandaloneRemoteEnv(t)
	t.Setenv("CELLP_AGENT_ALLOW_PRIVATE_NAT", "true")
	t.Setenv(envAgentCelldBindHost, "10.0.0.5")
	t.Setenv(envAgentCelldAdvertiseHost, "8.8.8.8")
	cfg, err := LoadStandaloneAgentConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CelldAdvertiseHost != "8.8.8.8" {
		t.Fatalf("advertise=%q", cfg.CelldAdvertiseHost)
	}
}
