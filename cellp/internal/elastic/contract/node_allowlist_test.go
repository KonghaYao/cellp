package contract

import (
	"testing"
)

func TestCanonicalAgentBaseURLPrivateHost(t *testing.T) {
	got, err := CanonicalAgentBaseURL("https://10.0.0.5:9443")
	if err != nil || got != "https://10.0.0.5:9443" {
		t.Fatalf("canonical: %q err=%v", got, err)
	}
}

func TestValidateNodeRegistrationAllowlistBijection(t *testing.T) {
	uri := "spiffe://cellp/test/node/n1"
	ok := map[string]NodeRegistrationBinding{
		"n1": {IdentityURI: uri, AgentBaseURL: "https://10.0.0.5:9443"},
	}
	if err := ValidateNodeRegistrationAllowlist(ok); err != nil {
		t.Fatal(err)
	}
	dupURI := map[string]NodeRegistrationBinding{
		"n1": {IdentityURI: uri, AgentBaseURL: "https://10.0.0.5:9443"},
		"n2": {IdentityURI: uri, AgentBaseURL: "https://10.0.0.6:9443"},
	}
	if err := ValidateNodeRegistrationAllowlist(dupURI); err == nil {
		t.Fatal("expected duplicate uri rejection")
	}
}

func TestValidateConfiguredAgentBaseURLRejectsLoopbackByDefault(t *testing.T) {
	if err := ValidateConfiguredAgentBaseURL("https://127.0.0.1:9443", false); err == nil {
		t.Fatal("expected loopback rejection")
	}
	if err := ValidateConfiguredAgentBaseURL("https://127.0.0.1:9443", true); err != nil {
		t.Fatal(err)
	}
}
