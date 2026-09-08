package agentrun

import (
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestRegistrationScopeValidatesAgainstContract(t *testing.T) {
	now := time.Now().UTC()
	scope := registrationScope("n1", 0, contract.MaxControlPlaneScopeTTL)
	if err := contract.ValidateNodeRegistrationScope(scope, now); err != nil {
		t.Fatalf("scope must satisfy contract window: %v", scope)
	}
}

func TestExpectedActivationGeneration(t *testing.T) {
	now := time.Now().UTC()
	if g, err := expectedActivationGeneration(contract.StatusNodeResponse{Found: false}, now); err != nil || g != 0 {
		t.Fatalf("g=%d err=%v", g, err)
	}
	active := contract.StatusNodeResponse{Found: true, Generation: 2, LeaseExpiry: now.Add(time.Minute)}
	if _, err := expectedActivationGeneration(active, now); err == nil {
		t.Fatal("expected active lease rejection")
	}
	expired := contract.StatusNodeResponse{Found: true, Generation: 3, LeaseExpiry: now.Add(-time.Second)}
	if g, err := expectedActivationGeneration(expired, now); err != nil || g != 3 {
		t.Fatalf("g=%d err=%v", g, err)
	}
}
