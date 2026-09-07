package contract

import (
	"strings"
	"testing"
	"time"
)

func validScope() CommandScope {
	return CommandScope{
		NodeID:      "node-a",
		ProjectID:   "demo",
		VersionID:   "v1",
		ReplicaID:   "rep-1",
		Generation:  1,
		LeaseExpiry: time.Now().UTC().Add(time.Hour),
		Nonce:       "nonce-1",
		Action:      ActionProbeReplica,
	}
}

func TestValidateCommandScope_ok(t *testing.T) {
	if err := ValidateCommandScope(validScope()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCommandScope_nonceTooLong(t *testing.T) {
	s := validScope()
	s.Nonce = strings.Repeat("n", MaxCommandNonceLen+1)
	if err := ValidateCommandScope(s); err == nil {
		t.Fatal("expected nonce too long")
	}
}

func TestValidateCommandScope_invalidAction(t *testing.T) {
	s := validScope()
	s.Action = LifecycleAction("not_a_real_action")
	if err := ValidateCommandScope(s); err == nil {
		t.Fatal("expected invalid action")
	}
}

func TestValidateRuntimeNodeActivationMetadata(t *testing.T) {
	ok := RuntimeNode{
		Zone: "local-a", AgentBaseURL: "https://n1.agent.example",
		IdentityURI: "spiffe://cellp/local/node/a",
	}
	if err := ValidateRuntimeNodeActivationMetadata(ok); err != nil {
		t.Fatal(err)
	}
	badURL := ok
	badURL.AgentBaseURL = "http://127.0.0.1:19443"
	if err := ValidateRuntimeNodeActivationMetadata(badURL); err == nil {
		t.Fatal("expected http rejection")
	}
	badPath := ok
	badPath.AgentBaseURL = "https://127.0.0.1:19443/extra"
	if err := ValidateRuntimeNodeActivationMetadata(badPath); err == nil {
		t.Fatal("expected path rejection")
	}
}

func TestNormalizeWireReason(t *testing.T) {
	if NormalizeWireReason(ReasonReplayRejected) != ReasonReplayRejected {
		t.Fatal("known reason preserved")
	}
	if NormalizeWireReason(ReasonCode("user_database_leaked")) != ReasonAuthFailed {
		t.Fatal("unknown reason normalized")
	}
}
