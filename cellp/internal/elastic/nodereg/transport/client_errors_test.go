package transport_test

import (
	"errors"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
)

func TestDecodeClientErrorCapacityRetryable(t *testing.T) {
	err := noderegtransport.DecodeClientErrorForTest(503, []byte(`{"reason":"capacity_exhausted"}`), "5")
	var ce *noderegtransport.ClientError
	if !errors.As(err, &ce) || ce.Reason != contract.ReasonCapacityExhausted || !ce.Retryable {
		t.Fatalf("got %v", err)
	}
	if noderegtransport.IsClientRetryable(err) != ce.Retryable {
		t.Fatal("retryable mismatch")
	}
}

func TestDecodeClientErrorAuthNotRetryable(t *testing.T) {
	err := noderegtransport.DecodeClientErrorForTest(401, []byte(`{"reason":"auth_failed"}`), "")
	if noderegtransport.IsClientRetryable(err) {
		t.Fatalf("auth must not be retryable: %v", err)
	}
}
