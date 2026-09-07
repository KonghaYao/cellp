package contract_test

import (
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestValidateRegistryRelayScope(t *testing.T) {
	now := time.Now().UTC()
	ok := contract.RegistryRelayScope{
		NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "n1",
	}
	if err := contract.ValidateRegistryRelayScope(ok, now); err != nil {
		t.Fatal(err)
	}
	stale := ok
	stale.ExpiresAt = now.Add(-time.Second)
	if err := contract.ValidateRegistryRelayScope(stale, now); err == nil {
		t.Fatal("expected expired scope rejection")
	}
}
