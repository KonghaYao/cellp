package nodereg_test

import (
	"errors"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/nodereg"
)

func TestMapHandlerErrorAuthFailedPassthrough(t *testing.T) {
	in := &nodereg.HandlerError{Reason: contract.ReasonAuthFailed, Message: "node not allowlisted"}
	out := nodereg.MapHandlerError(in)
	if !errors.Is(out, in) {
		t.Fatalf("expected passthrough, got %v", out)
	}
	if nodereg.IsRegistryUnavailable(out) {
		t.Fatal("auth must not be unavailable")
	}
}
